package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/investigate"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func (s *Server) handleInvestigateMAC(w http.ResponseWriter, r *http.Request) {
	mac := strings.TrimSpace(r.URL.Query().Get("mac"))
	if mac == "" {
		writeError(w, http.StatusBadRequest, "mac: обязателен")
		return
	}
	if _, ok := store.FormatFullMAC(mac); !ok {
		writeError(w, http.StatusBadRequest, "mac: ожидается полный MAC (aa:bb:cc:dd:ee:ff)")
		return
	}
	b := investigate.Builder{St: s.st}
	rep, err := b.BuildMACReport(r.Context(), mac)
	if err != nil {
		if err == investigate.ErrInvalidMAC {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err == investigate.ErrWiFiMACNotTracked {
			writeError(w, http.StatusNotFound,
				"WiFi-клиент: расследование отключено в настройках MAC (включите «Отслеживать WiFi устройства на AP»).")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleInvestigateLoops(w http.ResponseWriter, r *http.Request) {
	proto := strings.TrimSpace(r.URL.Query().Get("protocol"))
	b := investigate.Builder{St: s.st}
	rep, err := b.BuildLoopReport(r.Context(), proto)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleInvestigateMACFlappers(w http.ResponseWriter, r *http.Request) {
	hours := 24
	if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n <= 168 {
			hours = n
		}
	}
	minMoves := 2
	if raw := strings.TrimSpace(r.URL.Query().Get("min_moves")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 2 && n <= 100 {
			minMoves = n
		}
	}
	limit := 40
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	hideIgnored := r.URL.Query().Get("hide_ignored") != "0"
	since := time.Now().Add(-time.Duration(hours) * time.Hour)
	excludePrefix, err := s.st.WiFiTrackingExcludePrefix(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := s.st.ListFlappingMACs(r.Context(), since, minMoves, limit, excludePrefix)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hideIgnored && len(rows) > 0 {
		filtered := rows[:0]
		for _, r := range rows {
			if r.InvestigationStatus != store.MACInvestigationIgnored {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	if rows == nil {
		rows = []store.FlappingMACSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"hours":     hours,
		"min_moves": minMoves,
		"items":     rows,
	})
}

type macInvestigationStatusBody struct {
	Status string  `json:"status"`
	Note   *string `json:"note"`
}

func (s *Server) handlePatchMACInvestigationStatus(w http.ResponseWriter, r *http.Request) {
	mac := strings.TrimSpace(r.URL.Query().Get("mac"))
	if mac == "" {
		writeError(w, http.StatusBadRequest, "mac: обязателен")
		return
	}
	norm, ok := store.FormatFullMAC(mac)
	if !ok {
		writeError(w, http.StatusBadRequest, "mac: ожидается полный MAC")
		return
	}
	var body macInvestigationStatusBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	var userID *int64
	if u, ok := r.Context().Value(authUserKey).(string); ok && u != "" {
		if row, err := s.st.GetAuthUserByUsername(r.Context(), u); err == nil {
			userID = &row.ID
		}
	}
	st, err := s.st.UpsertMACInvestigationStatus(r.Context(), norm, body.Status, body.Note, userID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "investigate.mac.status", "mac", nil, map[string]interface{}{
		"mac": norm, "status": st.Status,
	})
	writeJSON(w, http.StatusOK, st)
}
