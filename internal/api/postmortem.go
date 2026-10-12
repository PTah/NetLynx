package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/postmortem"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func (s *Server) handleGetPostmortem(w http.ResponseWriter, r *http.Request) {
	rawID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	if rawID == "" {
		writeError(w, http.StatusBadRequest, "device_id: обязателен")
		return
	}
	deviceID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || deviceID <= 0 {
		writeError(w, http.StatusBadRequest, "device_id: ожидается положительное число")
		return
	}

	center := time.Now()
	if raw := strings.TrimSpace(r.URL.Query().Get("around")); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "around: ожидается RFC3339")
			return
		}
		center = t
	}

	window := 5 * time.Minute
	if raw := strings.TrimSpace(r.URL.Query().Get("window")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil || d <= 0 {
			writeError(w, http.StatusBadRequest, "window: ожидается duration (например 5m, 10m)")
			return
		}
		window = d
	}

	hops := 1
	if raw := strings.TrimSpace(r.URL.Query().Get("hops")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 || n > 3 {
			writeError(w, http.StatusBadRequest, "hops: 0..3")
			return
		}
		hops = n
	}

	b := postmortem.Builder{St: s.st}
	rep, err := b.Build(r.Context(), deviceID, center, window, hops)
	if err != nil {
		if err == store.ErrDeviceNotFound {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
