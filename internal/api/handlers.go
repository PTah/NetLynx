package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/netutil"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/notify"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/uisp"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "ok",
		"version":    s.bi.Version,
		"commit":     s.bi.Commit,
		"built_at":   s.bi.BuiltAt,
		"started_at": startedAt.UTC().Format(time.RFC3339),
	})
}

func isSNMPCommunityMode(v string) bool {
	return v == "v1" || v == "v2c"
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	redactDevicesForAPI(list)
	writeJSON(w, http.StatusOK, list)
}

type createDeviceBody struct {
	Name                string  `json:"name"`
	Host                string  `json:"host"`
	Location            *string `json:"location"`
	DeviceCategory      string  `json:"device_category"`
	SNMPVersion         string  `json:"snmp_version"`
	Community           *string `json:"community"`
	V3User              *string `json:"v3_user"`
	V3AuthProtocol      *string `json:"v3_auth_protocol"`
	V3AuthPass          *string `json:"v3_auth_pass"`
	V3PrivProtocol      *string `json:"v3_priv_protocol"`
	V3PrivPass          *string `json:"v3_priv_pass"`
	V3EngineID          *string `json:"v3_engine_id"`
	PollIntervalSeconds int     `json:"poll_interval_seconds"`
}

func (s *Server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	var body createDeviceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Host = strings.TrimSpace(body.Host)
	body.SNMPVersion = strings.TrimSpace(strings.ToLower(body.SNMPVersion))
	if body.Name == "" || body.Host == "" {
		writeError(w, http.StatusBadRequest, "name и host обязательны")
		return
	}
	if err := netutil.ValidateDeviceHost(body.Host); err != nil {
		writeError(w, http.StatusBadRequest, "host: "+err.Error())
		return
	}
	if !isSNMPCommunityMode(body.SNMPVersion) && body.SNMPVersion != "v3" {
		writeError(w, http.StatusBadRequest, "snmp_version должен быть v1, v2c или v3")
		return
	}
	if isSNMPCommunityMode(body.SNMPVersion) && (body.Community == nil || strings.TrimSpace(*body.Community) == "") {
		writeError(w, http.StatusBadRequest, "для v1/v2c нужен community")
		return
	}
	if body.SNMPVersion == "v3" && (body.V3User == nil || strings.TrimSpace(*body.V3User) == "") {
		writeError(w, http.StatusBadRequest, "для v3 нужен v3_user")
		return
	}
	if body.SNMPVersion == "v3" {
		authProto := strings.ToUpper(strings.TrimSpace(ptrStr(body.V3AuthProtocol)))
		if authProto == "" {
			authProto = "SHA"
		}
		switch authProto {
		case "SHA", "MD5", "SHA224", "SHA256", "SHA384", "SHA512":
		default:
			writeError(w, http.StatusBadRequest, "v3_auth_protocol должен быть SHA/MD5/SHA224/SHA256/SHA384/SHA512")
			return
		}
		authPass := strings.TrimSpace(ptrStr(body.V3AuthPass))
		if len(authPass) < 8 {
			writeError(w, http.StatusBadRequest, "для v3_auth_pass нужно минимум 8 символов")
			return
		}
		privProto := strings.ToUpper(strings.TrimSpace(ptrStr(body.V3PrivProtocol)))
		if privProto == "" {
			privProto = "AES"
		}
		switch privProto {
		case "AES", "AES128", "AES192", "AES256", "DES", "NONE":
		default:
			writeError(w, http.StatusBadRequest, "v3_priv_protocol должен быть AES/AES128/AES192/AES256/DES/NONE")
			return
		}
		if privProto != "NONE" {
			privPass := strings.TrimSpace(ptrStr(body.V3PrivPass))
			if len(privPass) < 8 {
				writeError(w, http.StatusBadRequest, "для v3_priv_pass нужно минимум 8 символов (или v3_priv_protocol=NONE)")
				return
			}
		}
	}

	var locPtr *string
	if body.Location != nil {
		t := strings.TrimSpace(*body.Location)
		if t != "" {
			locPtr = &t
		}
	}

	id, err := s.st.CreateDevice(r.Context(), store.CreateDeviceInput{
		Name:                body.Name,
		Host:                body.Host,
		Location:            locPtr,
		DeviceCategory:      body.DeviceCategory,
		SNMPVersion:         body.SNMPVersion,
		Community:           body.Community,
		V3User:              body.V3User,
		V3AuthProtocol:      body.V3AuthProtocol,
		V3AuthPass:          body.V3AuthPass,
		V3PrivProtocol:      body.V3PrivProtocol,
		V3PrivPass:          body.V3PrivPass,
		V3EngineID:          body.V3EngineID,
		PollIntervalSeconds: body.PollIntervalSeconds,
	})
	if err != nil {
		if dup, ok := store.IsDuplicateIdentity(err); ok {
			writeError(w, http.StatusConflict, dup.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "device.create", "device", &id, map[string]interface{}{"name": body.Name, "host": body.Host})
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) handlePatchDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body createDeviceBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Host = strings.TrimSpace(body.Host)
	body.SNMPVersion = strings.TrimSpace(strings.ToLower(body.SNMPVersion))
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name обязателен")
		return
	}
	// host может быть пустым (очистка адреса); дубликаты проверяет store
	if !isSNMPCommunityMode(body.SNMPVersion) && body.SNMPVersion != "v3" {
		writeError(w, http.StatusBadRequest, "snmp_version должен быть v1, v2c или v3")
		return
	}
	if isSNMPCommunityMode(body.SNMPVersion) {
		if body.Community == nil || strings.TrimSpace(*body.Community) == "" {
			cur, err := s.st.GetDevice(r.Context(), id)
			if err != nil || cur == nil {
				writeError(w, http.StatusNotFound, "узел не найден")
				return
			}
			if cur.Community == nil || strings.TrimSpace(*cur.Community) == "" {
				writeError(w, http.StatusBadRequest, "для v1/v2c нужен community")
				return
			}
			c := strings.TrimSpace(*cur.Community)
			body.Community = &c
		}
		body.V3User, body.V3AuthProtocol, body.V3AuthPass, body.V3PrivProtocol, body.V3PrivPass, body.V3EngineID = nil, nil, nil, nil, nil, nil
	}
	if body.SNMPVersion == "v3" {
		if body.V3User == nil || strings.TrimSpace(*body.V3User) == "" {
			writeError(w, http.StatusBadRequest, "для v3 нужен v3_user")
			return
		}
		authProto := strings.ToUpper(strings.TrimSpace(ptrStr(body.V3AuthProtocol)))
		if authProto == "" {
			authProto = "SHA"
			body.V3AuthProtocol = &authProto
		}
		switch authProto {
		case "SHA", "MD5", "SHA224", "SHA256", "SHA384", "SHA512":
		default:
			writeError(w, http.StatusBadRequest, "v3_auth_protocol должен быть SHA/MD5/SHA224/SHA256/SHA384/SHA512")
			return
		}
		authPass := strings.TrimSpace(ptrStr(body.V3AuthPass))
		if len(authPass) < 8 {
			writeError(w, http.StatusBadRequest, "для v3_auth_pass нужно минимум 8 символов")
			return
		}
		privProto := strings.ToUpper(strings.TrimSpace(ptrStr(body.V3PrivProtocol)))
		if privProto == "" {
			privProto = "AES"
			body.V3PrivProtocol = &privProto
		}
		switch privProto {
		case "AES", "AES128", "AES192", "AES256", "DES", "NONE":
		default:
			writeError(w, http.StatusBadRequest, "v3_priv_protocol должен быть AES/AES128/AES192/AES256/DES/NONE")
			return
		}
		if privProto != "NONE" {
			privPass := strings.TrimSpace(ptrStr(body.V3PrivPass))
			if len(privPass) < 8 {
				writeError(w, http.StatusBadRequest, "для v3_priv_pass нужно минимум 8 символов (или v3_priv_protocol=NONE)")
				return
			}
		}
		body.Community = nil
	}
	if err := s.st.UpdateDevice(r.Context(), id, store.CreateDeviceInput{
		Name:                body.Name,
		Host:                body.Host,
		SNMPVersion:         body.SNMPVersion,
		Community:           body.Community,
		V3User:              body.V3User,
		V3AuthProtocol:      body.V3AuthProtocol,
		V3AuthPass:          body.V3AuthPass,
		V3PrivProtocol:      body.V3PrivProtocol,
		V3PrivPass:          body.V3PrivPass,
		V3EngineID:          body.V3EngineID,
		PollIntervalSeconds: body.PollIntervalSeconds,
	}); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		if dup, ok := store.IsDuplicateIdentity(err); ok {
			writeError(w, http.StatusConflict, dup.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "device.update", "device", &id, map[string]interface{}{
		"name": body.Name, "host": body.Host, "snmp_version": body.SNMPVersion,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (s *Server) handleListDeviceEvents(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	eventType := r.URL.Query().Get("event_type")
	list, err := s.st.ListEventsByDevice(r.Context(), id, limit, eventType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list != nil {
		list, err = s.st.FilterEventsHideWiFiMACs(r.Context(), list)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if list == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	d, err := s.st.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "узел не найден")
		return
	}
	redactDeviceForAPI(d)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleListInterfaces(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	list, err := s.st.ListInterfacesByDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleSNMPTest(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	pd, err := s.st.GetPollDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if pd == nil {
		writeError(w, http.StatusNotFound, "узел не найден")
		return
	}
	if err := netutil.ValidateDeviceHost(pd.Host); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": "host: " + err.Error()})
		return
	}
	s.audit(r, "device.snmp_test", "device", &id, map[string]interface{}{"host": pd.Host})
	g, err := snmp.NewGoSNMP(*pd)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	if err := g.Connect(); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	defer g.Conn.Close()
	sysName, sysDescr, err := snmp.SysGet(g)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":        true,
		"sys_name":  sysName,
		"sys_descr": sysDescr,
	})
}

func (s *Server) handleGetNotifications(w http.ResponseWriter, r *http.Request) {
	ns, err := s.st.GetNotificationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPublicNotifications(ns))
}

type testEmailBody struct {
	EmailFrom       *string `json:"email_from"`
	EmailTo         *string `json:"email_to"`
	SMTPHost        *string `json:"smtp_host"`
	SMTPPort        *int    `json:"smtp_port"`
	SMTPUsername    *string `json:"smtp_username"`
	SMTPPassword    *string `json:"smtp_password"` // пусто = взять сохранённый
	SMTPTLSSkipVerify *bool `json:"smtp_tls_skip_verify"`
}

// handlePostEmailTest шлёт пробное письмо по полям формы (пароль можно не слать — берётся из БД).
func (s *Server) handlePostEmailTest(w http.ResponseWriter, r *http.Request) {
	var body testEmailBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	cur, err := s.st.GetNotificationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	from := strings.TrimSpace(derefStr(body.EmailFrom))
	if from == "" && cur.EmailFrom != nil {
		from = strings.TrimSpace(*cur.EmailFrom)
	}
	to := strings.TrimSpace(derefStr(body.EmailTo))
	if to == "" && cur.EmailTo != nil {
		to = strings.TrimSpace(*cur.EmailTo)
	}
	host := strings.TrimSpace(derefStr(body.SMTPHost))
	if host == "" && cur.SMTPHost != nil {
		host = strings.TrimSpace(*cur.SMTPHost)
	}
	port := 587
	if body.SMTPPort != nil && *body.SMTPPort > 0 {
		port = *body.SMTPPort
	} else if cur.SMTPPort > 0 {
		port = cur.SMTPPort
	}
	user := strings.TrimSpace(derefStr(body.SMTPUsername))
	if user == "" && cur.SMTPUsername != nil {
		user = strings.TrimSpace(*cur.SMTPUsername)
	}
	pass := strings.TrimSpace(derefStr(body.SMTPPassword))
	if pass == "" && cur.SMTPPassword != nil {
		pass = *cur.SMTPPassword
	}
	tlsSkip := cur.SMTPTLSSkipVerify
	if body.SMTPTLSSkipVerify != nil {
		tlsSkip = *body.SMTPTLSSkipVerify
	}
	if host == "" {
		writeError(w, http.StatusBadRequest, "укажите SMTP host")
		return
	}
	if err := netutil.ValidateDeviceHost(host); err != nil {
		writeError(w, http.StatusBadRequest, "smtp_host: "+err.Error())
		return
	}
	if from == "" {
		writeError(w, http.StatusBadRequest, "укажите отправителя (From)")
		return
	}
	if to == "" {
		writeError(w, http.StatusBadRequest, "укажите получателей")
		return
	}
	cfg := notify.EmailConfig{
		From:          from,
		To:            to,
		SMTPHost:      host,
		SMTPPort:      port,
		SMTPUsername:  user,
		SMTPPassword:  pass,
		TLSSkipVerify: tlsSkip,
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	em := notify.NewEmail()
	base := strings.TrimRight(strings.TrimSpace(s.cfg.PublicBaseURL), "/")
	demoURL := base
	if demoURL != "" {
		demoURL += "/devices/1"
	}
	mail := notify.BuildAlertEmail(to, []notify.EmailDeviceCard{
		{
			DeviceID:   1,
			Title:      "Пример расположения",
			Subtitle:   "demo-switch",
			StatusLine: "Снова в сети с " + time.Now().Local().Format("15:04") + " после 5м 0с отсутствия в сети",
			Kind:       "DEVICE_ONLINE",
			URL:        demoURL,
		},
	}, time.Now())
	inline := []notify.InlineImage{{
		CID:         "netlynx-logo",
		ContentType: "image/png",
		Data:        notify.LogoPNG(),
	}}
	if err := em.SendHTML(ctx, cfg, mail.Subject, mail.TextBody, mail.HTMLBody, inline); err != nil {
		writeError(w, http.StatusBadGateway, "не удалось отправить: "+err.Error())
		return
	}
	s.audit(r, "notifications.email_test", "settings", nil, map[string]interface{}{
		"smtp_host": host,
		"smtp_port": port,
		"to":        to,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "to": to})
}

type patchNotificationsBody struct {
	WebhookURL           *string `json:"webhook_url"`
	WebhookEnabled       *bool   `json:"webhook_enabled"`
	WebhookEventTypes    *string `json:"webhook_event_types"`
	WebhookSeverities    *string `json:"webhook_severities"`
	EmailEnabled         *bool   `json:"email_enabled"`
	EmailFrom            *string `json:"email_from"`
	EmailTo              *string `json:"email_to"`
	EmailEventTypes      *string `json:"email_event_types"`
	EmailSeverities      *string `json:"email_severities"`
	SMTPHost             *string `json:"smtp_host"`
	SMTPPort             *int    `json:"smtp_port"`
	SMTPUsername         *string `json:"smtp_username"`
	SMTPPassword         *string `json:"smtp_password"`
	SMTPTLSSkipVerify    *bool   `json:"smtp_tls_skip_verify"`
	TelegramBotToken     *string `json:"telegram_bot_token"`
	TelegramChatID       *string `json:"telegram_chat_id"`
	TelegramEnabled      *bool   `json:"telegram_enabled"`
	TelegramEventTypes   *string `json:"telegram_event_types"`
	TelegramSeverities   *string `json:"telegram_severities"`
	NotifyMaxRetries           *int    `json:"notify_max_retries"`
	NotifyRetryBackoffMs       *int    `json:"notify_retry_backoff_ms"`
	IncidentActionEnabled      *bool   `json:"incident_action_enabled"`
	IncidentActionEventTypes   *string `json:"incident_action_event_types"`
	IncidentActionDryRun       *bool   `json:"incident_action_dry_run"`
	IncidentActionCooldownSec  *int    `json:"incident_action_cooldown_seconds"`
}

func (s *Server) handlePatchNotifications(w http.ResponseWriter, r *http.Request) {
	var body patchNotificationsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	cur, err := s.st.GetNotificationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	url := cur.WebhookURL
	if body.WebhookURL != nil {
		u := strings.TrimSpace(*body.WebhookURL)
		if u != "" {
			if err := netutil.ValidateOutboundURL(u, netutil.WebhookPolicy()); err != nil {
				writeError(w, http.StatusBadRequest, "webhook_url: "+err.Error()+" (нужен https:// на публичный хост)")
				return
			}
			uu := u
			url = &uu
		} else {
			url = nil
		}
	}
	en := cur.WebhookEnabled
	if body.WebhookEnabled != nil {
		en = *body.WebhookEnabled
	}
	whTypes := cur.WebhookEventTypes
	if body.WebhookEventTypes != nil {
		t := strings.TrimSpace(*body.WebhookEventTypes)
		if t == "" {
			whTypes = nil
		} else {
			whTypes = &t
		}
	}
	whSev := cur.WebhookSeverities
	if body.WebhookSeverities != nil {
		t := strings.TrimSpace(*body.WebhookSeverities)
		if t == "" {
			whSev = nil
		} else {
			whSev = &t
		}
	}
	emailEn := cur.EmailEnabled
	if body.EmailEnabled != nil {
		emailEn = *body.EmailEnabled
	}
	emailFrom := cur.EmailFrom
	if body.EmailFrom != nil {
		t := strings.TrimSpace(*body.EmailFrom)
		if t == "" {
			emailFrom = nil
		} else {
			emailFrom = &t
		}
	}
	emailTo := cur.EmailTo
	if body.EmailTo != nil {
		t := strings.TrimSpace(*body.EmailTo)
		if t == "" {
			emailTo = nil
		} else {
			emailTo = &t
		}
	}
	emailTypes := cur.EmailEventTypes
	if body.EmailEventTypes != nil {
		t := strings.TrimSpace(*body.EmailEventTypes)
		if t == "" {
			emailTypes = nil
		} else {
			emailTypes = &t
		}
	}
	emailSev := cur.EmailSeverities
	if body.EmailSeverities != nil {
		t := strings.TrimSpace(*body.EmailSeverities)
		if t == "" {
			emailSev = nil
		} else {
			emailSev = &t
		}
	}
	smtpHost := cur.SMTPHost
	if body.SMTPHost != nil {
		t := strings.TrimSpace(*body.SMTPHost)
		if t == "" {
			smtpHost = nil
		} else {
			smtpHost = &t
		}
	}
	smtpPort := cur.SMTPPort
	if body.SMTPPort != nil {
		if *body.SMTPPort <= 0 || *body.SMTPPort > 65535 {
			writeError(w, http.StatusBadRequest, "smtp_port должен быть в диапазоне 1..65535")
			return
		}
		smtpPort = *body.SMTPPort
	}
	smtpUser := cur.SMTPUsername
	if body.SMTPUsername != nil {
		t := strings.TrimSpace(*body.SMTPUsername)
		if t == "" {
			smtpUser = nil
		} else {
			smtpUser = &t
		}
	}
	smtpPass := cur.SMTPPassword
	if body.SMTPPassword != nil {
		t := strings.TrimSpace(*body.SMTPPassword)
		if t != "" {
			smtpPass = &t
		}
		// пустая строка — не менять сохранённый пароль
	}
	smtpTLSSkip := cur.SMTPTLSSkipVerify
	if body.SMTPTLSSkipVerify != nil {
		smtpTLSSkip = *body.SMTPTLSSkipVerify
	}
	tok := cur.TelegramBotToken
	if body.TelegramBotToken != nil {
		t := strings.TrimSpace(*body.TelegramBotToken)
		if t != "" {
			tok = &t
		}
	}
	chat := cur.TelegramChatID
	if body.TelegramChatID != nil {
		c := strings.TrimSpace(*body.TelegramChatID)
		if c == "" {
			chat = nil
		} else {
			chat = &c
		}
	}
	tgEn := cur.TelegramEnabled
	if body.TelegramEnabled != nil {
		tgEn = *body.TelegramEnabled
	}
	tgTypes := cur.TelegramEventTypes
	if body.TelegramEventTypes != nil {
		t := strings.TrimSpace(*body.TelegramEventTypes)
		if t == "" {
			tgTypes = nil
		} else {
			tgTypes = &t
		}
	}
	tgSev := cur.TelegramSeverities
	if body.TelegramSeverities != nil {
		t := strings.TrimSpace(*body.TelegramSeverities)
		if t == "" {
			tgSev = nil
		} else {
			tgSev = &t
		}
	}
	maxRetries := cur.NotifyMaxRetries
	if body.NotifyMaxRetries != nil {
		if *body.NotifyMaxRetries < 0 || *body.NotifyMaxRetries > 10 {
			writeError(w, http.StatusBadRequest, "notify_max_retries должен быть в диапазоне 0..10")
			return
		}
		maxRetries = *body.NotifyMaxRetries
	}
	backoffMs := cur.NotifyRetryBackoffMs
	if body.NotifyRetryBackoffMs != nil {
		if *body.NotifyRetryBackoffMs < 100 || *body.NotifyRetryBackoffMs > 60000 {
			writeError(w, http.StatusBadRequest, "notify_retry_backoff_ms должен быть в диапазоне 100..60000")
			return
		}
		backoffMs = *body.NotifyRetryBackoffMs
	}
	if tgEn {
		if tok == nil || strings.TrimSpace(*tok) == "" || chat == nil || strings.TrimSpace(*chat) == "" {
			writeError(w, http.StatusBadRequest, "для telegram_enabled нужны непустые telegram_bot_token и telegram_chat_id")
			return
		}
	}
	if emailEn {
		if emailFrom == nil || strings.TrimSpace(*emailFrom) == "" || emailTo == nil || strings.TrimSpace(*emailTo) == "" {
			writeError(w, http.StatusBadRequest, "для email_enabled нужны непустые email_from и email_to")
			return
		}
		if smtpHost == nil || strings.TrimSpace(*smtpHost) == "" {
			writeError(w, http.StatusBadRequest, "для email_enabled нужен непустой smtp_host")
			return
		}
	}
	incEn := cur.IncidentActionEnabled
	if body.IncidentActionEnabled != nil {
		incEn = *body.IncidentActionEnabled
	}
	incTypes := cur.IncidentActionEventTypes
	if body.IncidentActionEventTypes != nil {
		t := strings.TrimSpace(*body.IncidentActionEventTypes)
		if t == "" {
			incTypes = nil
		} else {
			incTypes = &t
		}
	}
	incDry := cur.IncidentActionDryRun
	if body.IncidentActionDryRun != nil {
		incDry = *body.IncidentActionDryRun
	}
	incCD := cur.IncidentActionCooldownSeconds
	if body.IncidentActionCooldownSec != nil {
		if *body.IncidentActionCooldownSec < 0 || *body.IncidentActionCooldownSec > 86400 {
			writeError(w, http.StatusBadRequest, "incident_action_cooldown_seconds: 0..86400")
			return
		}
		incCD = *body.IncidentActionCooldownSec
	}
	next := store.NotificationSettings{
		WebhookURL:                     url,
		WebhookEnabled:                 en,
		WebhookEventTypes:              whTypes,
		WebhookSeverities:              whSev,
		EmailEnabled:                   emailEn,
		EmailFrom:                      emailFrom,
		EmailTo:                        emailTo,
		EmailEventTypes:                emailTypes,
		EmailSeverities:                emailSev,
		SMTPHost:                       smtpHost,
		SMTPPort:                       smtpPort,
		SMTPUsername:                   smtpUser,
		SMTPPassword:                   smtpPass,
		SMTPTLSSkipVerify:              smtpTLSSkip,
		TelegramBotToken:               tok,
		TelegramChatID:                 chat,
		TelegramEnabled:                tgEn,
		TelegramEventTypes:             tgTypes,
		TelegramSeverities:             tgSev,
		NotifyMaxRetries:               maxRetries,
		NotifyRetryBackoffMs:           backoffMs,
		IncidentActionEnabled:          incEn,
		IncidentActionEventTypes:       incTypes,
		IncidentActionDryRun:           incDry,
		IncidentActionCooldownSeconds:   incCD,
	}
	if err := s.st.UpsertNotificationSettings(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "settings.notifications.update", "settings", nil, map[string]interface{}{
		"webhook_enabled": en, "telegram_enabled": tgEn, "email_enabled": emailEn,
		"incident_action_enabled": incEn, "incident_action_dry_run": incDry,
	})
	ns, _ := s.st.GetNotificationSettings(r.Context())
	writeJSON(w, http.StatusOK, toPublicNotifications(ns))
}

type uispSettingsResponse struct {
	Enabled         bool    `json:"enabled"`
	BaseURL         *string `json:"base_url,omitempty"`
	HasAPIToken     bool    `json:"has_api_token"`
	ImportCommunity string  `json:"import_community"`
}

func (s *Server) handleGetUISPSettings(w http.ResponseWriter, r *http.Request) {
	row, err := s.st.GetUISPSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	hasTok := row.APIToken != nil && strings.TrimSpace(*row.APIToken) != ""
	out := uispSettingsResponse{
		Enabled:         row.Enabled,
		BaseURL:         row.BaseURL,
		HasAPIToken:     hasTok,
		ImportCommunity: row.ImportCommunity,
	}
	if strings.TrimSpace(out.ImportCommunity) == "" {
		out.ImportCommunity = "public"
	}
	writeJSON(w, http.StatusOK, out)
}

type patchUISPBody struct {
	Enabled          *bool   `json:"enabled"`
	BaseURL          *string `json:"base_url"`
	APIToken         *string `json:"api_token"`
	ImportCommunity  *string `json:"import_community"`
}

func (s *Server) handlePatchUISPSettings(w http.ResponseWriter, r *http.Request) {
	var body patchUISPBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	cur, err := s.st.GetUISPSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	enabled := cur.Enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	baseURL := cur.BaseURL
	if body.BaseURL != nil {
		t := strings.TrimSpace(*body.BaseURL)
		if t == "" {
			baseURL = nil
		} else {
			tt := t
			baseURL = &tt
		}
	}
	apiTok := cur.APIToken
	if body.APIToken != nil {
		t := strings.TrimSpace(*body.APIToken)
		if t != "" {
			tt := t
			apiTok = &tt
		}
		// пустая строка — оставить прежний token
	}
	importComm := strings.TrimSpace(cur.ImportCommunity)
	if importComm == "" {
		importComm = "public"
	}
	if body.ImportCommunity != nil {
		t := strings.TrimSpace(*body.ImportCommunity)
		if t == "" {
			importComm = "public"
		} else {
			importComm = t
		}
	}
	if enabled {
		if baseURL == nil || strings.TrimSpace(*baseURL) == "" {
			writeError(w, http.StatusBadRequest, "при включённом UISP укажите непустой base_url (https://…)")
			return
		}
		if _, err := uisp.NormalizeBaseURL(*baseURL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if apiTok == nil || strings.TrimSpace(*apiTok) == "" {
			writeError(w, http.StatusBadRequest, "при включённом UISP нужен непустой api_token")
			return
		}
	}
	if err := s.st.UpsertUISPSettings(r.Context(), enabled, baseURL, apiTok, importComm); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "settings.uisp.update", "settings", nil, map[string]interface{}{
		"enabled": enabled, "has_api_token": apiTok != nil && strings.TrimSpace(*apiTok) != "",
	})
	s.handleGetUISPSettings(w, r)
}

func (s *Server) handleGetTopologySettings(w http.ResponseWriter, r *http.Request) {
	row, err := s.st.GetTopologySettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

type patchTopologySettingsBody struct {
	RootDeviceID *int64 `json:"root_device_id"`
	// ClearRoot=true сбрасывает корень (JSON null без указателя не отличить от «поле не передано»).
	ClearRoot bool `json:"clear_root"`
}

func (s *Server) handlePatchTopologySettings(w http.ResponseWriter, r *http.Request) {
	var body patchTopologySettingsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	var root *int64
	if body.ClearRoot {
		root = nil
	} else if body.RootDeviceID != nil {
		if *body.RootDeviceID <= 0 {
			writeError(w, http.StatusBadRequest, "root_device_id должен быть > 0")
			return
		}
		root = body.RootDeviceID
	} else {
		writeError(w, http.StatusBadRequest, "укажите root_device_id или clear_root=true")
		return
	}
	if err := s.st.SetTopologyRootDeviceID(r.Context(), root); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "не найдено") {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		writeError(w, http.StatusInternalServerError, msg)
		return
	}
	auditVal := map[string]interface{}{"clear_root": body.ClearRoot}
	if root != nil {
		auditVal["root_device_id"] = *root
	}
	s.audit(r, "settings.topology.update", "settings", nil, auditVal)
	s.handleGetTopologySettings(w, r)
}

func (s *Server) handleImportUISP(w http.ResponseWriter, r *http.Request) {
	row, err := s.st.GetUISPSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !row.Enabled {
		writeError(w, http.StatusBadRequest, "интеграция UISP выключена в настройках")
		return
	}
	if row.BaseURL == nil || strings.TrimSpace(*row.BaseURL) == "" {
		writeError(w, http.StatusBadRequest, "не задан URL UISP")
		return
	}
	if row.APIToken == nil || strings.TrimSpace(*row.APIToken) == "" {
		writeError(w, http.StatusBadRequest, "не задан API token UISP")
		return
	}
	comm := strings.TrimSpace(row.ImportCommunity)
	if comm == "" {
		comm = "public"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	switches, err := uisp.FetchSwitches(ctx, *row.BaseURL, *row.APIToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	created, updated, skipped := 0, 0, 0
	for _, sw := range switches {
		if err := netutil.ValidateDeviceHost(sw.Host); err != nil {
			skipped++
			continue
		}
		isNew, err := s.st.UpsertSwitchFromUISP(r.Context(), sw.Name, sw.Host, sw.Location, comm, sw.UISPDeviceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if isNew {
			created++
		} else {
			updated++
		}
	}
	if m, err := uisp.FetchSwitchOverviewStatuses(ctx, *row.BaseURL, *row.APIToken); err == nil {
		_, _ = s.st.ApplyUISPOverviewStatuses(r.Context(), m)
	}
	s.audit(r, "devices.import_uisp", "devices", nil, map[string]interface{}{
		"created": created, "updated": updated, "skipped": skipped, "total": len(switches),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"created": created,
		"updated": updated,
		"skipped": skipped,
		"total":   len(switches),
	})
}

type patchDeviceNameBody struct {
	Name string `json:"name"`
}

func (s *Server) handlePatchDeviceName(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceNameBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name обязателен")
		return
	}
	if err := s.st.UpdateDeviceName(r.Context(), id, name); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "device.rename", "device", &id, map[string]interface{}{"name": name})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "name": name})
}

type patchDeviceHostBody struct {
	Host *string `json:"host"`
}

func (s *Server) handlePatchDeviceHost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceHostBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	host := ""
	if body.Host != nil {
		host = *body.Host
	}
	if err := netutil.ValidateDeviceHost(host); err != nil {
		writeError(w, http.StatusBadRequest, "host: "+err.Error())
		return
	}
	if err := s.st.UpdateDeviceHost(r.Context(), id, host); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		if dup, ok := store.IsDuplicateIdentity(err); ok {
			writeError(w, http.StatusConflict, dup.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "device.host", "device", &id, map[string]interface{}{"host": strings.TrimSpace(host)})
	outHost := strings.TrimSpace(host)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "host": outHost})
}

