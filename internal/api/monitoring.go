package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleGetDeviceDetail(w http.ResponseWriter, r *http.Request) {
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
	ifs, err := s.st.ListInterfacesByDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ignoreList, _ := s.st.GetPortEventIgnoreMap(r.Context(), id)
	for i := range ifs {
		if rule, ok := ignoreList[ifs[i].IfIndex]; ok {
			ifs[i].EventIgnored = true
			ifs[i].IgnoreMode = store.ClassifyPortIgnoreMode(rule)
		} else {
			ifs[i].IgnoreMode = store.IgnoreModeOff
		}
	}
	ev, err := s.st.ListEventsByDevice(r.Context(), id, 80, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ev != nil {
		ev, err = s.st.FilterEventsHideWiFiMACs(r.Context(), ev)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	neighbors, _ := s.st.ListPortNeighbors(r.Context(), id)
	if ifs == nil {
		ifs = []models.DeviceInterface{}
	}
	if ev == nil {
		ev = []models.Event{}
	}
	ignArr := make([]store.PortEventIgnore, 0, len(ignoreList))
	for _, v := range ignoreList {
		ignArr = append(ignArr, v)
	}
	if neighbors == nil {
		neighbors = []store.PortNeighbor{}
	} else if enriched, err := s.st.EnrichNeighborsRemoteDeviceID(r.Context(), neighbors); err == nil {
		neighbors = enriched
	}
	redactDeviceForAPI(d)
	out := map[string]interface{}{
		"device":         d,
		"interfaces":     ifs,
		"recent_events":  ev,
		"neighbors":      neighbors,
		"port_ignores":   ignArr,
	}
	if d.ChassisMAC == nil || strings.TrimSpace(*d.ChassisMAC) == "" {
		if host := strings.TrimSpace(d.Host); host != "" {
			if macs, err := s.st.ListDistinctARPMACsForHost(r.Context(), host); err == nil && len(macs) > 0 {
				out["arp_observed_macs"] = macs
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type patchDeviceMonitoringBody struct {
	UtilHighPct            *float32 `json:"util_high_pct"`
	UtilOkPct              *float32 `json:"util_ok_pct"`
	FDBPollIntervalSeconds *int     `json:"fdb_poll_interval_seconds"`
}

func (s *Server) handlePatchDeviceMonitoring(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchDeviceMonitoringBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	if body.FDBPollIntervalSeconds != nil {
		sec := *body.FDBPollIntervalSeconds
		if sec != 0 && (sec < 30 || sec > 86400) {
			writeError(w, http.StatusBadRequest, "fdb_poll_interval_seconds: 30–86400 или null")
			return
		}
	}
	if err := s.st.UpdateDeviceMonitoring(r.Context(), id, body.UtilHighPct, body.UtilOkPct, body.FDBPollIntervalSeconds); err != nil {
		if err == store.ErrDeviceNotFound {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "device.monitoring.update", "device", &id, nil)
	d, _ := s.st.GetDevice(r.Context(), id)
	writeJSON(w, http.StatusOK, d)
}

type patchPortThresholdBody struct {
	UtilHighPct *float32 `json:"util_high_pct"`
	UtilOkPct   *float32 `json:"util_ok_pct"`
}

func (s *Server) handlePatchPortThresholds(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body patchPortThresholdBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	if err := s.st.UpdateInterfaceThresholds(r.Context(), deviceID, ifIndex, body.UtilHighPct, body.UtilOkPct); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "port.thresholds.update", "device", &deviceID, map[string]interface{}{"if_index": ifIndex})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type patchPortDescrBody struct {
	Descr *string `json:"descr"`
}

func (s *Server) handleListPortIgnores(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	m, err := s.st.GetPortEventIgnoreMap(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]store.PortEventIgnore, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

type putPortIgnoreBody struct {
	Mode string `json:"mode"`
	store.PortEventIgnore
}

func (s *Server) handlePutPortIgnore(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body putPortIgnoreBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	mode := strings.TrimSpace(strings.ToLower(body.Mode))
	if mode != "" {
		if mode == store.IgnoreModeOff {
			if err := s.st.DeletePortEventIgnore(r.Context(), deviceID, ifIndex); err != nil {
				if err == pgx.ErrNoRows {
					writeJSON(w, http.StatusOK, map[string]interface{}{"mode": store.IgnoreModeOff})
					return
				}
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			s.audit(r, "port.ignore.delete", "device", &deviceID, map[string]interface{}{"if_index": ifIndex, "mode": mode})
			writeJSON(w, http.StatusOK, map[string]interface{}{"mode": store.IgnoreModeOff})
			return
		}
		if mode != store.IgnoreModeSoft && mode != store.IgnoreModeAll {
			writeError(w, http.StatusBadRequest, "mode: off, soft или all")
			return
		}
		rec := store.PortIgnoreFromMode(deviceID, ifIndex, mode)
		if rec == nil {
			writeError(w, http.StatusBadRequest, "неверный mode")
			return
		}
		if err := s.st.UpsertPortEventIgnore(r.Context(), *rec); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.audit(r, "port.ignore.upsert", "device", &deviceID, map[string]interface{}{"if_index": ifIndex, "mode": mode})
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"mode":          mode,
			"port_ignore":   rec,
			"ignore_mode":   mode,
		})
		return
	}
	body.PortEventIgnore.DeviceID = deviceID
	body.PortEventIgnore.IfIndex = ifIndex
	if err := s.st.UpsertPortEventIgnore(r.Context(), body.PortEventIgnore); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "port.ignore.upsert", "device", &deviceID, map[string]interface{}{"if_index": ifIndex})
	writeJSON(w, http.StatusOK, body.PortEventIgnore)
}

func (s *Server) handleDeletePortIgnore(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	if err := s.st.DeletePortEventIgnore(r.Context(), deviceID, ifIndex); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "запись не найдена")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "port.ignore.delete", "device", &deviceID, map[string]interface{}{"if_index": ifIndex})
	w.WriteHeader(http.StatusNoContent)
}

func parseDeviceIfIndex(w http.ResponseWriter, r *http.Request) (deviceID int64, ifIndex int, ok bool) {
	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || deviceID <= 0 {
		writeError(w, http.StatusBadRequest, "неверный device id")
		return 0, 0, false
	}
	ifIndex, err = strconv.Atoi(chi.URLParam(r, "ifIndex"))
	if err != nil || ifIndex <= 0 {
		writeError(w, http.StatusBadRequest, "неверный ifIndex")
		return 0, 0, false
	}
	return deviceID, ifIndex, true
}
