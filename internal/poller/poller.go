package poller

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/actions"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/fdbsnapshot"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/notify"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/pinghost"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/poecli"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/portrolesync"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"sync/atomic"
)

const snmpParallel = 40

// Таймауты SNMP-опроса: обычные — для обходов живого узла; короткие — для первой пробы,
// когда ICMP уже молчит (мёртвый коммутатор иначе висит в очереди опроса ~10 с).
const (
	snmpProbeNormalTimeout = 5 * time.Second
	snmpProbeNormalRetries = 1
	snmpProbeFastTimeout   = 2 * time.Second
	snmpProbeFastRetries   = 0
)

// SNMP-backoff: если узел стабильно не отвечает по SNMP (нет агента — камеры, ПК, ТВ),
// не жжём слот опроса таймаутами и не спамим журнал: после N сбоев подряд SNMP
// не опрашивается N минут, ICMP при этом продолжает работать (достижимость видна).
const (
	snmpFailThreshold = 3
	snmpBackoffPause  = 15 * time.Minute
)

type snmpBackoffState struct {
	failures int
	until    time.Time
}

// Базовое число подряд опросов с IsOnline=false до DEVICE_OFFLINE (критичные/важные).
// Один сбой SNMP при живом ping (часто EdgeSwitch) иначе даёт ложный email.
// Для AP/телефонов/камер/МФУ см. offlineConfirmForCategory — там порог выше.
const offlineConfirmPolls = 2

// Сколько подряд успешных опросов до DEVICE_ONLINE после оффлайна.
// Один краткий ping во сне (WoL/ARP offload) иначе даёт ONLINE→OFFLINE за ~1 мин (Varenik-PC).
const onlineConfirmPolls = 2

// Пустой LLDP/CDP walk (err=nil, len=0) не должен мгновенно staleить все рёбра —
// vendor SNMP иногда отдаёт пустую таблицу на один проход.
const neighborEmptyConfirmPolls = 2

// Если процесс перезапускали, пока узел уже был оффлайн, in-memory streak пустой.
// DEVICE_ONLINE по offline_since только если простой явно длиннее одного опроса:
// иначе 1.5 мин SNMP-blip даёт «снова в сети» без письма об оффлайне.
const onlineRecoverMinOffline = 3 * time.Minute

type Engine struct {
	log                    *slog.Logger
	st                     *store.Store
	cfg                    config.Config
	hook                   *notify.EventHook // может быть nil
	hub                    *live.Hub
	topoDirty              func(reason string) // optional: topology blast cache NotifyDirty
	pollInflight           sync.Map            // device id -> struct{}: опрос уже выполняется
	offlineStreak          sync.Map            // device id -> offlineStreakState
	onlineStreak           sync.Map            // device id -> int: подряд «онлайн» после оффлайна
	neighborEmptyStreak    sync.Map            // "deviceID:protocol" -> int: подряд пустой neighbor walk
	broadcastStormActive   sync.Map            // device id -> bool: эвристика broadcast storm активна
	upsDischargeAlertAt    sync.Map            // device id -> time.Time: антидребезг UPS_DISCHARGE_FAST
	utilSampleAt           sync.Map            // device id -> time.Time: троттлинг util/rate сэмплов
	snmpBackoff            sync.Map            // device id -> snmpBackoffState: узел без SNMP не опрашиваем таймаутами
	macMultiAccessLastEmit sync.Map            // mac -> time.Time: глобальный дедуп MAC_MULTI_ACCESS
	wifiFilterMu           sync.Mutex
	wifiFilterCache        wifiFilterCache
	bg                     sync.WaitGroup
	bgMu                   sync.Mutex
	bgStopping             bool
	pollPaused             atomic.Bool
}

func New(log *slog.Logger, st *store.Store, cfg config.Config, hook *notify.EventHook, hub *live.Hub) *Engine {
	if log == nil {
		log = slog.Default()
	}
	return &Engine{log: log, st: st, cfg: cfg, hook: hook, hub: hub}
}

// SetTopologyDirty — callback для debounced rebuild topology blast cache.
func (e *Engine) SetTopologyDirty(fn func(reason string)) {
	if e == nil {
		return
	}
	e.topoDirty = fn
}

func (e *Engine) notifyTopologyDirty(reason string) {
	if e == nil || e.topoDirty == nil {
		return
	}
	e.topoDirty(reason)
}

// ifaceEventPayload поля интерфейса для событий: ifAlias перекрывает SNMP ifDescr; ручная подпись порта — ещё выше.
func ifaceEventPayload(row snmp.IfRow, old store.InterfaceSnapshot) map[string]interface{} {
	desc := snmp.HumanizeSNMPText(row.IfDescr)
	if alias := strings.TrimSpace(row.IfAlias); alias != "" {
		desc = snmp.HumanizeSNMPText(alias)
	}
	if ov := old.DisplayDescr(); ov != "" && old.DescrOverride != nil && strings.TrimSpace(*old.DescrOverride) != "" {
		desc = ov
	}
	pl := map[string]interface{}{
		"if_descr": desc,
		"if_name":  row.IfName,
	}
	if a := strings.TrimSpace(row.IfAlias); a != "" {
		pl["if_alias"] = a
	}
	return pl
}

// snapshotBitsPerSecond — номинал линка из прошлого снимка (ifHighSpeed Мбит/с, ifSpeed бит/с).
func snapshotBitsPerSecond(old store.InterfaceSnapshot) int64 {
	if old.IfHighSpeed != nil && *old.IfHighSpeed > 0 {
		return *old.IfHighSpeed * 1_000_000
	}
	if old.IfSpeed != nil && *old.IfSpeed > 0 {
		return *old.IfSpeed
	}
	return 0
}

func ignoreReaction(ignore map[int]store.PortEventIgnore, ifIdx *int, typ string) (blockEvents, blockNotify, blockActions bool) {
	if ifIdx == nil || len(ignore) == 0 {
		return false, false, false
	}
	rule, ok := ignore[*ifIdx]
	if !ok || !store.MatchesIgnoreEventTypes(typ, rule.EventTypes) {
		return false, false, false
	}
	return rule.BlockEvents, rule.BlockNotify, rule.BlockActions
}

func (e *Engine) emit(ctx context.Context, ignore map[int]store.PortEventIgnore, dev store.PollDevice, ifIdx *int, typ, sev string, pl map[string]interface{}) {
	blockEv, blockNotify, blockActions := ignoreReaction(ignore, ifIdx, typ)
	if blockEv {
		return
	}
	if mac := store.EventPayloadMAC(typ, pl); mac != "" {
		skip, err := e.st.ShouldSkipWiFiMACTracking(ctx, mac)
		if err != nil {
			e.log.Warn("wifi mac event skip", "mac", mac, "type", typ, "err", err)
			return
		}
		if skip {
			return
		}
	}
	id, err := e.st.InsertEvent(ctx, dev.ID, ifIdx, typ, sev, pl)
	if err != nil {
		e.log.Warn("insert event", "err", err, "device_id", dev.ID, "type", typ)
		return
	}
	if e.hub != nil {
		e.hub.Publish(live.EventPayload{
			EventID: id, DeviceID: dev.ID, DeviceName: dev.Name, DeviceHost: dev.Host,
			IfIndex: ifIdx, EventType: typ, Severity: sev, Payload: pl,
		})
	}
	if e.hook != nil && !blockNotify {
		e.hook.DispatchEvent(dev.ID, dev.Name, dev.Host, id, ifIdx, typ, sev, pl)
	}
	if !blockActions {
		actCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		if !e.tryBgAdd() {
			cancel()
		} else {
			go func() {
				defer e.bg.Done()
				defer cancel()
				e.maybeIncidentAction(actCtx, dev, ifIdx, typ)
			}()
		}
	}
}

// annotateLinkFromTrap: если был trap linkUp/Down — помечаем событие как подтверждённое опросом.
// Возвращает true, если LINK_* уже создан из trap (дубликат от poller не нужен).
func (e *Engine) annotateLinkFromTrap(ctx context.Context, deviceID int64, ifIndex, expectedOper int, eventType string, pl map[string]interface{}) bool {
	pending, ok, err := e.st.TakeTrapPendingLink(ctx, deviceID, ifIndex, expectedOper)
	if err != nil {
		e.log.Warn("trap pending take", "device_id", deviceID, "if_index", ifIndex, "err", err)
		return false
	}
	if !ok || pending == nil {
		return false
	}
	pl["trap_confirmed"] = true
	pl["trap_label"] = pending.TrapLabel
	pl["trap_received_at"] = pending.ReceivedAt.UTC().Format(time.RFC3339Nano)
	if pending.SourceIP != "" {
		pl["trap_source_ip"] = pending.SourceIP
	}
	confirmed, err := e.st.ConfirmRecentTrapLinkEvent(ctx, deviceID, ifIndex, eventType)
	if err != nil {
		e.log.Warn("trap link confirm", "device_id", deviceID, "if_index", ifIndex, "err", err)
		return false
	}
	return confirmed
}

// TrapLinkIncidentAction — port actions из traprecv при link_trap_effects=full.
func (e *Engine) TrapLinkIncidentAction(ctx context.Context, deviceID int64, ifIndex int, typ string) {
	if e == nil || ifIndex <= 0 {
		return
	}
	pd, err := e.st.GetPollDevice(ctx, deviceID)
	if err != nil || pd == nil {
		return
	}
	e.maybeIncidentAction(ctx, *pd, &ifIndex, typ)
}

// TrapLinkPortFlap — после trap LINK_UP/DOWN: кандидат на PORT_FLAP.
func (e *Engine) TrapLinkPortFlap(ctx context.Context, deviceID int64, ifIndex int, _ string) {
	e.NotifyPortFlapAfterLink(ctx, deviceID, ifIndex)
}