type patchDeviceChassisMACBody struct {
	ChassisMAC *string `json:"chassis_mac"`
}

func (s *Server) handlePatchDeviceChassisMAC(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceChassisMACBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	raw := ""
	if body.ChassisMAC != nil {
		raw = *body.ChassisMAC
	}
	if err := s.st.UpdateDeviceChassisMAC(r.Context(), id, raw); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		if dup, ok := store.IsDuplicateIdentity(err); ok {
			writeError(w, http.StatusConflict, dup.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	outMAC := strings.TrimSpace(raw)
	if outMAC != "" {
		if mac, ok := store.NormalizeMACQuery(outMAC); ok {
			outMAC = mac
		}
	}
	s.audit(r, "device.chassis_mac", "device", &id, map[string]interface{}{"chassis_mac": outMAC})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "chassis_mac": outMAC})
}

type patchDeviceOnlineOverrideBody struct {
	Mode string `json:"mode"` // auto | online | offline
}

func (s *Server) handlePatchDeviceOnlineOverride(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceOnlineOverrideBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	mode := strings.TrimSpace(strings.ToLower(body.Mode))
	if mode == "" {
		mode = "auto"
	}
	if err := s.st.UpdateDeviceOnlineOverride(r.Context(), id, mode); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "device.online_override", "device", &id, map[string]interface{}{"mode": mode})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "mode": mode})
}

