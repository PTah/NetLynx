package notify

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/netutil"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

const emailBatchWindow = 25 * time.Second

// EventHook после сохранения события в БД пытается отправить webhook и/или Telegram (если включено в настройках).
type EventHook struct {
	log           *slog.Logger
	st            *store.Store
	w             *Webhook
	tg            *Telegram
	em            *Email
	publicBaseURL string
	wg            sync.WaitGroup
	mu            sync.Mutex
	stopping      bool // новые DispatchEvent не принимаем
	closed        bool // Wait завершился — больше нельзя wg.Add

	emailBatchMu sync.Mutex
	emailBatches map[string]*emailBatchBuf // DEVICE_OFFLINE | DEVICE_ONLINE
}

type emailBatchBuf struct {
	items []EmailDeviceCard
	timer *time.Timer
	ns    store.NotificationSettings
}

type HookOptions struct {
	PublicBaseURL string
}

func NewEventHook(log *slog.Logger, st *store.Store, w *Webhook, opts HookOptions) *EventHook {
	if log == nil {
		log = slog.Default()
	}
	return &EventHook{
		log:           log,
		st:            st,
		w:             w,
		tg:            NewTelegram(),
		em:            NewEmail(),
		publicBaseURL: strings.TrimRight(strings.TrimSpace(opts.PublicBaseURL), "/"),
		emailBatches:  make(map[string]*emailBatchBuf),
	}
}

// Wait ждёт завершения фоновых notify-горутин (с таймаутом на shutdown).
// Сначала закрывает приём новых DispatchEvent, дожидается уже запущенных
// (они могут положить письма в пачку), затем сбрасывает пачки и ждёт SMTP.
func (h *EventHook) Wait(timeout time.Duration) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.stopping = true
	h.mu.Unlock()
	deadline := time.Now().Add(timeout)
	h.waitGroup(time.Until(deadline))
	h.flushEmailBatches()
	h.waitGroup(time.Until(deadline))
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
}

func (h *EventHook) waitGroup(d time.Duration) {
	if d < 0 {
		d = 0
	}
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-done:
	case <-t.C:
		if d > 0 {
			h.log.Warn("notify: timeout waiting for in-flight dispatches", "timeout", d.String())
		}
	}
}

func (h *EventHook) tryAdd() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping || h.closed {
		return false
	}
	h.wg.Add(1)
	return true
}

// tryAddSend — отправка пачки email на shutdown (stopping уже true).
func (h *EventHook) tryAddSend() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return false
	}
	h.wg.Add(1)
	return true
}

// DispatchEvent не блокирует поллер: уходит в отдельную горутину с коротким таймаутом HTTP.
func (h *EventHook) DispatchEvent(
	deviceID int64,
	deviceName, deviceHost string,
	eventID int64,
	ifIndex *int,
	eventType, severity string,
	payload map[string]interface{},
) {
	if h == nil {
		return
	}
	if !h.tryAdd() {
		return
	}
	go func() {
		defer h.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		if mac := store.EventPayloadMAC(eventType, payload); mac != "" {
			skip, err := h.st.ShouldSkipWiFiMACTracking(ctx, mac)
			if err != nil {
				h.log.Debug("wifi mac notify skip", "mac", mac, "err", err)
				return
			}
			if skip {
				return
			}
		}
		ns, err := h.st.GetNotificationSettings(ctx)
		if err != nil {
			h.log.Debug("notification settings", "err", err)
			return
		}
		retries, backoff := normalizeRetry(ns.NotifyMaxRetries, ns.NotifyRetryBackoffMs)
		if ns.WebhookEnabled && ns.WebhookURL != nil {
			u := strings.TrimSpace(*ns.WebhookURL)
			if u != "" {
				if err := netutil.ValidateOutboundURL(u, netutil.WebhookPolicy()); err != nil {
					h.log.Warn("webhook url invalid", "url", u, "err", err)
				} else if matchesFilter(eventType, ns.WebhookEventTypes) && matchesFilter(severity, ns.WebhookSeverities) {
					body := map[string]interface{}{
						"source":       "netlynx",
						"event_id":     eventID,
						"device_id":    deviceID,
						"device_name":  deviceName,
						"device_host":  deviceHost,
						"event_type":   eventType,
						"severity":     severity,
						"if_index":     ifIndex,
						"payload":      payload,
						"occurred_iso": time.Now().UTC().Format(time.RFC3339),
					}
					if err := withRetry(ctx, retries, backoff, func(c context.Context) error {
						return h.w.PostJSON(c, u, body)
					}); err != nil {
						h.log.Warn("webhook", "err", err, "device_id", deviceID, "event_type", eventType)
					}
				}
			}
		}
		if ns.TelegramEnabled && ns.TelegramBotToken != nil && ns.TelegramChatID != nil {
			tok := strings.TrimSpace(*ns.TelegramBotToken)
			ch := strings.TrimSpace(*ns.TelegramChatID)
			if tok != "" && ch != "" && matchesFilter(eventType, ns.TelegramEventTypes) && matchesFilter(severity, ns.TelegramSeverities) {
				card := h.makeEmailCard(ctx, deviceID, deviceName, deviceHost, eventType, payload)
				msg := formatTelegramFromCard(card, deviceHost, eventType, payload, time.Now())
				if err := withRetry(ctx, retries, backoff, func(c context.Context) error {
					return h.tg.SendHTMLMessage(c, tok, ch, msg)
				}); err != nil {
					h.log.Warn("telegram", "err", err, "device_id", deviceID, "event_type", eventType)
				}
			}
		}
		if ns.EmailEnabled && ns.EmailFrom != nil && ns.EmailTo != nil && ns.SMTPHost != nil {
			if matchesFilter(eventType, ns.EmailEventTypes) && matchesFilter(severity, ns.EmailSeverities) {
				card := h.makeEmailCard(ctx, deviceID, deviceName, deviceHost, eventType, payload)
				et := strings.ToUpper(strings.TrimSpace(eventType))
				if et == "DEVICE_OFFLINE" || et == "DEVICE_ONLINE" {
					h.enqueueEmailBatch(et, card, ns)
				} else {
					h.sendEmailCards(ctx, ns, []EmailDeviceCard{card}, retries, backoff)
				}
			}
		}
	}()
}

