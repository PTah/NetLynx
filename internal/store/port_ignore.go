package store

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type PortEventIgnore struct {
	DeviceID     int64     `json:"device_id"`
	IfIndex      int       `json:"if_index"`
	EventTypes   *string   `json:"event_types,omitempty"`
	BlockEvents  bool      `json:"block_events"`
	BlockNotify  bool      `json:"block_notify"`
	BlockActions bool      `json:"block_actions"`
	Comment      *string   `json:"comment,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Store) GetPortEventIgnoreMap(ctx context.Context, deviceID int64) (map[int]PortEventIgnore, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT device_id, if_index, event_types, block_events, block_notify, block_actions, comment, updated_at
		FROM port_event_ignore WHERE device_id = $1`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int]PortEventIgnore)
	for rows.Next() {
		var r PortEventIgnore
		if err := rows.Scan(&r.DeviceID, &r.IfIndex, &r.EventTypes, &r.BlockEvents, &r.BlockNotify, &r.BlockActions, &r.Comment, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out[r.IfIndex] = r
	}
	return out, rows.Err()
}

func (s *Store) UpsertPortEventIgnore(ctx context.Context, r PortEventIgnore) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO port_event_ignore (device_id, if_index, event_types, block_events, block_notify, block_actions, comment, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now())
		ON CONFLICT (device_id, if_index) DO UPDATE SET
			event_types = EXCLUDED.event_types,
			block_events = EXCLUDED.block_events,
			block_notify = EXCLUDED.block_notify,
			block_actions = EXCLUDED.block_actions,
			comment = EXCLUDED.comment,
			updated_at = now()`,
		r.DeviceID, r.IfIndex, r.EventTypes, r.BlockEvents, r.BlockNotify, r.BlockActions, r.Comment)
	return err
}

func (s *Store) DeletePortEventIgnore(ctx context.Context, deviceID int64, ifIndex int) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM port_event_ignore WHERE device_id = $1 AND if_index = $2`, deviceID, ifIndex)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// MatchesIgnoreEventTypes true если rule пустой, '*' или содержит eventType.
func MatchesIgnoreEventTypes(eventType string, filter *string) bool {
	if filter == nil {
		return true
	}
	raw := strings.TrimSpace(*filter)
	if raw == "" || raw == "*" {
		return true
	}
	et := strings.ToUpper(strings.TrimSpace(eventType))
	for _, part := range strings.Split(raw, ",") {
		if strings.ToUpper(strings.TrimSpace(part)) == et {
			return true
		}
	}
	return false
}