type patchDeviceTrustLinkTrapsBody struct {
	TrustLinkTraps bool `json:"trust_link_traps"`
}

func (s *Server) handlePatchDeviceTrustLinkTraps(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceTrustLinkTrapsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	if err := s.st.UpdateDeviceTrustLinkTraps(r.Context(), id, body.TrustLinkTraps); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "device.trust_link_traps", "device", &id, map[string]interface{}{"trust_link_traps": body.TrustLinkTraps})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "trust_link_traps": body.TrustLinkTraps})
}

type patchDeviceLocationBody struct {
	Location *string `json:"location"`
}

func (s *Server) handlePatchDeviceLocation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceLocationBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	if err := s.st.UpdateDeviceLocation(r.Context(), id, body.Location); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "device.location.update", "device", &id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

type patchDeviceCategoryBody struct {
	DeviceCategory string `json:"device_category"`
}

func (s *Server) handlePatchDeviceCategory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceCategoryBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	raw := strings.TrimSpace(body.DeviceCategory)
	if raw == "" || !store.ValidDeviceCategory(raw) {
		writeError(w, http.StatusBadRequest, "device_category: неверный id типа")
		return
	}
	cat := store.NormalizeDeviceCategory(raw)
	ok, err := s.st.DeviceCategoryExists(r.Context(), cat)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "неизвестный тип узла: "+cat)
		return
	}
	if err := s.st.UpdateDeviceCategory(r.Context(), id, cat); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "device.category.update", "device", &id, map[string]interface{}{"device_category": cat})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "device_category": cat})
}

