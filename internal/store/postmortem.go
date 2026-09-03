package store

import (
	"context"
	"encoding/json"
	"time"
)

// ListEventsForDevicesInWindow — события на узлах за интервал [from, to].
func (s *Store) ListEventsForDevicesInWindow(ctx context.Context, deviceIDs []int64, from, to time.Time, limit int) ([]EventBrief, error) {
	if len(deviceIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, device_id, if_index, event_type, severity, created_at, payload
		FROM events
		WHERE device_id = ANY($1)
		  AND created_at >= $2 AND created_at <= $3
		ORDER BY created_at ASC
		LIMIT $4`, deviceIDs, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEventBriefs(rows)
}

// ListSNMPTrapLogsInWindow — trap-журнал по узлам и времени.
func (s *Store) ListSNMPTrapLogsInWindow(ctx context.Context, deviceIDs []int64, from, to time.Time, limit int) ([]SNMPTrapLogRow, error) {
	if len(deviceIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, received_at, source_ip, device_id, COALESCE(snmp_version, ''),
			COALESCE(community, ''), COALESCE(trap_oid, ''), if_index, payload
		FROM snmp_trap_logs
		WHERE device_id = ANY($1)
		  AND received_at >= $2 AND received_at <= $3
		ORDER BY received_at ASC
		LIMIT $4`, deviceIDs, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SNMPTrapLogRow, 0)
	for rows.Next() {
		var r SNMPTrapLogRow
		var raw []byte
		if err := rows.Scan(
			&r.ID, &r.ReceivedAt, &r.SourceIP, &r.DeviceID, &r.SNMPVersion,
			&r.Community, &r.TrapOID, &r.IfIndex, &raw,
		); err != nil {
			return nil, err
		}
		r.Payload = map[string]interface{}{}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &r.Payload)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListMACMovesForDevicesInWindow — перемещения MAC на узлах за интервал.
func (s *Store) ListMACMovesForDevicesInWindow(ctx context.Context, deviceIDs []int64, from, to time.Time, limit int) ([]MACFDBMove, error) {
	if len(deviceIDs) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, mac, device_id, from_if_index, to_if_index, vlan_id, seen_at, source
		FROM mac_fdb_moves
		WHERE device_id = ANY($1)
		  AND seen_at >= $2 AND seen_at <= $3
		ORDER BY seen_at ASC
		LIMIT $4`, deviceIDs, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MACFDBMove
	for rows.Next() {
		var m MACFDBMove
		if err := rows.Scan(&m.ID, &m.MAC, &m.DeviceID, &m.FromIfIndex, &m.ToIfIndex, &m.VLANID, &m.SeenAt, &m.Source); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListConfigSnapshotsInWindow — снимки конфига узла за интервал (метаданные).
func (s *Store) ListConfigSnapshotsInWindow(ctx context.Context, deviceID int64, from, to time.Time, limit int) ([]ConfigSnapshot, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, device_id, fetched_at, config_hash, source, octet_length(config_text)
		FROM device_config_snapshots
		WHERE device_id = $1
		  AND fetched_at >= $2 AND fetched_at <= $3
		ORDER BY fetched_at ASC
		LIMIT $4`, deviceID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConfigSnapshot
	for rows.Next() {
		var snap ConfigSnapshot
		if err := rows.Scan(&snap.ID, &snap.DeviceID, &snap.FetchedAt, &snap.ConfigHash, &snap.Source, &snap.ByteSize); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}
