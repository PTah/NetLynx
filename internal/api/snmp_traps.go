package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/traprecv"
)

// TrapListener — hot-reload UDP-приёмника SNMP traps.
type TrapListener interface {
	Reload(ctx context.Context) error
	Status() (enabled bool, listenAddr string)
}

type snmpTrapLogAPIRow struct {
	store.SNMPTrapLogRow
	TrapLabel   string `json:"trap_label"`
	TrapSummary string `json:"trap_summary"`
}

func enrichTrapLogRow(r store.SNMPTrapLogRow) snmpTrapLogAPIRow {
	dec := traprecv.DecodeTrap(r.TrapOID, r.IfIndex, r.Payload)
	if lbl, ok := r.Payload["trap_label"].(string); ok && strings.TrimSpace(lbl) != "" {
		dec.Label = strings.TrimSpace(lbl)
	}
	if sum, ok := r.Payload["trap_summary"].(string); ok && strings.TrimSpace(sum) != "" {
		dec.Summary = strings.TrimSpace(sum)
	}
	return snmpTrapLogAPIRow{
		SNMPTrapLogRow: r,
		TrapLabel:      dec.Label,
		TrapSummary:    dec.Summary,
	}
}

func (s *Server) writeSNMPTrapSettings(w http.ResponseWriter, r *http.Request, row store.SNMPTrapSettings) {
	count, _ := s.st.CountSNMPTrapLogs(r.Context())
	include := traprecv.ParseIncludeLabels(row.TrapIncludeLabels)
	listenAddr := ""
	receiverOn := false
	if s.traps != nil {
		receiverOn, listenAddr = s.traps.Status()
	}
	if listenAddr == "" && row.ListenEnabled {
		listenAddr = ":" + strconv.Itoa(row.ListenPort)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"log_enabled":            row.LogEnabled,
		"listen_enabled":         row.ListenEnabled,
		"listen_port":            row.ListenPort,
		"trap_include_labels":    row.TrapIncludeLabels,
		"link_trap_events_mode":  row.LinkTrapEventsMode,
		"link_trap_effects":      row.LinkTrapEffects,
		"trap_filter_active":     len(include) > 0,
		"trap_label_options":     traprecv.TrapLabelOptions(),
		"updated_at":             row.UpdatedAt,
		"listen_addr":            listenAddr,
		"receiver_enabled":       receiverOn && row.ListenEnabled,
		"community_filter":       s.cfg.SNMPTrapCommunity != "",
		"log_count":              count,
		"log_retain_max":         2000,
	})
}

func (s *Server) handleGetSNMPTrapSettings(w http.ResponseWriter, r *http.Request) {
	row, err := s.st.GetSNMPTrapSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSNMPTrapSettings(w, r, row)
}

type patchSNMPTrapSettingsBody struct {
	LogEnabled         *bool   `json:"log_enabled"`
	ListenEnabled      *bool   `json:"listen_enabled"`
	ListenPort         *int    `json:"listen_port"`
	TrapIncludeLabels  *string `json:"trap_include_labels"`
	LinkTrapEventsMode *string `json:"link_trap_events_mode"`
	LinkTrapEffects    *string `json:"link_trap_effects"`
}

func (s *Server) handlePatchSNMPTrapSettings(w http.ResponseWriter, r *http.Request) {
	var body patchSNMPTrapSettingsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if body.LogEnabled == nil && body.ListenEnabled == nil && body.ListenPort == nil &&
		body.TrapIncludeLabels == nil && body.LinkTrapEventsMode == nil && body.LinkTrapEffects == nil {
		writeError(w, http.StatusBadRequest, "nothing to update")
		return
	}
	if body.ListenPort != nil && (*body.ListenPort < 1 || *body.ListenPort > 65535) {
		writeError(w, http.StatusBadRequest, "listen_port must be 1..65535")
		return
	}
	if body.LinkTrapEventsMode != nil {
		raw := strings.ToLower(strings.TrimSpace(*body.LinkTrapEventsMode))
		if raw != store.LinkTrapEventsOff && raw != store.LinkTrapEventsPerDevice && raw != store.LinkTrapEventsAll {
			writeError(w, http.StatusBadRequest, "link_trap_events_mode must be off, per_device or all")
			return
		}
	}
	if body.LinkTrapEffects != nil {
		raw := strings.ToLower(strings.TrimSpace(*body.LinkTrapEffects))
		if raw != store.LinkTrapEffectsNotify && raw != store.LinkTrapEffectsFull {
			writeError(w, http.StatusBadRequest, "link_trap_effects must be notify or full")
			return
		}
	}
	if err := s.st.PatchSNMPTrapSettings(r.Context(), store.PatchSNMPTrapSettingsInput{
		LogEnabled:         body.LogEnabled,
		ListenEnabled:      body.ListenEnabled,
		ListenPort:         body.ListenPort,
		TrapIncludeLabels:  body.TrapIncludeLabels,
		LinkTrapEventsMode: body.LinkTrapEventsMode,
		LinkTrapEffects:    body.LinkTrapEffects,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.traps != nil && (body.ListenEnabled != nil || body.ListenPort != nil) {
		if err := s.traps.Reload(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "settings saved but listener reload failed: "+err.Error())
			return
		}
	}
	row, err := s.st.GetSNMPTrapSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeSNMPTrapSettings(w, r, row)
}

func (s *Server) handleListSNMPTrapLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit invalid")
			return
		}
		limit = n
	}
	settings, err := s.st.GetSNMPTrapSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	include := traprecv.ParseIncludeLabels(settings.TrapIncludeLabels)

	rows, err := s.st.ListSNMPTrapLogs(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]snmpTrapLogAPIRow, 0, len(rows))
	for _, row := range rows {
		enriched := enrichTrapLogRow(row)
		if !traprecv.LabelMatchesInclude(enriched.TrapLabel, include) {
			continue
		}
		items = append(items, enriched)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": items})
}

func (s *Server) handleClearSNMPTrapLogs(w http.ResponseWriter, r *http.Request) {
	n, err := s.st.ClearSNMPTrapLogs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deleted": n})
}