func deviceReachabilityPayload(dev *models.Device) map[string]interface{} {
	pl := map[string]interface{}{
		"host":            dev.Host,
		"name":            dev.Name,
		"device_category": dev.DeviceCategory,
	}
	if dev.LastSNMPOK != nil {
		pl["last_snmp_ok"] = *dev.LastSNMPOK
	}
	if dev.LastPingOK != nil {
		pl["last_ping_ok"] = *dev.LastPingOK
	}
	if dev.LastSNMPError != nil && strings.TrimSpace(*dev.LastSNMPError) != "" {
		pl["last_snmp_error"] = *dev.LastSNMPError
	}
	if dev.LastPingRTTMs != nil {
		pl["last_ping_rtt_ms"] = *dev.LastPingRTTMs
	}
	switch models.NormalizeReachabilityMode(dev.ReachabilityMode, dev.OnlineOverride) {
	case "online", "offline":
		pl["reason"] = "override"
	case "ping":
		pl["reason"] = "ping"
	default:
		cat := strings.ToLower(strings.TrimSpace(dev.DeviceCategory))
		switchLike := cat == "" || cat == "switch" || cat == "router" || cat == "коммутатор" || cat == "коммутаторы"
		if switchLike {
			pl["reason"] = "snmp"
		} else if dev.LastPingOK != nil && !*dev.LastPingOK {
			pl["reason"] = "ping"
		} else if dev.LastSNMPOK != nil && !*dev.LastSNMPOK {
			pl["reason"] = "snmp"
		} else {
			pl["reason"] = "reachability"
		}
	}
	if !dev.IsOnline() && dev.OfflineSince != nil {
		pl["offline_since"] = dev.OfflineSince.UTC().Format(time.RFC3339)
	}
	return pl
}

// offlineStreakState считает подряд «оффлайн» только в эпизоде после online→offline.
// После рестарта уже-оффлайн узлы не должны снова слать DEVICE_OFFLINE.
type offlineStreakState struct {
	n          int
	fromOnline bool
}

func onlineRecoverThreshold(pollIntervalSec int) time.Duration {
	need := onlineRecoverMinOffline
	if pollIntervalSec > 0 {
		twoPolls := time.Duration(pollIntervalSec*2) * time.Second
		if twoPolls > need {
			need = twoPolls
		}
	}
	return need
}

// shouldEmitDeviceOnline — recovery-письмо.
// OFFLINE после рестарта не дублируем (streak in-memory). ONLINE по БД — только
// если за этот простой уже было DEVICE_OFFLINE (иначе «снова в сети» без «ушёл»).
func shouldEmitDeviceOnline(wasOnline bool, streak offlineStreakState, offlineSince *time.Time, now time.Time, pollIntervalSec int, hasOfflineEvent bool) bool {
	if wasOnline {
		return false
	}
	if streak.fromOnline && streak.n >= offlineConfirmPolls {
		return true
	}
	if hasOfflineEvent && offlineSince != nil && now.Sub(*offlineSince) >= onlineRecoverThreshold(pollIntervalSec) {
		return true
	}
	return false
}

// shouldCatchUpDeviceOffline — процесс рестартнули до 2-го неудачного опроса:
// DEVICE_OFFLINE не создали, а узел всё ещё лежит дольше порога.
func shouldCatchUpDeviceOffline(wasOnline bool, streak offlineStreakState, offlineSince *time.Time, now time.Time, pollIntervalSec int, hasOfflineEvent bool) bool {
	if wasOnline || hasOfflineEvent || streak.fromOnline || offlineSince == nil {
		return false
	}
	return now.Sub(*offlineSince) >= onlineRecoverThreshold(pollIntervalSec)
}

// shouldEmitDeviceOfflineNow — когда сообщать DEVICE_OFFLINE.
// Обычно ждём confirm опросов (короткий SNMP-блип при живом ping не должен давать письмо).
// Для критичных/важных: если ICMP отвечал и замолчал (и SNMP тоже) — сразу на первом опросе.
// Для AP/телефонов/камер instant ping-dead выключен — только полный confirm.
func shouldEmitDeviceOfflineNow(streak offlineStreakState, pingWentDead bool, confirm int, instantPingDead bool) bool {
	if !streak.fromOnline {
		return false
	}
	if confirm < 1 {
		confirm = offlineConfirmPolls
	}
	if streak.n == confirm {
		return true
	}
	return instantPingDead && streak.n == 1 && pingWentDead
}

// pingWentDead — ICMP отвечал в прошлом опросе и молчит в текущем (при этом SNMP тоже
// не ответил: IsOnline=false). Для узлов, где ICMP закрыт всегда (prev.LastPingOK=false),
// условие не срабатывает — такие узлы подтверждаются двумя опросами, как раньше.
func pingWentDead(cur, prev *models.Device) bool {
	if cur == nil || prev == nil || cur.LastPingOK == nil || prev.LastPingOK == nil {
		return false
	}
	return !*cur.LastPingOK && *prev.LastPingOK
}

// offlineEventLookupSkew — DEVICE_OFFLINE пишется до RefreshOfflineSince; created_at события
// может быть на несколько мс раньше offline_since. Без запаса catch-up после рестарта
// не видит алерт и шлёт второй OFFLINE (SNR 192.168.160.168: 3 мс → дубль в 12:01).
const offlineEventLookupSkew = 30 * time.Second

// emitReachabilityTransition — DEVICE_OFFLINE / DEVICE_ONLINE при смене IsOnline после опроса.
// OFFLINE: был онлайн → N подряд неудачных опросов (короткий SNMP-timeout не шлёт письмо).
// ONLINE: после оффлайна ждём onlineConfirmPolls успешных опросов подряд (антидребезг сна/ARP).
// holdOfflineSince=true — не сбрасывать offline_since (ждём подтверждения ONLINE).
func (e *Engine) emitReachabilityTransition(ctx context.Context, d store.PollDevice, prev *models.Device, wasOnline bool) (holdOfflineSince bool) {
	cur, err := e.st.GetDevice(ctx, d.ID)
	if err != nil || cur == nil {
		return false
	}
	nowOnline := cur.IsOnline()
	var since *time.Time
	if prev != nil {
		since = prev.OfflineSince
	}
	now := time.Now()
	hasOff := false
	if since != nil {
		hasOff, err = e.st.HasEventSince(ctx, d.ID, "DEVICE_OFFLINE", since.Add(-offlineEventLookupSkew))
		if err != nil {
			e.log.Warn("has DEVICE_OFFLINE", "device_id", d.ID, "err", err)
		}
	}

	if nowOnline {
		prevStreak := e.peekOfflineStreak(d.ID)
		if shouldCatchUpDeviceOffline(wasOnline, prevStreak, since, now, d.PollIntervalSeconds, hasOff) {
			e.emitDeviceOffline(ctx, d, cur, prev, 0)
			hasOff = true
			prevStreak = offlineStreakState{n: offlineConfirmForCategory(d.DeviceCategory), fromOnline: true}
			e.offlineStreak.Store(d.ID, prevStreak)
		}
		// recovering: OFFLINE уже был, offline_since ещё держим (в т.ч. после 1-го ping,
		// когда wasOnline уже true из last_ping_ok — иначе confirm срывается).
		recovering := hasOff && cur.OfflineSince != nil
		wantOnline := recovering || shouldEmitDeviceOnline(wasOnline, prevStreak, since, now, d.PollIntervalSeconds, hasOff)
		if wantOnline {
			n := e.noteOnlinePoll(d.ID)
			if n >= onlineConfirmPolls {
				e.resetOfflineStreak(d.ID)
				e.resetOnlineStreak(d.ID)
				e.emitDeviceOnline(ctx, d, cur, prev)
				return false
			}
			// Краткий ping во сне: ещё не ONLINE, offline_since держим.
			e.resetOfflineStreak(d.ID)
			return true
		}
		e.resetOfflineStreak(d.ID)
		e.resetOnlineStreak(d.ID)
		return false
	}

	// Снова оффлайн: если ONLINE ещё не подтвердили — тот же эпизод, без второго OFFLINE.
	onlineN := e.resetOnlineStreak(d.ID)
	if onlineN > 0 && onlineN < onlineConfirmPolls && hasOff {
		confirm := offlineConfirmForCategory(d.DeviceCategory)
		e.offlineStreak.Store(d.ID, offlineStreakState{n: confirm, fromOnline: true})
		return false
	}

	st := e.noteOfflinePoll(d.ID, wasOnline)
	confirm := offlineConfirmForCategory(d.DeviceCategory)
	instant := allowInstantPingDead(d.DeviceCategory)
	if shouldEmitDeviceOfflineNow(st, pingWentDead(cur, prev), confirm, instant) {
		e.emitDeviceOffline(ctx, d, cur, prev, st.n)
		// Эпизод зафиксирован: до восстановления повторов не шлём
		// (следующий опрос даст n=confirm+1 и условие уже не сработает).
		e.offlineStreak.Store(d.ID, offlineStreakState{n: confirm, fromOnline: true})
		return false
	}
	if shouldCatchUpDeviceOffline(wasOnline, st, since, now, d.PollIntervalSeconds, hasOff) {
		e.emitDeviceOffline(ctx, d, cur, prev, st.n)
		e.offlineStreak.Store(d.ID, offlineStreakState{n: confirm, fromOnline: true})
	}
	return false
}

func (e *Engine) emitDeviceOnline(ctx context.Context, d store.PollDevice, cur *models.Device, prev *models.Device) {
	pl := deviceReachabilityPayload(cur)
	if prev != nil && prev.OfflineSince != nil {
		pl["was_offline_since"] = prev.OfflineSince.UTC().Format(time.RFC3339)
		pl["offline_duration_sec"] = int(time.Since(*prev.OfflineSince).Seconds())
	}
	e.emit(ctx, nil, d, nil, "DEVICE_ONLINE", "info", pl)
}

func (e *Engine) emitDeviceOffline(ctx context.Context, d store.PollDevice, cur *models.Device, prev *models.Device, confirmPolls int) {
	// Сначала offline_since, потом событие — иначе created_at < offline_since и после рестарта дубль.
	if err := e.st.EnsureOfflineSince(ctx, d.ID); err != nil {
		e.log.Warn("ensure offline_since", "device_id", d.ID, "err", err)
	}
	pl := deviceReachabilityPayload(cur)
	if confirmPolls > 0 {
		pl["offline_confirm_polls"] = confirmPolls
	}
	if prev != nil && prev.OfflineSince != nil {
		pl["offline_since"] = prev.OfflineSince.UTC().Format(time.RFC3339)
	} else if cur != nil && cur.OfflineSince != nil {
		pl["offline_since"] = cur.OfflineSince.UTC().Format(time.RFC3339)
	} else if fresh, err := e.st.GetDevice(ctx, d.ID); err == nil && fresh != nil && fresh.OfflineSince != nil {
		pl["offline_since"] = fresh.OfflineSince.UTC().Format(time.RFC3339)
	}
	e.emit(ctx, nil, d, nil, "DEVICE_OFFLINE", "warning", pl)
}