type patchDevicePollIntervalBody struct {
	PollIntervalSeconds int `json:"poll_interval_seconds"`
}

func (s *Server) handlePatchDevicePollInterval(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDevicePollIntervalBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	if err := s.st.UpdateDevicePollInterval(r.Context(), id, body.PollIntervalSeconds); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "device.poll_interval.update", "device", &id, map[string]interface{}{
		"poll_interval_seconds": body.PollIntervalSeconds,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id, "poll_interval_seconds": body.PollIntervalSeconds})
}

func (s *Server) handleDeleteAllDevices(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Confirm")) != "DELETE-ALL-DEVICES" {
		writeError(w, http.StatusBadRequest, "нужен заголовок X-Confirm: DELETE-ALL-DEVICES")
		return
	}
	n, err := s.st.DeleteAllDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "device.delete_all", "device", nil, map[string]interface{}{"deleted": n})
	writeJSON(w, http.StatusOK, map[string]int64{"deleted": n})
}

func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	if err := s.st.DeleteDevice(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "device.delete", "device", &id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	q := r.URL.Query()
	eventType := q.Get("event_type")
	severity := q.Get("severity")
	var deviceID *int64
	if v := strings.TrimSpace(q.Get("device_id")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			deviceID = &n
		}
	}
	list, err := s.st.ListEvents(r.Context(), limit, deviceID, eventType, severity)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err = s.st.FilterEventsHideWiFiMACs(r.Context(), list)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		writeJSON(w, http.StatusOK, []interface{}{})
		return
	}
	writeJSON(w, http.StatusOK, list)
}
