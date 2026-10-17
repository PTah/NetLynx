package store

import (
	"context"
	"encoding/json"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

func decodeUPSSnapshot(raw []byte) *models.UPSSnapshot {
	if len(raw) == 0 {
		return nil
	}
	var u models.UPSSnapshot
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil
	}
	return &u
}

// UpdateDeviceUPSSnapshot сохраняет снимок UPS-MIB в devices.last_ups.
func (s *Store) UpdateDeviceUPSSnapshot(ctx context.Context, id int64, u *models.UPSSnapshot) error {
	if u == nil {
		return nil
	}
	b, err := json.Marshal(u)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE devices SET
			last_ups = $2::jsonb,
			last_ups_at = now(),
			updated_at = now()
		WHERE id = $1`,
		id, b,
	)
	return err
}