func (e *Engine) noteOfflinePoll(deviceID int64, wasOnline bool) offlineStreakState {
	if wasOnline {
		st := offlineStreakState{n: 1, fromOnline: true}
		e.offlineStreak.Store(deviceID, st)
		return st
	}
	v, _ := e.offlineStreak.Load(deviceID)
	st := offlineStreakState{}
	if v != nil {
		if prev, ok := v.(offlineStreakState); ok {
			st = prev
		}
	}
	st.n++
	e.offlineStreak.Store(deviceID, st)
	return st
}

func (e *Engine) peekOfflineStreak(deviceID int64) offlineStreakState {
	v, _ := e.offlineStreak.Load(deviceID)
	if v == nil {
		return offlineStreakState{}
	}
	st, _ := v.(offlineStreakState)
	return st
}

func (e *Engine) resetOfflineStreak(deviceID int64) offlineStreakState {
	v, loaded := e.offlineStreak.LoadAndDelete(deviceID)
	if !loaded || v == nil {
		return offlineStreakState{}
	}
	st, _ := v.(offlineStreakState)
	return st
}

func (e *Engine) noteOnlinePoll(deviceID int64) int {
	n := 1
	if v, ok := e.onlineStreak.Load(deviceID); ok {
		if prev, ok := v.(int); ok && prev > 0 {
			n = prev + 1
		}
	}
	e.onlineStreak.Store(deviceID, n)
	return n
}

func (e *Engine) resetOnlineStreak(deviceID int64) int {
	v, loaded := e.onlineStreak.LoadAndDelete(deviceID)
	if !loaded || v == nil {
		return 0
	}
	n, _ := v.(int)
	return n
}

func (e *Engine) SetPollingPaused(v bool) {
	if e == nil {
		return
	}
	e.pollPaused.Store(v)
	if v {
		e.log.Info("poller: paused for backup")
	} else {
		e.log.Info("poller: resumed after backup")
	}
}

func (e *Engine) tryBgAdd() bool {
	e.bgMu.Lock()
	defer e.bgMu.Unlock()
	if e.bgStopping {
		return false
	}
	e.bg.Add(1)
	return true
}

// Wait ждёт фоновые incident-actions (shutdown).
// Перед Wait закрывает приём новых фоновых задач (иначе Add после Wait — UB).
func (e *Engine) Wait(timeout time.Duration) {
	if e == nil {
		return
	}
	e.bgMu.Lock()
	e.bgStopping = true
	e.bgMu.Unlock()
	done := make(chan struct{})
	go func() {
		e.bg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		e.log.Warn("poller: timeout waiting for background actions", "timeout", timeout.String())
	}
}

func (e *Engine) Run(ctx context.Context) {
	t := time.NewTicker(time.Duration(e.cfg.PollSchedulerSeconds) * time.Second)
	defer t.Stop()
	prune := time.NewTicker(6 * time.Hour)
	defer prune.Stop()
	if n, err := e.st.RepairAllFDBTopologyLinks(ctx); err != nil {
		e.log.Warn("fdb topology repair", "err", err)
	} else if n > 0 {
		e.log.Info("fdb topology repair", "linked", n)
	}
	e.pollAll(ctx) // сразу один цикл
	e.pruneMetrics(ctx)
	e.pruneMACMoves(ctx)
	e.pruneStaleLiveFDB(ctx)
	e.scanMACMultiAccess(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.pollAll(ctx)
		case <-prune.C:
			e.pruneMetrics(ctx)
			e.pruneMACMoves(ctx)
			e.pruneStaleLiveFDB(ctx)
			e.scanMACMultiAccess(ctx)
		}
	}
}

func (e *Engine) pruneMetrics(ctx context.Context) {
	if !e.cfg.MetricsEnabled || e.cfg.MetricsRetentionDays <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(e.cfg.MetricsRetentionDays) * 24 * time.Hour)
	printerDays := e.cfg.MetricsPrinterRetentionDays
	if printerDays <= 0 {
		printerDays = e.cfg.MetricsRetentionDays
	}
	printerCutoff := time.Now().Add(-time.Duration(printerDays) * 24 * time.Hour)
	if n, err := e.st.PruneMetricSamples(ctx, cutoff, printerCutoff); err != nil {
		e.log.Warn("prune metrics", "err", err)
	} else if n > 0 {
		e.log.Info("prune metrics", "deleted", n, "retention_days", e.cfg.MetricsRetentionDays, "printer_retention_days", printerDays)
	}
}

func (e *Engine) pruneStaleLiveFDB(ctx context.Context) {
	days := e.cfg.FDBStaleClearDays
	if days <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	res, err := e.st.ClearStaleLiveFDB(ctx, cutoff)
	if err != nil {
		e.log.Warn("prune stale live FDB", "err", err)
		return
	}
	if res.DevicesAffected > 0 || res.EntriesDeleted > 0 {
		e.log.Info("prune stale live FDB",
			"devices", res.DevicesAffected,
			"entries", res.EntriesDeleted,
			"older_than_days", days)
	}
}

// Приоритет опроса по типу узла: критичные типы опрашиваются чаще и раньше в очереди.
const (
	pollPriorityCritical  = 0 // роутер, коммутатор, сервер — падение надо видеть сразу
	pollPriorityHigh      = 1 // NAS — сразу после критичных
	pollPriorityImportant = 2 // ИБП, компьютер, iLO/IPMI, гипервизор
	pollPrioritySoft      = 3 // AP, SIP-телефоны — алерт после нескольких неудач
	pollPriorityLow       = 4 // камеры, МФУ, прочее — реже и с большим подтверждением
)

func pollPriority(category string) int {
	switch store.NormalizeDeviceCategory(category) {
	case store.DeviceCategoryRouter, store.DeviceCategorySwitch, store.DeviceCategoryServer:
		return pollPriorityCritical
	case "nas":
		return pollPriorityHigh
	case store.DeviceCategoryUPS, store.DeviceCategoryComputer, "ilo", "hypervisor", "ipmi":
		return pollPriorityImportant
	case store.DeviceCategoryAP, store.DeviceCategoryPhone:
		return pollPrioritySoft
	default:
		return pollPriorityLow
	}
}

// offlineConfirmForCategory — сколько подряд неудачных опросов до DEVICE_OFFLINE.
// Карточка «Опрашивать каждые N сек» задаёт частоту; здесь — антидребезг алерта.
func offlineConfirmForCategory(category string) int {
	switch pollPriority(category) {
	case pollPriorityCritical, pollPriorityHigh:
		return offlineConfirmPolls // 2; при пропаже ICMP — сразу (см. allowInstantPingDead)
	case pollPriorityImportant:
		return offlineConfirmPolls
	case pollPrioritySoft:
		return 3 // AP / телефоны: веб может молчать при живом SIP
	default:
		return 3 // камеры, МФУ, иное
	}
}

// allowInstantPingDead — слать OFFLINE на первом опросе, если ICMP отвечал и замолчал.
// Для «мягких» узлов (телефоны, камеры) ICMP часто врёт — ждём полный confirm.
func allowInstantPingDead(category string) bool {
	switch pollPriority(category) {
	case pollPrioritySoft, pollPriorityLow:
		return false
	default:
		return true
	}
}

func (e *Engine) shouldSchedulePoll(d *store.PollDevice, now time.Time) bool {
	sec := d.PollIntervalSeconds
	if sec <= 0 {
		sec = 60
	}
	if sec < 10 {
		sec = 10
	}
	if sec > 86400 {
		sec = 86400
	}
	interval := time.Duration(sec) * time.Second
	// Критичные типы (роутер/коммутатор/сервер) опрашиваем не реже CriticalPollInterval:
	// их пропажу из сети надо видеть сразу, даже если в карточке интервал больше.
	if pollPriority(d.DeviceCategory) == pollPriorityCritical && e.cfg.CriticalPollInterval > 0 &&
		interval > e.cfg.CriticalPollInterval {
		interval = e.cfg.CriticalPollInterval
	}
	if d.LastPollAt == nil {
		return true
	}
	return now.Sub(*d.LastPollAt) >= interval
}

// pollIntervalOf — фактический интервал лёгкого опроса с учётом приоритета типа.
func (e *Engine) pollIntervalOf(d *store.PollDevice) time.Duration {
	sec := d.PollIntervalSeconds
	if sec <= 0 {
		sec = 60
	}
	if sec < 10 {
		sec = 10
	}
	interval := time.Duration(sec) * time.Second
	if pollPriority(d.DeviceCategory) == pollPriorityCritical && e.cfg.CriticalPollInterval > 0 &&
		interval > e.cfg.CriticalPollInterval {
		interval = e.cfg.CriticalPollInterval
	}
	return interval
}

// shouldPollHeavy — пора ли делать тяжёлый опрос (LLDP/CDP/ARP/PoE/STP/FDB, роли портов).
func (e *Engine) shouldPollHeavy(d *store.PollDevice, now time.Time) bool {
	interval := e.cfg.HeavyPollInterval
	if d.HeavyPollIntervalSeconds != nil && *d.HeavyPollIntervalSeconds > 0 {
		interval = time.Duration(*d.HeavyPollIntervalSeconds) * time.Second
	}
	if interval <= 0 {
		return false
	}
	if d.LastHeavyPollAt == nil {
		return true
	}
	return now.Sub(*d.LastHeavyPollAt) >= interval
}

// snmpBackoffEligible — пауза SNMP только для узлов, у которых агента часто нет
// (камеры, МФУ, AP, телефоны, «прочее»). Коммутатор/роутер/сервер/ИБП/ПК —
// всегда опрашиваем: один таймаут не должен прятать свитч на 15 минут.
func snmpBackoffEligible(category string) bool {
	switch pollPriority(category) {
	case pollPrioritySoft, pollPriorityLow:
		return true
	default:
		return false
	}
}

// snmpBackoffActive — SNMP-опрос узла на паузе (устройство не отвечает по SNMP).
func (e *Engine) snmpBackoffActive(deviceID int64, now time.Time) bool {
	if v, ok := e.snmpBackoff.Load(deviceID); ok {
		if st, okT := v.(snmpBackoffState); okT {
			return now.Before(st.until)
		}
	}
	return false
}

