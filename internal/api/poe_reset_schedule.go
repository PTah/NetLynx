package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

type putPoEResetScheduleBody struct {
	Enabled      bool   `json:"enabled"`
	Mode         string `json:"mode"`
	StartHour    *int   `json:"start_hour"`
	StartMinute  *int   `json:"start_minute"`
	TimesPerDay  *int   `json:"times_per_day"`
	ResetSeconds *int   `json:"reset_seconds"`
	// Time — альтернатива hour/minute: "HH:MM" (как в UI type=time).
	Time string `json:"time"`
}

func defaultPoEResetSchedule(deviceID int64, ifIndex int) store.PortPoEResetSchedule {
	return store.PortPoEResetSchedule{
		DeviceID:     deviceID,
		IfIndex:      ifIndex,
		Enabled:      false,
		Mode:         store.PoEResetSchedAtTime,
		StartHour:    3,
		StartMinute:  0,
		TimesPerDay:  1,
		ResetSeconds: 10,
	}
}

func (s *Server) handleGetPortPoEResetSchedule(w http.ResponseWriter, r *http.Request) {
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	row, err := s.st.GetPortPoEResetSchedule(r.Context(), deviceID, ifIndex)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if row == nil {
		d := defaultPoEResetSchedule(deviceID, ifIndex)
		store.AttachPoEResetNextSlots(&d, time.Now())
		writeJSON(w, http.StatusOK, d)
		return
	}
	store.AttachPoEResetNextSlots(row, time.Now())
	writeJSON(w, http.StatusOK, row)
}

func (s *Server) handlePutPortPoEResetSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	var body putPoEResetScheduleBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	cur, err := s.st.GetPortPoEResetSchedule(r.Context(), deviceID, ifIndex)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	row := defaultPoEResetSchedule(deviceID, ifIndex)
	if cur != nil {
		row = *cur
	}
	row.Enabled = body.Enabled
	if body.Mode != "" {
		row.Mode = body.Mode
	}
	if strings.TrimSpace(body.Time) != "" {
		h, m, perr := parseHHMM(body.Time)
		if perr != nil {
			writeError(w, http.StatusBadRequest, perr.Error())
			return
		}
		row.StartHour = h
		row.StartMinute = m
	}
	if body.StartHour != nil {
		row.StartHour = *body.StartHour
	}
	if body.StartMinute != nil {
		row.StartMinute = *body.StartMinute
	}
	if body.TimesPerDay != nil {
		row.TimesPerDay = *body.TimesPerDay
	}
	if body.ResetSeconds != nil {
		row.ResetSeconds = *body.ResetSeconds
	}
	row.DeviceID = deviceID
	row.IfIndex = ifIndex
	norm, err := store.NormalizePortPoEResetSchedule(row)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.st.UpsertPortPoEResetSchedule(r.Context(), norm); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	details := map[string]interface{}{
		"if_index":      ifIndex,
		"enabled":       norm.Enabled,
		"mode":          norm.Mode,
		"start_hour":    norm.StartHour,
		"start_minute":  norm.StartMinute,
		"times_per_day": norm.TimesPerDay,
		"reset_seconds": norm.ResetSeconds,
	}
	s.audit(r, "port.poe.reset_schedule", "device", &deviceID, details)
	s.emitConfigEditEvent(r, deviceID, &ifIndex, "port.poe_reset_schedule", details)

	out, err := s.st.GetPortPoEResetSchedule(r.Context(), deviceID, ifIndex)
	if err != nil || out == nil {
		store.AttachPoEResetNextSlots(&norm, time.Now())
		writeJSON(w, http.StatusOK, norm)
		return
	}
	store.AttachPoEResetNextSlots(out, time.Now())
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeletePortPoEResetSchedule(w http.ResponseWriter, r *http.Request) {
	if !s.requireOperatorOrAdmin(w, r) {
		return
	}
	deviceID, ifIndex, ok := parseDeviceIfIndex(w, r)
	if !ok {
		return
	}
	err := s.st.DeletePortPoEResetSchedule(r.Context(), deviceID, ifIndex)
	if err != nil {
		writeError(w, http.StatusNotFound, "расписание не найдено")
		return
	}
	details := map[string]interface{}{"if_index": ifIndex, "deleted": true}
	s.audit(r, "port.poe.reset_schedule.delete", "device", &deviceID, details)
	s.emitConfigEditEvent(r, deviceID, &ifIndex, "port.poe_reset_schedule", details)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

func parseHHMM(s string) (hour, minute int, err error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("time: ожидается ЧЧ:ММ")
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("time: ожидается ЧЧ:ММ")
	}
	return h, m, nil
}