func (h *EventHook) makeEmailCard(
	ctx context.Context,
	deviceID int64,
	deviceName, deviceHost, eventType string,
	payload map[string]interface{},
) EmailDeviceCard {
	name := strings.TrimSpace(deviceName)
	host := strings.TrimSpace(deviceHost)
	loc := ""
	cat := ""
	catLabel := ""
	if d, err := h.st.GetDevice(ctx, deviceID); err == nil && d != nil {
		if nm := strings.TrimSpace(d.Name); nm != "" {
			name = nm
		}
		if hst := strings.TrimSpace(d.Host); hst != "" {
			host = hst
		}
		if d.Location != nil {
			loc = strings.TrimSpace(*d.Location)
		}
		cat = store.NormalizeDeviceCategory(d.DeviceCategory)
		catLabel = categoryLabelForEmail(ctx, h.st, cat)
	}
	if name == "" {
		name = host
	}
	if name == "" {
		name = fmt.Sprintf("устройство #%d", deviceID)
	}
	subtitle := host
	if host != "" && catLabel != "" {
		subtitle = host + " · " + catLabel
	} else if host == "" && catLabel != "" {
		subtitle = catLabel
	}
	if host == name {
		if catLabel != "" {
			subtitle = catLabel
		} else {
			subtitle = ""
		}
	}
	if loc == name || loc == host {
		loc = ""
	}

	attachment := ""
	et := strings.ToUpper(strings.TrimSpace(eventType))
	if et == "DEVICE_OFFLINE" || et == "DEVICE_ONLINE" {
		if att, err := h.st.FindDeviceAttachment(ctx, deviceID); err == nil && att != nil {
			attachment = att.FormatRU()
		}
	}

	return EmailDeviceCard{
		DeviceID:           deviceID,
		Title:              name,
		Subtitle:           subtitle,
		Detail:             loc,
		Attachment:         attachment,
		StatusLine:         cardStatusLine(eventType, cat, payload, time.Now()),
		Kind:               et,
		Category:           cat,
		URL:                buildDeviceURL(h.publicBaseURL, deviceID),
		OfflineDurationSec: payloadOfflineDurationSec(payload),
	}
}

func categoryLabelForEmail(ctx context.Context, st *store.Store, cat string) string {
	cat = store.NormalizeDeviceCategory(cat)
	if st == nil {
		return cat
	}
	defs, err := st.ListDeviceCategoryDefs(ctx)
	if err != nil {
		return cat
	}
	for _, d := range defs {
		if d.ID == cat {
			return d.Label
		}
	}
	return cat
}

func (h *EventHook) enqueueEmailBatch(kind string, card EmailDeviceCard, ns store.NotificationSettings) {
	h.emailBatchMu.Lock()
	defer h.emailBatchMu.Unlock()
	b := h.emailBatches[kind]
	if b == nil {
		b = &emailBatchBuf{ns: ns}
		h.emailBatches[kind] = b
	}
	// ONLINE отменяет ещё не отправленный OFFLINE только при коротком флапе
	// (в пределах окна пачки). Длинный простой (рестарт посреди опроса) — оба письма.
	if strings.EqualFold(kind, "DEVICE_ONLINE") && shouldCancelPendingOffline(card.OfflineDurationSec) {
		if ob := h.emailBatches["DEVICE_OFFLINE"]; ob != nil {
			filtered := ob.items[:0]
			for _, it := range ob.items {
				if it.DeviceID != card.DeviceID {
					filtered = append(filtered, it)
				}
			}
			ob.items = filtered
			if len(ob.items) == 0 {
				if ob.timer != nil {
					ob.timer.Stop()
				}
				delete(h.emailBatches, "DEVICE_OFFLINE")
			}
		}
	}
	// дедуп по device id в окне
	for i := range b.items {
		if b.items[i].DeviceID == card.DeviceID {
			b.items[i] = card
			h.armBatchTimer(kind, b)
			return
		}
	}
	b.items = append(b.items, card)
	b.ns = ns
	h.armBatchTimer(kind, b)
}

