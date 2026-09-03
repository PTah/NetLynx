package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type FDBSnapshotMeta struct {
	ID          int64     `json:"id"`
	DeviceID    int64     `json:"device_id"`
	SnapshotAt  time.Time `json:"snapshot_at"`
	EntryCount  int       `json:"entry_count"`
	Source      string    `json:"source"`
}

type FDBSnapshotMACHit struct {
	DeviceID   int64     `json:"device_id"`
	DeviceName string    `json:"device_name,omitempty"`
	DeviceHost string    `json:"device_host,omitempty"`
	SnapshotID int64     `json:"snapshot_id"`
	SnapshotAt time.Time `json:"snapshot_at"`
	IfIndex    int       `json:"if_index"`
	IfName     string    `json:"if_name,omitempty"`
	IfDescr    string    `json:"if_descr,omitempty"`
	VLANID     *int      `json:"vlan_id,omitempty"`
}

func (s *Store) GetLatestFDBSnapshotTime(ctx context.Context, deviceID int64) (*time.Time, error) {
	var at time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT snapshot_at FROM fdb_snapshots
		WHERE device_id = $1
		ORDER BY snapshot_at DESC
		LIMIT 1`, deviceID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &at, nil
}

func (s *Store) InsertFDBSnapshot(ctx context.Context, deviceID int64, at time.Time, source string, entries map[string]FDBLearnedEntry) (int64, error) {
	src := strings.TrimSpace(source)
	if src == "" {
		src = "fdb_poll"
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var snapID int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO fdb_snapshots (device_id, snapshot_at, entry_count, source)
		VALUES ($1, $2, $3, $4)
		RETURNING id`, deviceID, at, len(entries), src).Scan(&snapID); err != nil {
		return 0, err
	}

	batch := &pgx.Batch{}
	for mac, ent := range entries {
		if ent.IfIndex <= 0 {
			continue
		}
		m, ok := FormatFullMAC(mac)
		if !ok {
			m = strings.ToLower(strings.TrimSpace(mac))
			if m == "" {
				continue
			}
		}
		batch.Queue(`
			INSERT INTO fdb_snapshot_entries (snapshot_id, mac, if_index, vlan_id)
			VALUES ($1, $2, $3, $4)`,
			snapID, m, ent.IfIndex, ent.VLANID)
	}
	if batch.Len() > 0 {
		br := tx.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := br.Exec(); err != nil {
				br.Close()
				return 0, err
			}
		}
		br.Close()
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return snapID, nil
}

func (s *Store) ListFDBSnapshots(ctx context.Context, deviceID int64, limit int) ([]FDBSnapshotMeta, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, device_id, snapshot_at, entry_count, source
		FROM fdb_snapshots
		WHERE device_id = $1
		ORDER BY snapshot_at DESC
		LIMIT $2`, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FDBSnapshotMeta
	for rows.Next() {
		var m FDBSnapshotMeta
		if err := rows.Scan(&m.ID, &m.DeviceID, &m.SnapshotAt, &m.EntryCount, &m.Source); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// LookupMACInFDBSnapshots — где MAC был на каждом узле в ближайшем снимке не позже at.
func (s *Store) LookupMACInFDBSnapshots(ctx context.Context, mac string, at time.Time) ([]FDBSnapshotMACHit, error) {
	full, ok := FormatFullMAC(mac)
	if !ok {
		return nil, nil
	}
	hex := macHexDigits(full)
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (s.device_id)
			s.device_id, d.name, COALESCE(d.host, ''), s.id, s.snapshot_at,
			e.if_index, e.vlan_id,
			COALESCE(i.if_name, ''), COALESCE(i.if_descr, '')
		FROM fdb_snapshot_entries e
		JOIN fdb_snapshots s ON s.id = e.snapshot_id
		JOIN devices d ON d.id = s.device_id
		LEFT JOIN device_interfaces i ON i.device_id = s.device_id AND i.if_index = e.if_index
		WHERE lower(replace(replace(e.mac, ':', ''), '-', '')) = $1
		  AND s.snapshot_at <= $2
		ORDER BY s.device_id, s.snapshot_at DESC`, hex, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FDBSnapshotMACHit
	for rows.Next() {
		var h FDBSnapshotMACHit
		if err := rows.Scan(
			&h.DeviceID, &h.DeviceName, &h.DeviceHost, &h.SnapshotID, &h.SnapshotAt,
			&h.IfIndex, &h.VLANID, &h.IfName, &h.IfDescr,
		); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) PruneFDBSnapshots(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM fdb_snapshots WHERE snapshot_at < $1`, olderThan)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