// noteSNMPFailure — учитывает сбой SNMP; после snmpFailThreshold подряд ставит паузу.
func (e *Engine) noteSNMPFailure(deviceID int64, now time.Time) snmpBackoffState {
	st := snmpBackoffState{}
	if v, ok := e.snmpBackoff.Load(deviceID); ok {
		if prev, okT := v.(snmpBackoffState); okT {
			st = prev
		}
	}
	st.failures++
	if st.failures >= snmpFailThreshold && now.After(st.until) {
		st.until = now.Add(snmpBackoffPause)
		e.log.Warn("snmp backoff", "device_id", deviceID, "failures", st.failures, "pause", snmpBackoffPause.String())
	}
	e.snmpBackoff.Store(deviceID, st)
	return st
}

// clearSNMPFailures — успешный опрос: счётчик сбоев и пауза сбрасываются.
func (e *Engine) clearSNMPFailures(deviceID int64) {
	e.snmpBackoff.Delete(deviceID)
}

// ClearSNMPBackoff — сброс паузы SNMP (после ручной проверки / смены community).
func (e *Engine) ClearSNMPBackoff(deviceID int64) {
	e.clearSNMPFailures(deviceID)
}

// RequestPoll — сбрасывает backoff и ставит полный опрос узла в фон (не блокирует HTTP).
func (e *Engine) RequestPoll(deviceID int64) {
	if e == nil || deviceID <= 0 {
		return
	}
	e.clearSNMPFailures(deviceID)
	if !e.tryBgAdd() {
		return
	}
	go func() {
		defer e.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		pd, err := e.st.GetPollDevice(ctx, deviceID)
		if err != nil || pd == nil {
			return
		}
		if _, loaded := e.pollInflight.LoadOrStore(pd.ID, struct{}{}); loaded {
			return
		}
		defer e.pollInflight.Delete(pd.ID)
		if err := e.pollOne(ctx, *pd); err != nil {
			e.log.Debug("request poll", "device_id", deviceID, "err", err)
		}
	}()
}

// snmpFailures — сколько сбоев SNMP подряд у узла (для уровня логирования).
func (e *Engine) snmpFailures(deviceID int64) int {
	if v, ok := e.snmpBackoff.Load(deviceID); ok {
		if st, okT := v.(snmpBackoffState); okT {
			return st.failures
		}
	}
	return 0
}

func (e *Engine) pollAll(ctx context.Context) {
	if e.pollPaused.Load() {
		return
	}
	devs, err := e.st.ListPollDevices(ctx)
	if err != nil {
		e.log.Error("poll list devices", "err", err)
		return
	}
	now := time.Now()
	due := make([]store.PollDevice, 0, len(devs))
	for _, d := range devs {
		if !e.shouldSchedulePoll(&d, now) {
			continue
		}
		due = append(due, d)
	}
	// Сначала критичные типы (роутер/коммутатор/сервер), внутри — самые просроченные:
	// при большом батче (например сразу после рестарта, когда «просрочены» все) хвост
	// списка иначе не опрашивается минутами, и ребут устройства в этом окне не виден.
	sort.SliceStable(due, func(i, j int) bool {
		pi, pj := pollPriority(due[i].DeviceCategory), pollPriority(due[j].DeviceCategory)
		if pi != pj {
			return pi < pj
		}
		return e.overdueBy(due[i], now) > e.overdueBy(due[j], now)
	})
	sem := make(chan struct{}, snmpParallel)
	var wg sync.WaitGroup
	for _, d := range due {
		d := d
		if _, loaded := e.pollInflight.LoadOrStore(d.ID, struct{}{}); loaded {
			continue
		}
		wg.Add(1)
		go func(pd store.PollDevice) {
			defer e.pollInflight.Delete(pd.ID)
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if err := e.pollOne(ctx, pd); err != nil {
				// Узлы без SNMP-агента не должны заливать журнал: первый сбой эпизода — warn,
				// повторные попытки (раз в 15 минут) — debug. На опрос это не влияет.
				if e.snmpFailures(pd.ID) > 1 {
					e.log.Debug("poll device", "device_id", pd.ID, "host", pd.Host, "err", err)
				} else {
					e.log.Warn("poll device", "device_id", pd.ID, "host", pd.Host, "err", err)
				}
			}
		}(d)
	}
	wg.Wait()
}

// overdueBy — насколько опрос просрочен относительно фактического интервала узла.
func (e *Engine) overdueBy(d store.PollDevice, now time.Time) time.Duration {
	if d.LastPollAt == nil {
		return time.Duration(1<<62 - 1) // никогда не опрашивался — в первую очередь
	}
	return now.Sub(*d.LastPollAt) - e.pollIntervalOf(&d)
}