func shouldCancelPendingOffline(onlineDurationSec int) bool {
	if onlineDurationSec <= 0 {
		return true
	}
	return onlineDurationSec <= int(emailBatchWindow.Seconds())
}

func (h *EventHook) armBatchTimer(kind string, b *emailBatchBuf) {
	if b.timer != nil {
		return
	}
	b.timer = time.AfterFunc(emailBatchWindow, func() {
		h.fireEmailBatch(kind)
	})
}

func (h *EventHook) fireEmailBatch(kind string) {
	h.emailBatchMu.Lock()
	b := h.emailBatches[kind]
	if b == nil || len(b.items) == 0 {
		h.emailBatchMu.Unlock()
		return
	}
	items := append([]EmailDeviceCard(nil), b.items...)
	ns := b.ns
	delete(h.emailBatches, kind)
	h.emailBatchMu.Unlock()

	if !h.tryAddSend() {
		return
	}
	go func() {
		defer h.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if len(items) == 0 {
			return
		}
		retries, backoff := normalizeRetry(ns.NotifyMaxRetries, ns.NotifyRetryBackoffMs)
		h.sendEmailCards(ctx, ns, items, retries, backoff)
	}()
}

func (h *EventHook) flushEmailBatches() {
	h.emailBatchMu.Lock()
	kinds := make([]string, 0, len(h.emailBatches))
	for k, b := range h.emailBatches {
		if b.timer != nil {
			b.timer.Stop()
		}
		kinds = append(kinds, k)
	}
	h.emailBatchMu.Unlock()
	for _, k := range kinds {
		h.fireEmailBatch(k)
	}
}

func (h *EventHook) sendEmailCards(
	ctx context.Context,
	ns store.NotificationSettings,
	cards []EmailDeviceCard,
	retries int,
	backoff time.Duration,
) {
	if len(cards) == 0 || ns.EmailFrom == nil || ns.EmailTo == nil || ns.SMTPHost == nil {
		return
	}
	to := strings.TrimSpace(*ns.EmailTo)
	mail := BuildAlertEmail(to, cards, time.Now())
	cfg := EmailConfig{
		From:          strings.TrimSpace(*ns.EmailFrom),
		To:            to,
		SMTPHost:      strings.TrimSpace(*ns.SMTPHost),
		SMTPPort:      ns.SMTPPort,
		SMTPUsername:  strOrEmpty(ns.SMTPUsername),
		SMTPPassword:  strOrEmpty(ns.SMTPPassword),
		TLSSkipVerify: ns.SMTPTLSSkipVerify,
	}
	inline := []InlineImage{{
		CID:         "netlynx-logo",
		ContentType: "image/png",
		Data:        LogoPNG(),
	}}
	inline = append(inline, CollectDeviceIconInline(cards)...)
	if err := withRetry(ctx, retries, backoff, func(c context.Context) error {
		return h.em.SendHTML(c, cfg, mail.Subject, mail.TextBody, mail.HTMLBody, inline)
	}); err != nil {
		h.log.Warn("email", "err", err, "cards", len(cards), "subject", mail.Subject)
	}
}

func normalizeRetry(maxRetries, backoffMs int) (int, time.Duration) {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if maxRetries > 10 {
		maxRetries = 10
	}
	if backoffMs < 100 {
		backoffMs = 500
	}
	if backoffMs > 60000 {
		backoffMs = 60000
	}
	return maxRetries, time.Duration(backoffMs) * time.Millisecond
}

func withRetry(ctx context.Context, maxRetries int, baseBackoff time.Duration, fn func(context.Context) error) error {
	var lastErr error
	attempts := 1 + maxRetries
	for attempt := 0; attempt < attempts; attempt++ {
		if err := fn(ctx); err != nil {
			lastErr = err
			if attempt == attempts-1 {
				break
			}
			delay := baseBackoff * time.Duration(1<<attempt)
			if d, ok := retryAfterOf(err); ok {
				delay = d
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
			continue
		}
		return nil
	}
	return lastErr
}

func matchesFilter(value string, filter *string) bool {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	if filter == nil || strings.TrimSpace(*filter) == "" {
		return true
	}
	for _, part := range strings.Split(*filter, ",") {
		p := strings.ToUpper(strings.TrimSpace(part))
		if p == "" {
			continue
		}
		if p == "*" || p == value {
			return true
		}
	}
	return false
}

func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
