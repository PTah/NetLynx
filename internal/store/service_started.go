package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// EventTypeServiceStarted — старт процесса netlynxd (деплой, рестарт, восстановление после сбоя).
const EventTypeServiceStarted = "SERVICE_STARTED"

// ServiceStartInfo — поля для payload SERVICE_STARTED.
type ServiceStartInfo struct {
	Version string
	Commit  string
	BuiltAt string
}

// LastServiceStartedVersion — version из последнего SERVICE_STARTED (пустая строка, если нет).
func (s *Store) LastServiceStartedVersion(ctx context.Context) (string, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT payload FROM events
		WHERE event_type = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, EventTypeServiceStarted).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if len(raw) == 0 {
		return "", nil
	}
	var pl map[string]interface{}
	if err := json.Unmarshal(raw, &pl); err != nil {
		return "", nil
	}
	if v, ok := pl["version"].(string); ok {
		return strings.TrimSpace(v), nil
	}
	return "", nil
}

// InsertServiceStarted пишет системное событие без device_id (уведомления не шлёт — только лента events).
func (s *Store) InsertServiceStarted(ctx context.Context, info ServiceStartInfo, reason string) (int64, map[string]interface{}, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "start"
	}
	pl := map[string]interface{}{
		"version":    strings.TrimSpace(info.Version),
		"commit":     strings.TrimSpace(info.Commit),
		"built_at":   strings.TrimSpace(info.BuiltAt),
		"reason":     reason,
		"started_at": time.Now().UTC().Format(time.RFC3339),
		"source":     "service",
	}
	id, err := s.InsertEvent(ctx, 0, nil, EventTypeServiceStarted, "info", pl)
	return id, pl, err
}
