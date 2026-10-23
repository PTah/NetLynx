package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Режимы расписания сброса PoE.
const (
	PoEResetSchedAtTime      = "at_time"
	PoEResetSchedTimesPerDay = "times_per_day"
)

// PortPoEResetSchedule — расписание сброса PoE на порту (время сервера).
type PortPoEResetSchedule struct {
	DeviceID     int64      `json:"device_id"`
	IfIndex      int        `json:"if_index"`
	Enabled      bool       `json:"enabled"`
	Mode         string     `json:"mode"`
	StartHour    int        `json:"start_hour"`
	StartMinute  int        `json:"start_minute"`
	TimesPerDay  int        `json:"times_per_day"`
	ResetSeconds int        `json:"reset_seconds"`
	LastFiredKey *string    `json:"last_fired_key,omitempty"`
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
	LastStatus   *string    `json:"last_status,omitempty"`
	LastError    *string    `json:"last_error,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// NextSlots — ближайшие слоты сегодня/завтра (HH:MM), для UI.
	NextSlots []string `json:"next_slots,omitempty"`
}

// PoEResetTimeSlot — минута суток для сброса.
type PoEResetTimeSlot struct {
	Hour   int
	Minute int
}

// PoEResetScheduleSlotKey — ключ «день+минута» для антидребезга (как backup).
func PoEResetScheduleSlotKey(t time.Time) string {
	return t.Format("2006-01-02-15-04")
}

// ComputePoEResetSlots — слоты сброса: N раз в сутки начиная с start, с равным шагом 24h/N.
// Пример: 3 раза с 03:00 → 03:00, 11:00, 19:00.
func ComputePoEResetSlots(startHour, startMinute, timesPerDay int) []PoEResetTimeSlot {
	h := startHour
	m := startMinute
	if h < 0 || h > 23 {
		h = 0
	}
	if m < 0 || m > 59 {
		m = 0
	}
	n := timesPerDay
	if n < 1 {
		n = 1
	}
	if n > 24 {
		n = 24
	}
	interval := (24 * 60) / n
	start := h*60 + m
	out := make([]PoEResetTimeSlot, 0, n)
	for i := 0; i < n; i++ {
		t := (start + i*interval) % (24 * 60)
		out = append(out, PoEResetTimeSlot{Hour: t / 60, Minute: t % 60})
	}
	return out
}

// FormatPoEResetSlot — "03:00".
func FormatPoEResetSlot(s PoEResetTimeSlot) string {
	return fmt.Sprintf("%02d:%02d", s.Hour, s.Minute)
}

// PoEResetScheduleMatches — совпадает ли текущая минута с одним из слотов.
func PoEResetScheduleMatches(now time.Time, startHour, startMinute, timesPerDay int) bool {
	for _, s := range ComputePoEResetSlots(startHour, startMinute, timesPerDay) {
		if now.Hour() == s.Hour && now.Minute() == s.Minute {
			return true
		}
	}
	return false
}

// NormalizePortPoEResetSchedule — режим, границы полей; at_time → times=1.
func NormalizePortPoEResetSchedule(r PortPoEResetSchedule) (PortPoEResetSchedule, error) {
	mode := r.Mode
	if mode == "" {
		mode = PoEResetSchedAtTime
	}
	if mode != PoEResetSchedAtTime && mode != PoEResetSchedTimesPerDay {
		return r, fmt.Errorf("mode: ожидается at_time или times_per_day")
	}
	if r.StartHour < 0 || r.StartHour > 23 {
		return r, fmt.Errorf("start_hour: 0..23")
	}
	if r.StartMinute < 0 || r.StartMinute > 59 {
		return r, fmt.Errorf("start_minute: 0..59")
	}
	times := r.TimesPerDay
	if mode == PoEResetSchedAtTime {
		times = 1
	} else {
		if times < 2 {
			times = 2
		}
		if times > 24 {
			return r, fmt.Errorf("times_per_day: 2..24")
		}
	}
	sec := r.ResetSeconds
	if sec == 0 {
		sec = 10
	}
	if sec < 1 || sec > 60 {
		return r, fmt.Errorf("reset_seconds: 1..60")
	}
	r.Mode = mode
	r.TimesPerDay = times
	r.ResetSeconds = sec
	return r, nil
}

// AttachPoEResetNextSlots заполняет NextSlots (до 8 ближайших на сегодня/завтра).
func AttachPoEResetNextSlots(r *PortPoEResetSchedule, now time.Time) {
	if r == nil {
		return
	}
	slots := ComputePoEResetSlots(r.StartHour, r.StartMinute, r.TimesPerDay)
	const maxShow = 8
	out := make([]string, 0, maxShow)
	nowMin := now.Hour()*60 + now.Minute()
	// Сначала сегодняшние ≥ сейчас, потом завтрашние с начала.
	for _, s := range slots {
		if s.Hour*60+s.Minute >= nowMin {
			out = append(out, FormatPoEResetSlot(s))
			if len(out) >= maxShow {
				r.NextSlots = out
				return
			}
		}
	}
	for _, s := range slots {
		out = append(out, FormatPoEResetSlot(s))
		if len(out) >= maxShow {
			break
		}
	}
	r.NextSlots = out
}

func (s *Store) GetPortPoEResetSchedule(ctx context.Context, deviceID int64, ifIndex int) (*PortPoEResetSchedule, error) {
	var r PortPoEResetSchedule
	err := s.pool.QueryRow(ctx, `
		SELECT device_id, if_index, enabled, mode, start_hour, start_minute, times_per_day,
		       reset_seconds, last_fired_key, last_run_at, last_status, last_error, updated_at
		FROM port_poe_reset_schedule
		WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex).Scan(
		&r.DeviceID, &r.IfIndex, &r.Enabled, &r.Mode, &r.StartHour, &r.StartMinute, &r.TimesPerDay,
		&r.ResetSeconds, &r.LastFiredKey, &r.LastRunAt, &r.LastStatus, &r.LastError, &r.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) UpsertPortPoEResetSchedule(ctx context.Context, in PortPoEResetSchedule) error {
	norm, err := NormalizePortPoEResetSchedule(in)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO port_poe_reset_schedule (
			device_id, if_index, enabled, mode, start_hour, start_minute, times_per_day,
			reset_seconds, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
		ON CONFLICT (device_id, if_index) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			mode = EXCLUDED.mode,
			start_hour = EXCLUDED.start_hour,
			start_minute = EXCLUDED.start_minute,
			times_per_day = EXCLUDED.times_per_day,
			reset_seconds = EXCLUDED.reset_seconds,
			updated_at = now()`,
		norm.DeviceID, norm.IfIndex, norm.Enabled, norm.Mode, norm.StartHour, norm.StartMinute,
		norm.TimesPerDay, norm.ResetSeconds)
	return err
}

func (s *Store) DeletePortPoEResetSchedule(ctx context.Context, deviceID int64, ifIndex int) error {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM port_poe_reset_schedule WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// ListEnabledPortPoEResetSchedules — все включённые расписания (для планировщика).
func (s *Store) ListEnabledPortPoEResetSchedules(ctx context.Context) ([]PortPoEResetSchedule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT device_id, if_index, enabled, mode, start_hour, start_minute, times_per_day,
		       reset_seconds, last_fired_key, last_run_at, last_status, last_error, updated_at
		FROM port_poe_reset_schedule
		WHERE enabled = true`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PortPoEResetSchedule
	for rows.Next() {
		var r PortPoEResetSchedule
		if err := rows.Scan(
			&r.DeviceID, &r.IfIndex, &r.Enabled, &r.Mode, &r.StartHour, &r.StartMinute, &r.TimesPerDay,
			&r.ResetSeconds, &r.LastFiredKey, &r.LastRunAt, &r.LastStatus, &r.LastError, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ClaimPortPoEResetScheduleSlot атомарно занимает слот (антидребезг между тиками/рестартами).
// true — этот воркер должен выполнить сброс.
func (s *Store) ClaimPortPoEResetScheduleSlot(ctx context.Context, deviceID int64, ifIndex int, slotKey string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE port_poe_reset_schedule
		SET last_fired_key = $3, updated_at = now()
		WHERE device_id = $1 AND if_index = $2 AND enabled = true
		  AND (last_fired_key IS DISTINCT FROM $3)`,
		deviceID, ifIndex, slotKey)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) SetPortPoEResetScheduleResult(ctx context.Context, deviceID int64, ifIndex int, status, errMsg string) error {
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE port_poe_reset_schedule
		SET last_run_at = now(), last_status = $3, last_error = $4, updated_at = now()
		WHERE device_id = $1 AND if_index = $2`,
		deviceID, ifIndex, status, errPtr)
	return err
}