func (e *Engine) pollOne(ctx context.Context, d store.PollDevice) error {
	if strings.TrimSpace(d.Host) == "" {
		return nil // нет IP — склад / адрес ещё не задан
	}
	wasOnline := false
	var prevDev *models.Device
	if prev, err := e.st.GetDevice(ctx, d.ID); err == nil && prev != nil {
		prevDev = prev
		wasOnline = prev.IsOnline()
	}
	defer func() {
		holdOffline := e.emitReachabilityTransition(ctx, d, prevDev, wasOnline)
		if holdOffline {
			// Антидребезг ONLINE: один удачный ping ещё не «снова в сети».
			if err := e.st.EnsureOfflineSince(ctx, d.ID); err != nil {
				e.log.Warn("hold offline_since", "device_id", d.ID, "err", err)
			}
			return
		}
		if err := e.st.RefreshOfflineSince(ctx, d.ID); err != nil {
			e.log.Warn("offline_since", "device_id", d.ID, "err", err)
		}
	}()
	ignoreMap, err := e.st.GetPortEventIgnoreMap(ctx, d.ID)
	if err != nil {
		e.log.Warn("port ignore load", "device_id", d.ID, "err", err)
		ignoreMap = nil
	}

	pingOK, rttMs := pinghost.Probe(ctx, d.Host, 2*time.Second)
	if err := e.st.UpdateDevicePing(ctx, d.ID, pingOK, rttMs); err != nil {
		e.log.Warn("update ping", "device_id", d.ID, "err", err)
	}

	// Режим «Авто (Ping)»: SNMP у таких узлов нет (SIP-телефоны, IoT).
	// Не тратим таймауты на опрос и не пишем ложные ошибки SNMP.
	if models.NormalizeReachabilityMode(d.ReachabilityMode, d.OnlineOverride) == "ping" {
		if err := e.st.UpdateDevicePingPollMeta(ctx, d.ID); err != nil {
			e.log.Warn("poll meta (ping)", "device_id", d.ID, "err", err)
		}
		return nil
	}

	// Узел стабильно не отвечает по SNMP (нет агента: камеры, AP, телефоны) — пауза.
	// ICMP продолжает работать. Коммутаторы/роутеры/серверы в паузу не ставятся.
	if snmpBackoffEligible(d.DeviceCategory) && e.snmpBackoffActive(d.ID, time.Now()) {
		if err := e.st.UpdateDevicePollTime(ctx, d.ID); err != nil {
			e.log.Warn("poll meta (snmp backoff)", "device_id", d.ID, "err", err)
		}
		return nil
	}

	g, err := snmp.NewGoSNMP(d)
	if err != nil {
		msg := err.Error()
		_ = e.st.UpdateDevicePollMeta(ctx, d.ID, false, &msg, nil, nil, nil, nil, nil, nil)
		return err
	}
	// ICMP уже молчит: первая проба SNMP — короткая (2 с, без повторов), чтобы мёртвый
	// узел не висел в очереди опроса ~10 с и не растягивал цикл (в это окно устройство
	// может уйти в ребут и вернуться незамеченным). Ответил — вернём обычные таймауты.
	probeFast := !pingOK
	if probeFast {
		g.Timeout = snmpProbeFastTimeout
		g.Retries = snmpProbeFastRetries
	}
	if err := g.Connect(); err != nil {
		msg := err.Error()
		_ = e.st.UpdateDevicePollMeta(ctx, d.ID, false, &msg, nil, nil, nil, nil, nil, nil)
		if snmpBackoffEligible(d.DeviceCategory) {
			e.noteSNMPFailure(d.ID, time.Now())
		}
		return err
	}
	defer g.Conn.Close()

	sysName, sysDescr, err := snmp.SysGet(g)
	if err != nil {
		msg := err.Error()
		_ = e.st.UpdateDevicePollMeta(ctx, d.ID, false, &msg, nil, nil, nil, nil, nil, nil)
		if snmpBackoffEligible(d.DeviceCategory) {
			e.noteSNMPFailure(d.ID, time.Now())
		}
		return err
	}
	e.clearSNMPFailures(d.ID)
	if probeFast {
		// SNMP ответил (ICMP, видимо, фильтруется) — дальше обходы с обычными таймаутами.
		g.Timeout = snmpProbeNormalTimeout
		g.Retries = snmpProbeNormalRetries
	}

	var sysUptimeCs *int64
	if cs, upErr := snmp.SysUpTimeCentiseconds(g); upErr != nil {
		e.log.Debug("sys uptime", "device_id", d.ID, "err", upErr)
	} else {
		v := int64(cs)
		sysUptimeCs = &v
	}

	cat := ""
	if prevDev != nil {
		cat = prevDev.DeviceCategory
	}
	isPrinter := snmp.LooksLikePrinter(cat, sysDescr)
	isUPS := snmp.LooksLikeUPS(cat, sysDescr)

	var cpuProfileName string
	var cpuPct *float32
	var cpuProfile *string
	if isPrinter || isUPS {
		// У МФУ и ИБП CPU по SNMP нет / бессмысленен — не читаем и сбрасываем залипшие 100%.
		if err := e.st.ClearDeviceCPU(ctx, d.ID); err != nil {
			e.log.Debug("clear printer cpu", "device_id", d.ID, "err", err)
		}
	} else {
		cpuProfileName, cpuPct, _ = snmp.ReadCPU(g, sysDescr)
		if cpuProfileName != "" {
			cp := cpuProfileName
			cpuProfile = &cp
		}
	}

	var printer *snmp.PrinterReading
	var prevToners []models.PrinterToner
	if prevDev != nil {
		prevToners = prevDev.LastToners
	}
	if isPrinter {
		p, pErr := snmp.ReadPrinter(g, sysDescr)
		if pErr != nil {
			e.log.Debug("printer mib", "device_id", d.ID, "err", pErr)
		} else {
			printer = p
		}
	}
	if printer != nil && printer.HasData() {
		e.emitTonerThresholdAlerts(ctx, d, prevToners, printer)
		if err := e.st.UpdateDevicePrinterSnapshot(ctx, d.ID, printer.PageCount, printer.Toners); err != nil {
			e.log.Warn("printer snapshot", "device_id", d.ID, "err", err)
		}
	}

	// ИБП: только UPS-MIB. IF-MIB/FDB/LLDP/PoE/STP у ИБП нет — не ходим туда вовсе.
	if isUPS {
		e.pollUPS(ctx, d, prevDev, g, sysName, time.Now())
		sn, sd := sysName, sysDescr
		return e.st.UpdateDevicePollMeta(ctx, d.ID, true, nil, &sn, &sd, nil, nil, nil, sysUptimeCs)
	}

	// Тяжёлый опрос (LLDP/CDP/ARP/PoE/STP/FDB, роли портов) — раз в heavy_poll_interval_seconds.
	// Лёгкий (ping, sysUpTime, IF-MIB статусы/счётчики, CPU, ИБП/МФУ) — каждый интервал узла.
	now := time.Now()
	heavyDue := e.shouldPollHeavy(&d, now)

	var chassisMAC *string
	if heavyDue {
		if mac, macErr := snmp.ReadLocalChassisMAC(g, d.Host); macErr != nil {
			e.log.Debug("local chassis mac", "device_id", d.ID, "err", macErr)
		} else if mac != "" {
			chassisMAC = &mac
		}
	}

	sn, sd := sysName, sysDescr
	if err := e.st.UpdateDevicePollMeta(ctx, d.ID, true, nil, &sn, &sd, cpuProfile, cpuPct, chassisMAC, sysUptimeCs); err != nil {
		return err
	}

	if heavyDue {
		e.pollSTP(ctx, d, g)
	}

	ifRows, err := snmp.WalkIFMIB(g)
	if err != nil {
		msg := err.Error()
		_ = e.st.UpdateDevicePollMeta(ctx, d.ID, false, &msg, &sn, &sd, cpuProfile, cpuPct, chassisMAC, sysUptimeCs)
		return err
	}

	// Данные тяжёлого опроса: на лёгком опросе остаются nil, и UpsertInterfaces
	// сохраняет прежние poe_active/poe_power_w (COALESCE), а роль берётся из снимка.
	var poeByIf map[int]bool
	var poePowerByIf map[int]float32
	var lldpNeighbors []snmp.NeighborInfo
	var lldpErr error
	if heavyDue {
		poeActive, poeErr := snmp.WalkPoEDeliveringByIfIndex(g, ifRows)
		if poeErr != nil {
			e.log.Debug("poe mib", "device_id", d.ID, "err", poeErr)
		} else {
			poeByIf = poeActive
		}
		poePower, poePowerErr := snmp.WalkPoEPowerWByIfIndex(g, ifRows)
		if poePowerErr != nil {
			e.log.Debug("poe power mib", "device_id", d.ID, "err", poePowerErr)
		} else {
			poePowerByIf = poePower
		}
		lldp, lldpErrWalk := snmp.WalkLLDPNeighbors(g, ifRows)
		if lldpErrWalk != nil {
			e.log.Debug("lldp neighbors", "device_id", d.ID, "err", lldpErrWalk)
			lldpErr = lldpErrWalk
		} else {
			lldpNeighbors = lldp
		}
		sshGot := false
		if e.shouldTrySSHPoeFallback(sysDescr, poeByIf) {
			sshPoe, sshPower, sshErr := poecli.ReadPoEActiveAndPowerByIfIndex(
				d.Host,
				e.cfg.SSHPOEPort,
				e.cfg.SSHPOEUser,
				e.cfg.SSHPOEPass,
				e.cfg.SSHPOEEnablePass,
				sysDescr,
				ifRows,
				e.cfg.SSHPOETimeout,
				e.cfg.SSHPOEKnownHosts,
			)
			if sshErr != nil {
				e.log.Warn("poe ssh fallback", "device_id", d.ID, "host", d.Host, "err", sshErr)
			} else if len(sshPoe) > 0 || len(sshPower) > 0 {
				sshGot = len(sshPoe) > 0
				if len(sshPoe) > 0 {
					if poeByIf == nil {
						poeByIf = make(map[int]bool)
					}
					mergePoeMap(poeByIf, sshPoe)
				}
				// SNMP watts приоритетнее; SSH заполняет дыры (EdgeSwitch *-lite* без 4413).
				poePowerByIf = mergePoePowerMap(poePowerByIf, sshPower)
			}
		}
		// LLDP-PD только если PSE-MIB и SSH ничего не сказали (карта пустая/nil).
		// Не подмешиваем, когда SNMP уже дал явные false — иначе «сосед PD» снова станет ложной молнией.
		if !sshGot && len(poeByIf) == 0 {
			if m, lerr := snmp.WalkLLDPDot3RemotePDByIfIndex(g, ifRows); lerr == nil && len(m) > 0 {
				snmp.FilterLldpPdInfraUplinks(m, lldpNeighbors)
				if poeByIf == nil {
					poeByIf = make(map[int]bool)
				}
				mergePoeMap(poeByIf, m)
			}
		}
		// SFP/оптика никогда не PSE: сбрасываем ложные молнии (в т.ч. залипший poe_active в БД).
		if poeByIf == nil {
			poeByIf = make(map[int]bool)
		}
		snmp.ClearPoEOnFiberPorts(poeByIf, ifRows, sysDescr)
	}

	pollFDBNow := heavyDue && e.shouldPollFDB(d, d.LastFDBPollAt, now)
	if heavyDue {
		e.maybeSyncPortRolesFromConfig(ctx, d, pollFDBNow)
	}

	prev, err := e.st.ListInterfaceSnapshots(ctx, d.ID)
	if err != nil {
		return err
	}

	var curFDBEntries map[string]snmp.FDBLearnedEntry
	var fdbStats map[int]snmp.FDBIfStats
	if pollFDBNow {
		curFDBEntries, fdbStats, err = snmp.WalkFDBWithStats(g)
		if err != nil {
			e.log.Warn("poll fdb", "device_id", d.ID, "err", err)
			_ = e.st.UpdateDeviceFDBStatus(ctx, d.ID, "unavailable")
			// Отмечаем попытку: у узлов без bridge-MIB иначе FDB пробовалась бы на каждом
			// тяжёлом опросе (журнал «bridge port map empty» каждые 5 минут).
			if merr := e.st.MarkDeviceFDBPollAttempt(ctx, d.ID); merr != nil {
				e.log.Debug("fdb attempt mark", "device_id", d.ID, "err", merr)
			}
			pollFDBNow = false
		}
	} else if e.cfg.FDBPollInterval <= 0 {
		_ = e.st.UpdateDeviceFDBStatus(ctx, d.ID, "unavailable")
	}
	var upserts []store.InterfaceUpsert
	var stormHighUtil []stormUtilPort
	rateByIf := make(map[int][2]float32) // in_bps, out_bps

	infraByIf := make(map[int]bool)
	for _, n := range lldpNeighbors {
		if n.IfIndex > 0 && snmp.IsInfraLLDPNeighbor(n) {
			infraByIf[n.IfIndex] = true
		}
	}
	effectiveRoleByIf := make(map[int]string)

	for idx, row := range ifRows {
		if idx <= 0 {
			continue
		}
		ifIdx := idx
		old := prev[idx]
		// Для роли берём ifAlias, если есть (на EdgeSwitch описание порта обычно там).
		roleDescr := row.IfDescr
		if alias := strings.TrimSpace(row.IfAlias); alias != "" {
			roleDescr = alias
		}
		// operational role для алертов/UI — не полный mirror CLI switchport mode.
		// На лёгком опросе не пересчитываем: роль зависит от LLDP/FDB, которых в лёгком
		// опросе нет — иначе она мигала бы между значениями.
		portRole := inferRoleFromSnapshot(old)
		if heavyDue && portRole == "access" {
			portRole = inferPortRole(row.IfName, roleDescr, row.IfType, old)
			if portRole == "access" && infraByIf[ifIdx] {
				portRole = "trunk"
			}
			if portRole == "access" && isLikelyTrunkByFDB(fdbStats[ifIdx], e.cfg) {
				portRole = "trunk"
			}
		}
		effectiveRoleByIf[ifIdx] = portRole

		newAdmin := row.Admin
		newOper := row.Oper
		if old.OperStatus != nil {
			if *old.OperStatus != 1 && newOper == 1 {
				pl := ifaceEventPayload(row, old)
				if !e.annotateLinkFromTrap(ctx, d.ID, ifIdx, 1, "LINK_UP", pl) {
					e.emit(ctx, ignoreMap, d, &ifIdx, "LINK_UP", "info", pl)
				}
				e.maybeEmitPortFlap(ctx, ignoreMap, d, ifIdx, prev, time.Now())
			}
			if newAdmin == 1 && *old.OperStatus == 1 && newOper != 1 {
				pl := ifaceEventPayload(row, old)
				pl["oper_status"] = newOper
				if !e.annotateLinkFromTrap(ctx, d.ID, ifIdx, 2, "LINK_DOWN", pl) {
					e.emit(ctx, ignoreMap, d, &ifIdx, "LINK_DOWN", "warning", pl)
				}
				e.maybeEmitPortFlap(ctx, ignoreMap, d, ifIdx, prev, time.Now())
			}
		}

		// Падение/рост номинальной скорости линка (как ifHighSpeed/ifSpeed), пока порт up.
		// ifHighSpeed=0 при down не считаем деградацией — это LINK_DOWN.
		if newOper == 1 {
			oldBps := snapshotBitsPerSecond(old)
			newBps := row.BitsPerSecond()
			if oldBps > 0 && newBps > 0 && oldBps != newBps {
				pl := ifaceEventPayload(row, old)
				pl["old_bps"] = oldBps
				pl["new_bps"] = newBps
				pl["old_mbps"] = oldBps / 1_000_000
				pl["new_mbps"] = newBps / 1_000_000
				if newBps < oldBps {
					e.emit(ctx, ignoreMap, d, &ifIdx, "PORT_SPEED_DOWN", "warning", pl)
				} else {
					e.emit(ctx, ignoreMap, d, &ifIdx, "PORT_SPEED_OK", "info", pl)
				}
			}
		}

		var utilIn, utilOut, utilMax *float32
		utilHigh := old.UtilHighActive
		bps := row.BitsPerSecond()

		if bps > 0 && old.HCInOctets != nil && old.HCOutOctets != nil && old.CountersPolledAt != nil &&
			row.HCIn >= 0 && row.HCOut >= 0 {
			dt := now.Sub(*old.CountersPolledAt).Seconds()
			if dt >= 2 {
				din := row.HCIn - *old.HCInOctets
				dout := row.HCOut - *old.HCOutOctets
				if din >= 0 && dout >= 0 {
					rIn := float64(din) * 8 / dt
					rOut := float64(dout) * 8 / dt
					rateByIf[ifIdx] = [2]float32{float32(rIn), float32(rOut)}
					ui := float32(rIn / float64(bps) * 100)
					uo := float32(rOut / float64(bps) * 100)
					if ui > 100 {
						ui = 100
					}
					if uo > 100 {
						uo = 100
					}
					utilIn = &ui
					utilOut = &uo
					um := ui
					if uo > um {
						um = uo
					}
					utilMax = &um
					umf := float64(um)

					if newOper == 1 && umf >= e.cfg.BroadcastStormUtilPct {
						desc := row.IfDescr
						if alias := strings.TrimSpace(row.IfAlias); alias != "" {
							desc = alias
						}
						stormHighUtil = append(stormHighUtil, stormUtilPort{
							IfIndex: ifIdx,
							UtilMax: um,
							UtilIn:  ui,
							UtilOut: uo,
							IfDescr: desc,
						})
					}

					th := store.ResolveUtilThresholds(e.cfg, &d, &old)
					if umf >= th.HighPct {
						if !utilHigh {
							pl := map[string]interface{}{
								"util_in_pct":   ui,
								"util_out_pct":  uo,
								"util_max_pct":  um,
								"threshold_pct": th.HighPct,
								"bps_nominal":   bps,
							}
							for k, v := range ifaceEventPayload(row, old) {
								pl[k] = v
							}
							e.emit(ctx, ignoreMap, d, &ifIdx, "PORT_UTILIZATION_HIGH", "warning", pl)
						}
						utilHigh = true
					} else if umf < th.OkPct {
						if utilHigh {
							pl := map[string]interface{}{
								"util_max_pct": umf,
								"ok_below_pct": th.OkPct,
							}
							for k, v := range ifaceEventPayload(row, old) {
								pl[k] = v
							}
							e.emit(ctx, ignoreMap, d, &ifIdx, "PORT_UTILIZATION_OK", "info", pl)
						}
						utilHigh = false
					}
				}
			}
		}

		desc := snmp.HumanizeSNMPText(row.IfDescr)
		if alias := strings.TrimSpace(row.IfAlias); alias != "" {
			// Для Ubiquiti и ряда других вендоров ifAlias — это человекочитаемое имя порта.
			desc = snmp.HumanizeSNMPText(alias)
		}
		name := row.IfName
		typ := row.IfType
		ad := newAdmin
		op := newOper
		sp := row.IfSpeed
		hs := row.HighSpeed
		hi := row.HCIn
		ho := row.HCOut
		tNow := now

		var poePtr *bool
		if poeByIf != nil {
			if v, ok := poeByIf[ifIdx]; ok {
				vv := v
				poePtr = &vv
			}
		}
		var poePowerPtr *float32
		if poePowerByIf != nil {
			if v, ok := poePowerByIf[ifIdx]; ok {
				vv := v
				poePowerPtr = &vv
			}
		}

		upserts = append(upserts, store.InterfaceUpsert{
			IfIndex:          idx,
			IfDescr:          &desc,
			IfName:           &name,
			IfType:           &typ,
			AdminStatus:      &ad,
			OperStatus:       &op,
			IfSpeed:          &sp,
			IfHighSpeed:      &hs,
			PortRole:         &portRole,
			HCInOctets:       &hi,
			HCOutOctets:      &ho,
			CountersPolledAt: &tNow,
			UtilInPct:        utilIn,
			UtilOutPct:       utilOut,
			UtilMaxPct:       utilMax,
			UtilHighActive:   utilHigh,
			PoeActive:        poePtr,
			PoePowerW:        poePowerPtr,
		})
	}

	if err := e.st.UpsertInterfaces(ctx, d.ID, upserts); err != nil {
		return err
	}
	// Соседи (LLDP/CDP) и ARP — тяжёлый опрос: таблицы большие, не чаще heavy_poll_interval_seconds.
	if heavyDue {
		e.persistNeighbors(ctx, d, "lldp", now, func() ([]snmp.NeighborInfo, error) {
			if lldpErr != nil {
				return nil, lldpErr
			}
			return lldpNeighbors, nil
		})
		e.persistNeighbors(ctx, d, "cdp", now, func() ([]snmp.NeighborInfo, error) {
			return snmp.WalkCDPNeighbors(g)
		})
	}
	e.recordMetrics(ctx, d.ID, cpuPct, upserts, rateByIf, now, printer)
	if heavyDue {
		if arpEntries, err := snmp.WalkARP(g); err != nil {
			e.log.Warn("poll arp", "device_id", d.ID, "err", err)
		} else {
			storeARP := make([]store.ARPEntry, 0, len(arpEntries))
			for _, a := range arpEntries {
				storeARP = append(storeARP, store.ARPEntry{IP: a.IP, MAC: a.MAC, IfIndex: a.IfIndex})
			}
			if err := e.st.ReplaceARPSnapshot(ctx, d.ID, storeARP, now); err != nil {
				e.log.Warn("arp snapshot write", "device_id", d.ID, "err", err)
			} else if n, err := e.st.BackfillEmptyChassisFromARP(ctx, storeARP); err != nil {
				e.log.Debug("chassis backfill from arp", "device_id", d.ID, "err", err)
			} else if n > 0 {
				e.log.Info("chassis_mac backfilled from arp", "from_device_id", d.ID, "count", n)
				e.notifyTopologyDirty("arp-chassis")
			}
		}
	}
	if pollFDBNow {
		if len(curFDBEntries) > 0 {
			_ = e.st.UpdateDeviceFDBStatus(ctx, d.ID, "ok")
		} else {
			_ = e.st.UpdateDeviceFDBStatus(ctx, d.ID, "learning")
		}
		e.pollFDB(ctx, ignoreMap, d, now, prev, effectiveRoleByIf, curFDBEntries, fdbStats, stormHighUtil)
	} else if e.isFDBLearning(d, now) {
		_ = e.st.UpdateDeviceFDBStatus(ctx, d.ID, "learning")
	}
	if heavyDue {
		if err := e.st.UpdateDeviceHeavyPollAt(ctx, d.ID); err != nil {
			e.log.Warn("heavy poll mark", "device_id", d.ID, "err", err)
		}
	}
	return nil
}

