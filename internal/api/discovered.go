package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/netutil"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListDiscovered(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "new"
	}
	if _, err := s.st.HealOrphanDiscoveredAdded(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.st.HealDiscoveredAlreadyInInventory(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	list, err := s.st.ListDiscovered(r.Context(), status)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.DiscoveredDevice{}
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleIgnoreDiscovered(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	d, err := s.st.GetDiscovered(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "кандидат не найден")
		return
	}
	if err := s.st.SetDiscoveredStatus(r.Context(), id, store.DiscoveredStatusIgnored, nil); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "discovered.ignore", "discovered_device", &id, map[string]interface{}{
		"identity_key": d.IdentityKey,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": id, "status": store.DiscoveredStatusIgnored})
}

func (s *Server) handleReopenDiscovered(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	d, err := s.st.GetDiscovered(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "кандидат не найден")
		return
	}
	if err := s.st.ReopenDiscovered(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "discovered.reopen", "discovered_device", &id, map[string]interface{}{
		"identity_key": d.IdentityKey,
		"was_status":   d.Status,
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": id, "status": store.DiscoveredStatusNew})
}

type discoveredSNMPBody struct {
	Host                string  `json:"host"`
	Name                string  `json:"name"`
	Location            string  `json:"location"`
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

func (s *Server) handlePreviewDiscovered(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	d, err := s.st.GetDiscovered(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "кандидат не найден")
		return
	}
	var body discoveredSNMPBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	pd, errMsg := pollDeviceFromDiscoveredBody(d, body, true)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	g, err := snmp.NewGoSNMP(pd)
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
		"host":      pd.Host,
	})
}

func (s *Server) handlePromoteDiscovered(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	d, err := s.st.GetDiscovered(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d == nil {
		writeError(w, http.StatusNotFound, "кандидат не найден")
		return
	}
	if d.Status == store.DiscoveredStatusAdded && d.PromotedDeviceID != nil {
		// Узел ещё есть — не создаём дубликат.
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ok":         true,
			"id":         *d.PromotedDeviceID,
			"already":    true,
			"discovered": id,
		})
		return
	}
	// status=added без узла (удалили из Узлы) — разрешаем повторный promote.
	if d.Status == store.DiscoveredStatusAdded && d.PromotedDeviceID == nil {
		if err := s.st.ReopenDiscovered(r.Context(), id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	var body discoveredSNMPBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	pd, errMsg := pollDeviceFromDiscoveredBody(d, body, false)
	if errMsg != "" {
		writeError(w, http.StatusBadRequest, errMsg)
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = store.SuggestDiscoveredName(d)
	}
	var locPtr *string
	if loc := strings.TrimSpace(body.Location); loc != "" {
		locPtr = &loc
	}
	var chassis *string
	if mac := store.DiscoveredChassisMAC(d); mac != "" {
		chassis = &mac
	}
	deviceID, err := s.st.CreateDevice(r.Context(), store.CreateDeviceInput{
		Name:                name,
		Host:                pd.Host,
		Location:            locPtr,
		DeviceCategory:      body.DeviceCategory,
		SNMPVersion:         pd.SNMPVersion,
		Community:           pd.Community,
		V3User:              pd.V3User,
		V3AuthProtocol:      pd.V3AuthProtocol,
		V3AuthPass:          pd.V3AuthPass,
		V3PrivProtocol:      pd.V3PrivProtocol,
		V3PrivPass:          pd.V3PrivPass,
		V3EngineID:          pd.V3EngineID,
		PollIntervalSeconds: pd.PollIntervalSeconds,
		ChassisMAC:          chassis,
	})
	if err != nil {
		if dup, ok := store.IsDuplicateIdentity(err); ok {
			writeError(w, http.StatusConflict, dup.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.st.SetDiscoveredStatus(r.Context(), id, store.DiscoveredStatusAdded, &deviceID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cat := store.NormalizeDeviceCategory(body.DeviceCategory)
	s.audit(r, "discovered.promote", "device", &deviceID, map[string]interface{}{
		"discovered_id":   id,
		"identity_key":    d.IdentityKey,
		"host":            pd.Host,
		"name":            name,
		"location":        strings.TrimSpace(body.Location),
		"device_category": cat,
	})
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"ok":              true,
		"id":              deviceID,
		"discovered":      id,
		"device_category": cat,
	})
}

// requireHost: preview SNMP — да; promote в inventory без адреса (другой офис) — нет.
func pollDeviceFromDiscoveredBody(d *store.DiscoveredDevice, body discoveredSNMPBody, requireHost bool) (store.PollDevice, string) {
	host := strings.TrimSpace(body.Host)
	if host == "" {
		host = store.SuggestDiscoveredHost(d)
	}
	if host == "" && requireHost {
		return store.PollDevice{}, "host обязателен (mgmt addr неизвестен — укажите вручную)"
	}
	if err := netutil.ValidateDeviceHost(host); err != nil {
		return store.PollDevice{}, "host: " + err.Error()
	}
	ver := strings.TrimSpace(strings.ToLower(body.SNMPVersion))
	if ver == "" {
		ver = "v2c"
	}
	if !isSNMPCommunityMode(ver) && ver != "v3" {
		return store.PollDevice{}, "snmp_version должен быть v1, v2c или v3"
	}
	comm := body.Community
	if host == "" {
		// Узел без IP: SNMP с сервера недоступен — дефолты для строки inventory.
		if !isSNMPCommunityMode(ver) {
			ver = "v2c"
		}
		if comm == nil || strings.TrimSpace(*comm) == "" {
			c := "public"
			comm = &c
		}
	} else {
		if isSNMPCommunityMode(ver) && (comm == nil || strings.TrimSpace(*comm) == "") {
			return store.PollDevice{}, "для v1/v2c нужен community"
		}
		if ver == "v3" && (body.V3User == nil || strings.TrimSpace(*body.V3User) == "") {
			return store.PollDevice{}, "для v3 нужен v3_user"
		}
	}
	poll := body.PollIntervalSeconds
	if poll <= 0 {
		poll = 60
	}
	return store.PollDevice{
		Host:                host,
		SNMPVersion:         ver,
		Community:           comm,
		V3User:              body.V3User,
		V3AuthProtocol:      body.V3AuthProtocol,
		V3AuthPass:          body.V3AuthPass,
		V3PrivProtocol:      body.V3PrivProtocol,
		V3PrivPass:          body.V3PrivPass,
		V3EngineID:          body.V3EngineID,
		PollIntervalSeconds: poll,
	}, ""
}
