package store

import (
	"context"
	"fmt"
	"time"
)

// EventStatsKind — набор типов событий для сводки в «Настройки → уведомления → Статистика».
type EventStatsKind string

const (
	EventStatsReachability EventStatsKind = "reachability"
	EventStatsUPS          EventStatsKind = "ups"
)

// EventStatsRow — одна строка таблицы: устройство и счётчики парных событий.
type EventStatsRow struct {
	DeviceID      int64      `json:"device_id"`
	DeviceName    string     `json:"device_name"`
	Host          string     `json:"host"`
	PrimaryCount  int64      `json:"primary_count"`  // оффлайн / питание пропало
	RecoveryCount int64      `json:"recovery_count"` // онлайн / питание вернулось
	LastPrimaryAt *time.Time `json:"last_primary_at,omitempty"`
	AvgDurationSec *float64  `json:"avg_duration_sec,omitempty"` // среднее из payload offline_duration_sec (если есть)
}

// EventStatsResult — ответ агрегации.
type EventStatsResult struct {
	Kind         EventStatsKind  `json:"kind"`
	Hours        int             `json:"hours"`
	From         time.Time       `json:"from"`
	To           time.Time       `json:"to"`
	PrimaryType  string          `json:"primary_type"`
	RecoveryType string          `json:"recovery_type"`
	Rows         []EventStatsRow `json:"rows"`
}

const eventStatsMaxHours = 365 * 24 // 8760

func eventStatsTypes(kind EventStatsKind) (primary, recovery string, ok bool) {
	switch kind {
	case EventStatsReachability:
		return "DEVICE_OFFLINE", "DEVICE_ONLINE", true
	case EventStatsUPS:
		return "UPS_ON_BATTERY", "UPS_MAINS_OK", true
	default:
		return "", "", false
	}
}

// EventStatsByKind — сколько раз устройство «падало» / теряло питание за период (и сколько раз восстановилось).
// hours — длина окна назад от now (1…8760); 0 → 720 (30 суток).
func (s *Store) EventStatsByKind(ctx context.Context, kind EventStatsKind, hours int, limit int) (EventStatsResult, error) {
	primary, recovery, ok := eventStatsTypes(kind)
	if !ok {
		return EventStatsResult{}, fmt.Errorf("неизвестный kind: %s", kind)
	}
	if hours <= 0 {
		hours = 30 * 24
	}
	if hours > eventStatsMaxHours {
		hours = eventStatsMaxHours
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	to := time.Now().UTC()
	from := to.Add(-time.Duration(hours) * time.Hour)

	rows, err := s.pool.Query(ctx, `
		SELECT
			e.device_id,
			COALESCE(d.name, '(удалено)') AS device_name,
			COALESCE(d.host, '') AS host,
			COUNT(*) FILTER (WHERE e.event_type = $1) AS primary_count,
			COUNT(*) FILTER (WHERE e.event_type = $2) AS recovery_count,
			MAX(e.created_at) FILTER (WHERE e.event_type = $1) AS last_primary_at,
			AVG(
				CASE
					WHEN e.event_type = $2
					 AND (e.payload->>'offline_duration_sec') ~ '^[0-9]+(\.[0-9]+)?$'
					THEN (e.payload->>'offline_duration_sec')::double precision
					ELSE NULL
				END
			) AS avg_duration_sec
		FROM events e
		LEFT JOIN devices d ON d.id = e.device_id
		WHERE e.device_id IS NOT NULL
		  AND e.event_type IN ($1, $2)
		  AND e.created_at >= $3
		  AND e.created_at <= $4
		GROUP BY e.device_id, d.name, d.host
		HAVING COUNT(*) FILTER (WHERE e.event_type = $1) > 0
		    OR COUNT(*) FILTER (WHERE e.event_type = $2) > 0
		ORDER BY primary_count DESC, recovery_count DESC, device_name ASC
		LIMIT $5`,
		primary, recovery, from, to, limit,
	)
	if err != nil {
		return EventStatsResult{}, err
	}
	defer rows.Close()

	out := EventStatsResult{
		Kind:         kind,
		Hours:        hours,
		From:         from,
		To:           to,
		PrimaryType:  primary,
		RecoveryType: recovery,
		Rows:         make([]EventStatsRow, 0),
	}
	for rows.Next() {
		var r EventStatsRow
		var avg *float64
		if err := rows.Scan(
			&r.DeviceID,
			&r.DeviceName,
			&r.Host,
			&r.PrimaryCount,
			&r.RecoveryCount,
			&r.LastPrimaryAt,
			&avg,
		); err != nil {
			return EventStatsResult{}, err
		}
		if avg != nil {
			v := *avg
			r.AvgDurationSec = &v
		}
		out.Rows = append(out.Rows, r)
	}
	if err := rows.Err(); err != nil {
		return EventStatsResult{}, err
	}
	return out, nil
}