// utilSampleDue — пора ли писать util/rate сэмплы (троттлинг записи истории).
func (e *Engine) utilSampleDue(deviceID int64, now time.Time) bool {
	interval := e.cfg.MetricsUtilInterval
	if interval <= 0 {
		return true
	}
	if v, ok := e.utilSampleAt.Load(deviceID); ok {
		if last, okT := v.(time.Time); okT && now.Sub(last) < interval {
			return false
		}
	}
	e.utilSampleAt.Store(deviceID, now)
	return true
}

func (e *Engine) recordMetrics(ctx context.Context, deviceID int64, cpuPct *float32, upserts []store.InterfaceUpsert, rates map[int][2]float32, at time.Time, printer *snmp.PrinterReading) {
	if !e.cfg.MetricsEnabled {
		return
	}
	// util/rate пишем не чаще MetricsUtilInterval (по умолчанию 60 с): сам опрос идёт
	// каждые 15–20 с, и без троттлинга это ~19 млн строк в сутки вместо ~4 млн.
	writeUtil := e.utilSampleDue(deviceID, at)
	var samples []store.MetricSample
	if cpuPct != nil {
		samples = append(samples, store.MetricSample{MetricType: "cpu_pct", Value: *cpuPct, SampledAt: at})
	}
	if printer != nil {
		if printer.PageCount != nil {
			samples = append(samples, store.MetricSample{MetricType: "page_count", Value: float32(*printer.PageCount), SampledAt: at})
		}
		seen := map[string]bool{}
		for _, t := range printer.Toners {
			if t.Pct == nil {
				continue
			}
			mt := t.MetricType
			if mt == "" {
				mt = snmp.TonerMetricType(t.Key)
			}
			if seen[mt] {
				continue
			}
			seen[mt] = true
			samples = append(samples, store.MetricSample{MetricType: mt, Value: *t.Pct, SampledAt: at})
		}
	}
	for _, u := range upserts {
		if !writeUtil {
			break
		}
		if u.OperStatus == nil || *u.OperStatus != 1 {
			continue
		}
		idx := u.IfIndex
		if u.UtilMaxPct != nil {
			v := *u.UtilMaxPct
			samples = append(samples, store.MetricSample{
				IfIndex: &idx, MetricType: "util_max_pct", Value: v, SampledAt: at,
			})
		}
		if u.UtilInPct != nil {
			v := *u.UtilInPct
			samples = append(samples, store.MetricSample{
				IfIndex: &idx, MetricType: "util_in_pct", Value: v, SampledAt: at,
			})
		}
		if u.UtilOutPct != nil {
			v := *u.UtilOutPct
			samples = append(samples, store.MetricSample{
				IfIndex: &idx, MetricType: "util_out_pct", Value: v, SampledAt: at,
			})
		}
		if rates != nil {
			if r, ok := rates[u.IfIndex]; ok {
				in := r[0]
				out := r[1]
				samples = append(samples,
					store.MetricSample{IfIndex: &idx, MetricType: "rate_in_bps", Value: in, SampledAt: at},
					store.MetricSample{IfIndex: &idx, MetricType: "rate_out_bps", Value: out, SampledAt: at},
				)
			}
		}
	}
	if len(samples) > 0 {
		if err := e.st.InsertMetricSamplesBatch(ctx, deviceID, samples); err != nil {
			e.log.Warn("metric samples", "device_id", deviceID, "err", err)
		}
	}
}

