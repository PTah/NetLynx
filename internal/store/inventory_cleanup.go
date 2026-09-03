package store

import (
	"context"
	"time"
)

// StaleLiveFDBDevice — узел с live FDB без успешного опроса дольше cutoff.
type StaleLiveFDBDevice struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Host          string     `json:"host"`
	LastFDBPollAt *time.Time `json:"last_fdb_poll_at,omitempty"`
	EntryCount    int        `json:"entry_count"`
}

// OfflineDeviceRow — узел в оффлайне дольше cutoff (по offline_since).
type OfflineDeviceRow struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Host         string    `json:"host"`
	OfflineSince time.Time `json:"offline_since"`
}

// ClearStaleLiveFDBResult — итог очистки устаревшего live FDB.
type ClearStaleLiveFDBResult struct {
	DevicesAffected int64 `json:"devices_affected"`
	EntriesDeleted  int64 `json:"entries_deleted"`
}

// ListStaleLiveFDBDevices — узлы с device_fdb_entries, у которых last_fdb_poll_at старше olderThan
// (или NULL при наличии записей).
func (s *Store) ListStaleLiveFDBDevices(ctx context.Context, olderThan time.Time) ([]StaleLiveFDBDevice, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.name, COALESCE(d.host, ''), d.last_fdb_poll_at, COUNT(f.*)::int
		FROM devices d
		INNER JOIN device_fdb_entries f ON f.device_id = d.id
		WHERE d.last_fdb_poll_at IS NULL OR d.last_fdb_poll_at < $1
		GROUP BY d.id, d.name, d.host, d.last_fdb_poll_at
		ORDER BY d.last_fdb_poll_at ASC NULLS FIRST, d.name ASC`, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StaleLiveFDBDevice
	for rows.Next() {
		var r StaleLiveFDBDevice
		if err := rows.Scan(&r.ID, &r.Name, &r.Host, &r.LastFDBPollAt, &r.EntryCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ClearStaleLiveFDB удаляет live FDB у узлов без успешного FDB-опроса дольше olderThan
// и сбрасывает fdb_baseline_at, чтобы следующий опрос снова прошёл learning.
func (s *Store) ClearStaleLiveFDB(ctx context.Context, olderThan time.Time) (ClearStaleLiveFDBResult, error) {
	var res ClearStaleLiveFDBResult
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return res, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT d.id
		FROM devices d
		WHERE EXISTS (SELECT 1 FROM device_fdb_entries f WHERE f.device_id = d.id)
		  AND (d.last_fdb_poll_at IS NULL OR d.last_fdb_poll_at < $1)`, olderThan)
	if err != nil {
		return res, err
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return res, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}
	if len(ids) == 0 {
		return res, tx.Commit(ctx)
	}

	tag, err := tx.Exec(ctx, `DELETE FROM device_fdb_entries WHERE device_id = ANY($1)`, ids)
	if err != nil {
		return res, err
	}
	res.EntriesDeleted = tag.RowsAffected()
	res.DevicesAffected = int64(len(ids))

	if _, err := tx.Exec(ctx, `
		UPDATE devices SET
			fdb_baseline_at = NULL,
			updated_at = now()
		WHERE id = ANY($1)`, ids); err != nil {
		return res, err
	}
	if err := tx.Commit(ctx); err != nil {
		return res, err
	}
	return res, nil
}

// ListDevicesOfflineOlderThan — узлы с offline_since старше olderThan (без ручного «онлайн»).
func (s *Store) ListDevicesOfflineOlderThan(ctx context.Context, olderThan time.Time) ([]OfflineDeviceRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, COALESCE(host, ''), offline_since
		FROM devices
		WHERE offline_since IS NOT NULL
		  AND offline_since < $1
		  AND (online_override IS DISTINCT FROM TRUE)
		ORDER BY offline_since ASC, name ASC`, olderThan)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OfflineDeviceRow
	for rows.Next() {
		var r OfflineDeviceRow
		if err := rows.Scan(&r.ID, &r.Name, &r.Host, &r.OfflineSince); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteDevicesOfflineOlderThan удаляет узлы, оффлайн дольше olderThan.
func (s *Store) DeleteDevicesOfflineOlderThan(ctx context.Context, olderThan time.Time) (int64, []int64, error) {
	list, err := s.ListDevicesOfflineOlderThan(ctx, olderThan)
	if err != nil {
		return 0, nil, err
	}
	if len(list) == 0 {
		return 0, nil, nil
	}
	ids := make([]int64, 0, len(list))
	for _, d := range list {
		ids = append(ids, d.ID)
	}
	// Вернуть кандидатов «Обнаружено» в new до удаления узлов.
	if _, err := s.pool.Exec(ctx, `
		UPDATE discovered_devices SET
			status = $2,
			promoted_device_id = NULL,
			updated_at = now()
		WHERE promoted_device_id = ANY($1)`, ids, DiscoveredStatusNew); err != nil {
		return 0, nil, err
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM devices WHERE id = ANY($1)`, ids)
	if err != nil {
		return 0, nil, err
	}
	return tag.RowsAffected(), ids, nil
}
