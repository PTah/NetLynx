package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type DeviceSTPState struct {
	DeviceID        int64     `json:"device_id"`
	TopChanges      int64     `json:"top_changes"`
	DesignatedRoot  *string   `json:"designated_root,omitempty"`
	RootPort        *int      `json:"root_port,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *Store) GetDeviceSTPState(ctx context.Context, deviceID int64) (*DeviceSTPState, bool, error) {
	var st DeviceSTPState
	err := s.pool.QueryRow(ctx, `
		SELECT device_id, top_changes, designated_root, root_port, updated_at
		FROM device_stp_state WHERE device_id = $1
	`, deviceID).Scan(&st.DeviceID, &st.TopChanges, &st.DesignatedRoot, &st.RootPort, &st.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &st, true, nil
}

func (s *Store) UpsertDeviceSTPState(ctx context.Context, deviceID int64, topChanges int64, root *string, rootPort *int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO device_stp_state (device_id, top_changes, designated_root, root_port, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (device_id) DO UPDATE SET
			top_changes = EXCLUDED.top_changes,
			designated_root = EXCLUDED.designated_root,
			root_port = EXCLUDED.root_port,
			updated_at = now()
	`, deviceID, topChanges, root, rootPort)
	return err
}