func (e *Engine) maybeIncidentAction(ctx context.Context, dev store.PollDevice, ifIdx *int, typ string) {
	if ifIdx == nil {
		return
	}
	ns, err := e.st.GetNotificationSettings(ctx)
	if err != nil || !ns.IncidentActionEnabled {
		return
	}
	if !actionMatchesFilter(typ, ns.IncidentActionEventTypes) {
		return
	}
	okCD, err := e.st.IncidentActionInCooldown(ctx, dev.ID, *ifIdx, "admin_down", ns.IncidentActionCooldownSeconds)
	if err != nil {
		e.log.Warn("incident cooldown", "device_id", dev.ID, "err", err)
		return
	}
	if okCD {
		e.log.Debug("incident action skipped (cooldown)", "device_id", dev.ID, "if_index", *ifIdx)
		return
	}
	if ns.IncidentActionDryRun {
		pl := map[string]interface{}{"if_index": *ifIdx, "action": "admin_down", "dry_run": true}
		if _, err := e.st.InsertEvent(ctx, dev.ID, ifIdx, "PORT_ADMIN_DOWN_ACTION", "info", pl); err != nil {
			e.log.Warn("incident dry-run event", "err", err)
		}
		if err := e.st.TouchIncidentActionCooldown(ctx, dev.ID, *ifIdx, "admin_down"); err != nil {
			e.log.Warn("incident cooldown touch", "err", err)
		}
		e.log.Info("incident action dry-run", "device_id", dev.ID, "if_index", *ifIdx)
		return
	}
	if err := actions.TryPortAdminDown(ctx, e.log, e.st, dev, *ifIdx); err != nil {
		e.log.Warn("incident action", "device_id", dev.ID, "if_index", *ifIdx, "err", err)
		return
	}
	if err := e.st.TouchIncidentActionCooldown(ctx, dev.ID, *ifIdx, "admin_down"); err != nil {
		e.log.Warn("incident cooldown touch", "err", err)
	}
}

func (e *Engine) shouldTrySSHPoeFallback(sysDescr string, poeByIf map[int]bool) bool {
	if !e.cfg.SSHPOEFallbackEnabled {
		return false
	}
	if strings.TrimSpace(e.cfg.SSHPOEUser) == "" {
		return false
	}
	// Fallback только если SNMP не подтвердил ни одной активной выдачи.
	// len(map)>0 недостаточно: SNR/RFC могут заполнить карту false-ами.
	if poeMapHasActive(poeByIf) {
		return false
	}
	s := strings.ToLower(sysDescr)
	return strings.Contains(s, "edgeswitch") ||
		strings.Contains(s, "ubnt") ||
		strings.Contains(s, "snr")
}

func poeMapHasActive(poeByIf map[int]bool) bool {
	for _, v := range poeByIf {
		if v {
			return true
		}
	}
	return false
}

func mergePoeMap(dst, src map[int]bool) {
	if dst == nil || src == nil {
		return
	}
	for k, v := range src {
		if v {
			dst[k] = true
			continue
		}
		if _, exists := dst[k]; !exists {
			dst[k] = false
		}
	}
}

// mergePoePowerMap — SNMP имеет приоритет; SSH/CLI дополняет отсутствующие ifIndex.
func mergePoePowerMap(dst, src map[int]float32) map[int]float32 {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[int]float32, len(src))
	}
	for k, v := range src {
		if _, ok := dst[k]; ok {
			continue
		}
		dst[k] = v
	}
	return dst
}

func (e *Engine) shouldPollFDB(d store.PollDevice, last *time.Time, now time.Time) bool {
	interval := e.cfg.FDBPollInterval
	if d.FDBPollIntervalSeconds != nil && *d.FDBPollIntervalSeconds > 0 {
		interval = time.Duration(*d.FDBPollIntervalSeconds) * time.Second
	}
	if interval <= 0 {
		return false
	}
	if last == nil {
		return true
	}
	return now.Sub(*last) >= interval
}

func neighborEmptyKey(deviceID int64, protocol string) string {
	return strconv.FormatInt(deviceID, 10) + ":" + strings.ToLower(strings.TrimSpace(protocol))
}

func (e *Engine) bumpNeighborEmptyStreak(deviceID int64, protocol string) int {
	key := neighborEmptyKey(deviceID, protocol)
	n := 1
	if v, ok := e.neighborEmptyStreak.Load(key); ok {
		n = v.(int) + 1
	}
	e.neighborEmptyStreak.Store(key, n)
	return n
}

func (e *Engine) clearNeighborEmptyStreak(deviceID int64, protocol string) {
	e.neighborEmptyStreak.Delete(neighborEmptyKey(deviceID, protocol))
}

func (e *Engine) persistNeighbors(ctx context.Context, d store.PollDevice, protocol string, now time.Time, walk func() ([]snmp.NeighborInfo, error)) {
	neighbors, err := walk()
	if err != nil {
		e.clearNeighborEmptyStreak(d.ID, protocol)
		e.log.Warn(protocol+" neighbors walk", "device_id", d.ID, "err", err)
		return
	}
	if len(neighbors) == 0 {
		streak := e.bumpNeighborEmptyStreak(d.ID, protocol)
		if streak < neighborEmptyConfirmPolls {
			e.log.Debug(protocol+" neighbors empty (confirm pending)",
				"device_id", d.ID, "host", d.Host, "streak", streak, "need", neighborEmptyConfirmPolls)
			return
		}
	} else {
		e.clearNeighborEmptyStreak(d.ID, protocol)
	}
	rows := make([]store.PortNeighbor, 0, len(neighbors))
	var chassisIdx map[string]store.ChassisEndpoint
	if protocol == "lldp" || protocol == "cdp" {
		if idx, err := e.st.ListChassisMACIndex(ctx); err == nil {
			chassisIdx = idx
		}
	}
	for _, n := range neighbors {
		sys := n.RemoteSysName
		// Windows LLDP часто без sysName — подставить имя из inventory по chassis MAC.
		if strings.TrimSpace(sys) == "" && chassisIdx != nil {
			if mac, ok := store.FormatFullMAC(n.RemoteChassisID); ok {
				hex := strings.ToLower(strings.ReplaceAll(mac, ":", ""))
				if ep, ok := chassisIdx[hex]; ok {
					if name := strings.TrimSpace(ep.Name); name != "" {
						sys = name
					}
				}
			}
		}
		rows = append(rows, store.PortNeighbor{
			DeviceID:        d.ID,
			IfIndex:         n.IfIndex,
			RemIndex:        n.RemIndex,
			Protocol:        protocol,
			RemoteSysName:   strPtr(sys),
			RemotePortID:    strPtr(n.RemotePortID),
			RemoteChassisID: strPtr(n.RemoteChassisID),
			RemoteMgmtAddr:  strPtr(n.RemoteMgmtAddr),
		})
	}
	if err := e.st.UpsertPortNeighbors(ctx, d.ID, protocol, rows, now); err != nil {
		e.log.Warn(protocol+" neighbors write", "device_id", d.ID, "err", err)
	} else {
		// Active manual links are sticky — do not auto-supersede on LLDP/CDP
		// (wrong neighbors on MikroTik VLAN/SFP must not overwrite a user-pinned edge).
		if len(neighbors) == 0 {
			e.log.Debug(protocol+" neighbors empty", "device_id", d.ID, "host", d.Host)
		} else if err := e.st.SyncDiscoveredFromNeighbors(ctx, d.ID, rows); err != nil {
			e.log.Warn("discovered sync", "device_id", d.ID, "protocol", protocol, "err", err)
		}
		if protocol == "lldp" || protocol == "cdp" {
			e.notifyTopologyDirty(protocol)
		}
	}
}

