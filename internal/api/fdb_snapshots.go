package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListDeviceFDBSnapshots(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := 50
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	snaps, err := s.st.ListFDBSnapshots(r.Context(), deviceID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if snaps == nil {
		snaps = []store.FDBSnapshotMeta{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"items": snaps})
}

func (s *Server) handleInvestigateMACFDBHistory(w http.ResponseWriter, r *http.Request) {
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
	if skip, err := s.st.ShouldSkipWiFiMACTracking(r.Context(), norm); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if skip {
		writeError(w, http.StatusNotFound, "WiFi-клиент: история FDB недоступна при выключенном отслеживании WiFi.")
		return
	}
	days := 7
	if raw := strings.TrimSpace(r.URL.Query().Get("days")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n <= 90 {
			days = n
		}
	}
	at := time.Now().AddDate(0, 0, -days)
	hits, err := s.st.LookupMACInFDBSnapshots(r.Context(), norm, at)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hits == nil {
		hits = []store.FDBSnapshotMACHit{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mac":       norm,
		"days_ago":  days,
		"target_at": at,
		"hits":      hits,
	})
}
