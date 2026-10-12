package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

type olderThanBody struct {
	OlderThanDays int `json:"older_than_days"`
}

func (s *Server) resolveOlderThanDays(r *http.Request, def int) (int, time.Time, error) {
	days := def
	if r.Method == http.MethodPost {
		var body olderThanBody
		dec := json.NewDecoder(r.Body)
		_ = dec.Decode(&body)
		if body.OlderThanDays > 0 {
			days = body.OlderThanDays
		}
	}
	if q := strings.TrimSpace(r.URL.Query().Get("older_than_days")); q != "" {
		n, err := strconv.Atoi(q)
		if err != nil || n < 1 || n > 3650 {
			return 0, time.Time{}, errInvalidOlderThanDays
		}
		days = n
	}
	if days < 1 || days > 3650 {
		return 0, time.Time{}, errInvalidOlderThanDays
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	return days, cutoff, nil
}

type olderThanDaysError string

func (e olderThanDaysError) Error() string { return string(e) }

const errInvalidOlderThanDays = olderThanDaysError("older_than_days: ожидается 1–3650")

// GET /settings/inventory/stale-fdb?older_than_days=60
func (s *Server) handleListStaleLiveFDB(w http.ResponseWriter, r *http.Request) {
	days, cutoff, err := s.resolveOlderThanDays(r, 60)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := s.st.ListStaleLiveFDBDevices(r.Context(), cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.StaleLiveFDBDevice{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"older_than_days": days,
		"cutoff":          cutoff.Format(time.RFC3339),
		"devices":         list,
		"count":           len(list),
	})
}

// POST /settings/inventory/stale-fdb/clear
func (s *Server) handleClearStaleLiveFDB(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Confirm")) != "CLEAR-STALE-FDB" {
		writeError(w, http.StatusBadRequest, "нужен заголовок X-Confirm: CLEAR-STALE-FDB")
		return
	}
	days, cutoff, err := s.resolveOlderThanDays(r, 60)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.st.ClearStaleLiveFDB(r.Context(), cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "inventory.clear_stale_fdb", "device", nil, map[string]interface{}{
		"older_than_days":  days,
		"devices_affected": res.DevicesAffected,
		"entries_deleted":  res.EntriesDeleted,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"older_than_days":  days,
		"devices_affected": res.DevicesAffected,
		"entries_deleted":  res.EntriesDeleted,
	})
}

// GET /settings/inventory/offline-devices?older_than_days=60
func (s *Server) handleListOfflineDevices(w http.ResponseWriter, r *http.Request) {
	days, cutoff, err := s.resolveOlderThanDays(r, 60)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	list, err := s.st.ListDevicesOfflineOlderThan(r.Context(), cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.OfflineDeviceRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"older_than_days": days,
		"cutoff":          cutoff.Format(time.RFC3339),
		"devices":         list,
		"count":           len(list),
	})
}

// POST /settings/inventory/offline-devices/delete
func (s *Server) handleDeleteOfflineDevices(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(r.Header.Get("X-Confirm")) != "DELETE-OFFLINE-DEVICES" {
		writeError(w, http.StatusBadRequest, "нужен заголовок X-Confirm: DELETE-OFFLINE-DEVICES")
		return
	}
	days, cutoff, err := s.resolveOlderThanDays(r, 60)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	n, ids, err := s.st.DeleteDevicesOfflineOlderThan(r.Context(), cutoff)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ids == nil {
		ids = []int64{}
	}
	s.audit(r, "inventory.delete_offline_devices", "device", nil, map[string]interface{}{
		"older_than_days": days,
		"deleted":         n,
		"ids":             ids,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"older_than_days": days,
		"deleted":         n,
		"ids":             ids,
	})
}