// RunPoEResetScheduler — фоновый тик: сброс PoE по расписанию портов (время сервера).
func (s *Server) RunPoEResetScheduler(ctx context.Context) {
	log := slog.Default()
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	log.Info("poe reset schedule: scheduler started")
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.tickPoEResetSchedules(ctx, now, log)
		}
	}
}

func (s *Server) tickPoEResetSchedules(ctx context.Context, now time.Time, log *slog.Logger) {
	list, err := s.st.ListEnabledPortPoEResetSchedules(ctx)
	if err != nil {
		log.Warn("poe reset schedule: list", "err", err)
		return
	}
	slotKey := store.PoEResetScheduleSlotKey(now)
	for _, row := range list {
		times := row.TimesPerDay
		if row.Mode == store.PoEResetSchedAtTime {
			times = 1
		}
		if !store.PoEResetScheduleMatches(now, row.StartHour, row.StartMinute, times) {
			continue
		}
		claimed, cerr := s.st.ClaimPortPoEResetScheduleSlot(ctx, row.DeviceID, row.IfIndex, slotKey)
		if cerr != nil {
			log.Warn("poe reset schedule: claim", "device_id", row.DeviceID, "if_index", row.IfIndex, "err", cerr)
			continue
		}
		if !claimed {
			continue
		}
		sec := row.ResetSeconds
		if sec < 1 || sec > 60 {
			sec = 10
		}
		runCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		via, rerr := s.pushPortPoEReset(runCtx, row.DeviceID, []int{row.IfIndex}, sec)
		cancel()
		if rerr != nil {
			log.Warn("poe reset schedule: reset failed",
				"device_id", row.DeviceID, "if_index", row.IfIndex, "err", rerr)
			_ = s.st.SetPortPoEResetScheduleResult(ctx, row.DeviceID, row.IfIndex, "fail", rerr.Error())
			s.emitScheduledPoEResetEvent(ctx, row.DeviceID, row.IfIndex, sec, "", rerr.Error())
			continue
		}
		log.Info("poe reset schedule: ok",
			"device_id", row.DeviceID, "if_index", row.IfIndex, "seconds", sec, "via", via, "slot", slotKey)
		_ = s.st.SetPortPoEResetScheduleResult(ctx, row.DeviceID, row.IfIndex, "ok", "")
		s.emitScheduledPoEResetEvent(ctx, row.DeviceID, row.IfIndex, sec, via, "")
	}
}

func (s *Server) emitScheduledPoEResetEvent(ctx context.Context, deviceID int64, ifIndex, seconds int, via, errMsg string) {
	pl := map[string]interface{}{
		"change":   "port.poe_reset",
		"source":   "schedule",
		"username": "scheduler",
		"seconds":  seconds,
	}
	if via != "" {
		pl["via"] = via
	}
	if errMsg != "" {
		pl["error"] = errMsg
		pl["ok"] = false
	} else {
		pl["ok"] = true
	}
	if labels, ok, err := s.st.GetInterfaceEventLabels(ctx, deviceID, ifIndex); err == nil && ok {
		store.ApplyEventIfaceLabels(pl, labels)
	}
	sev := "info"
	if errMsg != "" {
		sev = "warning"
	}
	id, err := s.st.InsertEvent(ctx, deviceID, &ifIndex, EventTypeConfigEdit, sev, pl)
	if err != nil || s.hub == nil {
		return
	}
	name, host := "", ""
	if d, err := s.st.GetDevice(ctx, deviceID); err == nil && d != nil {
		name = d.Name
		host = strings.TrimSpace(d.Host)
	}
	s.hub.Publish(live.EventPayload{
		EventID:    id,
		DeviceID:   deviceID,
		DeviceName: name,
		DeviceHost: host,
		IfIndex:    &ifIndex,
		EventType:  EventTypeConfigEdit,
		Severity:   sev,
		Payload:    pl,
	})
}