func strPtr(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func snmpFDBToStore(entries map[string]snmp.FDBLearnedEntry) map[string]store.FDBLearnedEntry {
	out := make(map[string]store.FDBLearnedEntry, len(entries))
	for mac, ent := range entries {
		out[mac] = store.FDBLearnedEntry{IfIndex: ent.IfIndex, VLANID: ent.VLANID}
	}
	return out
}

func (e *Engine) pollFDB(ctx context.Context, ignore map[int]store.PortEventIgnore, d store.PollDevice, now time.Time, ifs map[int]store.InterfaceSnapshot, effectiveRoleByIf map[int]string, entries map[string]snmp.FDBLearnedEntry, fdbStats map[int]snmp.FDBIfStats, stormHighUtil []stormUtilPort) {
	cur := snmp.FDBEntriesToIfIndexMap(entries)
	prev, err := e.st.GetFDBSnapshot(ctx, d.ID)
	if err != nil {
		e.log.Warn("fdb snapshot read", "device_id", d.ID, "err", err)
		return
	}
	learning := e.isFDBLearning(d, now)
	resolveRole := makeResolvePortRole(ifs, effectiveRoleByIf, fdbStats, e.cfg)
	chassis, chErr := e.st.ListChassisMACIndex(ctx)
	if chErr != nil {
		e.log.Warn("fdb inventory mac index", "device_id", d.ID, "err", chErr)
		chassis = map[string]store.ChassisEndpoint{}
	}
	secState, secErr := e.st.GetPortAccessSecurity(ctx, d.ID)
	if secErr != nil {
		e.log.Warn("port access security read", "device_id", d.ID, "err", secErr)
		secState = map[int]store.PortAccessSecurity{}
	}
	isInventoryMAC := func(mac string) bool {
		return isInventoryChassisMAC(mac, chassis)
	}
	isKnownOnPort := func(mac string, ifIndex int) bool {
		st, ok := secState[ifIndex]
		if !ok || st.BoundMAC == nil {
			return false
		}
		return normalizeMAC(*st.BoundMAC) == normalizeMAC(mac)
	}
	// На первом снимке только сохраняем baseline без событий.
	if len(prev) > 0 && !learning {
		e.maybeDetectBroadcastStorm(ctx, ignore, d, ifs, len(prev), len(entries), stormHighUtil, now)
		for _, ev := range computeFDBAccessEvents(prev, cur, resolveRole, isInventoryMAC, isKnownOnPort) {
			if !e.shouldTrackMAC(ctx, ev.mac) {
				continue
			}
			pl := map[string]interface{}{"mac": ev.mac}
			if ev.oldIfIndex != nil {
				pl["old_if_index"] = *ev.oldIfIndex
				pl["new_if_index"] = ev.ifIndex
			}
			mergeIfacePayload(pl, ifs, ev.ifIndex)
			idx := ev.ifIndex
			e.emit(ctx, ignore, d, &idx, ev.eventType, ev.severity, pl)
		}
		e.recordAndDetectMACMoves(ctx, ignore, d, prev, cur, entries, ifs, now)
	}
	if len(prev) > 0 && !learning {
		if err := e.processAccessPortSecurity(ctx, ignore, d, prev, cur, ifs, effectiveRoleByIf, now); err != nil {
			e.log.Warn("access port security", "device_id", d.ID, "err", err)
		}
	}
	storeEntries := snmpFDBToStore(entries)
	if err := e.st.ReplaceFDBSnapshot(ctx, d.ID, storeEntries, now); err != nil {
		e.log.Warn("fdb snapshot write", "device_id", d.ID, "err", err)
	} else if !learning && len(entries) > 0 {
		if err := fdbsnapshot.MaybeSaveDaily(ctx, e.log, e.st, e.cfg, d.ID, storeEntries, now); err != nil {
			e.log.Warn("fdb daily snapshot", "device_id", d.ID, "err", err)
		}
	}
	e.syncFDBTopologyLinks(ctx, d, entries, ifs, fdbStats, chassis, now)
}

func (e *Engine) syncFDBTopologyLinks(
	ctx context.Context,
	d store.PollDevice,
	entries map[string]snmp.FDBLearnedEntry,
	ifs map[int]store.InterfaceSnapshot,
	fdbStats map[int]snmp.FDBIfStats,
	chassis map[string]store.ChassisEndpoint,
	now time.Time,
) {
	storeEntries := snmpFDBToStore(entries)
	portMACCount := make(map[int]int, len(fdbStats))
	for ifIndex, st := range fdbStats {
		portMACCount[ifIndex] = st.MACCount
	}
	// если stats пусты — посчитаем из снимка
	if len(portMACCount) == 0 {
		for _, ent := range storeEntries {
			portMACCount[ent.IfIndex]++
		}
	}
	portRole := store.PortRolesForFDBTopology(ifs)
	if len(chassis) > 0 {
		n, err := e.st.SyncFDBTopologyNeighbors(ctx, d.ID, storeEntries, portMACCount, portRole, chassis, now)
		if err != nil {
			e.log.Warn("fdb topology sync", "device_id", d.ID, "err", err)
		} else if n > 0 {
			e.log.Debug("fdb topology sync", "device_id", d.ID, "linked", n)
		}
	}
	// Offline ПК: карточка «Подключен к…» из attach_*, а port_neighbors мог так и не появиться.
	if m, err := e.st.EnsureFDBNeighborsFromAttachments(ctx, d.ID, now); err != nil {
		e.log.Warn("fdb topology ensure attach", "device_id", d.ID, "err", err)
	} else if m > 0 {
		e.log.Debug("fdb topology ensure attach", "device_id", d.ID, "linked", m)
	}
}

func (e *Engine) isFDBLearning(d store.PollDevice, now time.Time) bool {
	if e.cfg.FDBLearnDuration <= 0 {
		return false
	}
	if d.FDBBaselineAt == nil {
		return true
	}
	return now.Sub(*d.FDBBaselineAt) < e.cfg.FDBLearnDuration
}

func inferPortRole(ifName, ifDescr string, ifType int64, prev store.InterfaceSnapshot) string {
	nameLower := strings.ToLower(strings.TrimSpace(ifName))
	joined := strings.ToLower(strings.TrimSpace(ifName + " " + ifDescr))
	// VLAN/loopback/tunnel/etc — только по ifType и имени интерфейса.
	// В ifDescr часто пишут «ROOM-VLAN162-…» для обычного Ethernet — это не виртуальный VLAN-if.
	if ifType == 24 || ifType == 53 || ifType == 131 || ifType == 135 || ifType == 136 {
		return "ignore"
	}
	if strings.HasPrefix(nameLower, "vlan") || strings.Contains(nameLower, "loopback") {
		return "ignore"
	}
	// Uplink/trunk-like names and descriptions (operational role for alerts/UI; not CLI switchport mode).
	if strings.Contains(joined, "trunk") || strings.Contains(joined, "uplink") || strings.Contains(joined, "lag") || strings.Contains(joined, "port-channel") || strings.Contains(joined, "bond") {
		return "trunk"
	}
	if looksLikeUplinkDescr(joined) {
		return "trunk"
	}
	// If nothing obvious, consider physical ports as access by default.
	if ifType == 6 || ifType == 117 || ifType == 62 || ifType == 69 {
		return "access"
	}
	// Keep previous inference if we have one through names in snapshot.
	if prev.IfName != nil || prev.IfDescr != nil {
		r := inferPortRole(strOr(prev.IfName), strOr(prev.IfDescr), ifType, store.InterfaceSnapshot{})
		if r != "" {
			return r
		}
	}
	return "access"
}

// looksLikeUplinkDescr: описания вида «to-…», «TO EdgeSwitch», serverroom и т.п.
func looksLikeUplinkDescr(joinedLower string) bool {
	if joinedLower == "" {
		return false
	}
	if strings.Contains(joinedLower, "to-") || strings.HasPrefix(joinedLower, "to ") {
		return true
	}
	for _, kw := range []string{
		"edgeswitch", "eltex", "snr-", "snr ", "mes2", "mes-", "serverroom", "server-room", "server room",
		"backbone", "core switch", "to server", "to-es", "to es",
	} {
		if strings.Contains(joinedLower, kw) {
			return true
		}
	}
	// «port N: to Something Switch» без дефиса
	if strings.Contains(joinedLower, " to ") {
		for _, kw := range []string{"switch", "es-", "es ", "router", "snr", "eltex", "ubnt"} {
			if strings.Contains(joinedLower, kw) {
				return true
			}
		}
	}
	return false
}

func inferRoleFromSnapshot(s store.InterfaceSnapshot) string {
	if s.CLIPortMode != nil {
		switch strings.ToLower(strings.TrimSpace(*s.CLIPortMode)) {
		case "trunk":
			return "trunk"
		case "access", "general":
			// general = EdgeSwitch «No VLAN» → operational access
			return "access"
		}
	}
	pr := strings.ToLower(strings.TrimSpace(s.PortRole))
	if pr == "ignore" {
		return "ignore"
	}
	// Без cli_port_mode не держим залипший trunk после очистки show run (нет switchport в блоке).
	return inferPortRole(strOr(s.IfName), strOr(s.IfDescr), 0, store.InterfaceSnapshot{})
}

func makeResolvePortRole(
	ifs map[int]store.InterfaceSnapshot,
	effectiveRoleByIf map[int]string,
	fdbStats map[int]snmp.FDBIfStats,
	cfg config.Config,
) func(int) string {
	return func(ifIndex int) string {
		if effectiveRoleByIf != nil {
			if role, ok := effectiveRoleByIf[ifIndex]; ok && strings.TrimSpace(role) != "" {
				return role
			}
		}
		snap, ok := ifs[ifIndex]
		if !ok {
			return "access"
		}
		role := inferRoleFromSnapshot(snap)
		if role == "access" && isLikelyTrunkByFDB(fdbStats[ifIndex], cfg) {
			return "trunk"
		}
		return role
	}
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func isUnicastMAC(mac string) bool {
	hex := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(mac)), ":", "")
	if len(hex) < 2 {
		return false
	}
	first, err := strconv.ParseUint(hex[:2], 16, 8)
	if err != nil {
		return false
	}
	// multicast/broadcast if LSB=1 in first octet.
	return byte(first)&1 == 0
}

func actionMatchesFilter(value string, filter *string) bool {
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

func isLikelyTrunkByFDB(s snmp.FDBIfStats, cfg config.Config) bool {
	if s.MACCount <= 0 {
		return false
	}
	// Основной путь: есть VLAN-данные из Q-BRIDGE-MIB.
	if s.VLANCount > 0 {
		if s.MACCount < cfg.FDBAutoTrunkMinMACs {
			return false
		}
		return s.VLANCount >= cfg.FDBAutoTrunkMinVLANs
	}
	// Фолбэк: для моделей без dot1qTpFdbPort используем повышенный порог по числу MAC.
	return s.MACCount >= cfg.FDBAutoTrunkFallbackMinMACs
}

const (
	portRoleSyncInterval     = 6 * time.Hour
	portRoleSyncBeforeFDBAge = 30 * time.Minute
)

// maybeSyncPortRolesFromConfig подтягивает switchport mode из show run до FDB-событий (trunk не даёт MAC-алертов).
func (e *Engine) maybeSyncPortRolesFromConfig(ctx context.Context, d store.PollDevice, pollFDBNow bool) {
	dev, err := e.st.GetDevice(ctx, d.ID)
	if err != nil || dev == nil {
		return
	}
	if !portrolesync.ShouldSyncPortRolesFromConfig(dev) {
		return
	}
	syncedAt, err := e.st.GetDeviceCLIModeSyncAt(ctx, d.ID)
	if err != nil {
		e.log.Debug("port roles sync at", "device_id", d.ID, "err", err)
		return
	}
	maxAge := portRoleSyncInterval
	if pollFDBNow {
		maxAge = portRoleSyncBeforeFDBAge
	}
	if syncedAt != nil && time.Since(*syncedAt) < maxAge {
		return
	}
	bs, err := e.st.GetBackupSettings(ctx)
	if err != nil {
		e.log.Debug("port roles backup settings", "device_id", d.ID, "err", err)
		return
	}
	if res, err := portrolesync.SyncDevicePortRoles(ctx, e.st, e.cfg, bs, dev, nil, portrolesync.SyncOpts{
		Force:  false,
		MaxAge: portRoleSyncBeforeFDBAge,
	}); err != nil {
		e.log.Debug("port roles from config", "device_id", d.ID, "err", err)
		return
	} else if res.Total() > 0 {
		e.log.Debug("port roles from config", "device_id", d.ID, "roles", res.Roles, "descriptions", res.Descriptions)
	}
}
