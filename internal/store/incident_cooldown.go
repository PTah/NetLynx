package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func cooldownActionType(actionType string) string {
	if actionType == "" {
		return "admin_down"
	}
	return actionType
}

// IncidentActionInCooldown true, если last_at ещё в окне cooldownSec. cooldownSec<=0 — никогда не в кулдауне.
func (s *Store) IncidentActionInCooldown(ctx context.Context, deviceID int64, ifIndex int, actionType string, cooldownSec int) (bool, error) {
	actionType = cooldownActionType(actionType)
	if cooldownSec <= 0 {
		return false, nil
	}
	var lastAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT last_at FROM incident_action_cooldowns
		WHERE device_id = $1 AND if_index = $2 AND action_type = $3`,
		deviceID, ifIndex, actionType,
	).Scan(&lastAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return time.Since(lastAt) < time.Duration(cooldownSec)*time.Second, nil
}

// TouchIncidentActionCooldown ставит last_at=now после успешного (или dry-run) действия.
func (s *Store) TouchIncidentActionCooldown(ctx context.Context, deviceID int64, ifIndex int, actionType string) error {
	actionType = cooldownActionType(actionType)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO incident_action_cooldowns (device_id, if_index, action_type, last_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (device_id, if_index, action_type) DO UPDATE SET last_at = now()`,
		deviceID, ifIndex, actionType)
	return err
}
