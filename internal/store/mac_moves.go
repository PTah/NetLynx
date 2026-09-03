package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/macvendor"
	"github.com/jackc/pgx/v5"
)

const (
	MACMoveSourceFDBPoll = "fdb_poll"
	MACMoveSourceSyslog  = "syslog"
)

// MACFDBMove — одно наблюдение появления / перемещения / исчезновения MAC.
type MACFDBMove struct {
	ID          int64     `json:"id"`
	MAC         string    `json:"mac"`
	DeviceID    int64     `json:"device_id"`
	FromIfIndex *int      `json:"from_if_index,omitempty"`
	ToIfIndex   *int      `json:"to_if_index,omitempty"`
	VLANID      *int      `json:"vlan_id,omitempty"`
	SeenAt      time.Time `json:"seen_at"`
	Source      string    `json:"source"`
}

// MACFDBMoveInsert — запись для пакетной вставки.
type MACFDBMoveInsert struct {
	MAC         string
	DeviceID    int64
	FromIfIndex *int
	ToIfIndex   *int
	VLANID      *int
	SeenAt      time.Time
	Source      string
}

// MACFootprintHit — текущее положение MAC в FDB.
type MACFootprintHit struct {
	DeviceID   int64   `json:"device_id"`
	DeviceName string  `json:"device_name"`
	DeviceHost string  `json:"device_host"`
	Category   string  `json:"device_category,omitempty"`
	IfIndex    int     `json:"if_index"`
	IfName     *string `json:"if_name,omitempty"`
	IfDescr    *string `json:"if_descr,omitempty"`
	PortRole   string  `json:"port_role"`
	VLANID     *int    `json:"vlan_id,omitempty"`
	MACCount   int     `json:"macs_on_port"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// MACAccessSighting — MAC на access-порту (для кросс-свитч детекта).
type MACAccessSighting struct {
	MAC      string
	DeviceID int64
	IfIndex  int
}

func (s *Store) InsertMACFDBMoves(ctx context.Context, rows []MACFDBMoveInsert) error {
	if len(rows) == 0 {
		return nil
	}
	for _, r := range rows {
		mac := strings.ToLower(strings.TrimSpace(r.MAC))
		if mac == "" {
			continue
		}
		src := strings.TrimSpace(r.Source)
		if src == "" {
			src = MACMoveSourceFDBPoll
		}
		at := r.SeenAt
		if at.IsZero() {
			at = time.Now()
		}
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO mac_fdb_moves (mac, device_id, from_if_index, to_if_index, vlan_id, seen_at, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			mac, r.DeviceID, r.FromIfIndex, r.ToIfIndex, r.VLANID, at, src); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListMACFDBMoves(ctx context.Context, mac string, limit int) ([]MACFDBMove, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, mac, device_id, from_if_index, to_if_index, vlan_id, seen_at, source
		FROM mac_fdb_moves
		WHERE mac = $1
		ORDER BY seen_at DESC, id DESC
		LIMIT $2`, macNorm, limit)
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

// CountMACPortMovesInWindow — число реальных смен порта (from и to заданы) за окно.
func (s *Store) CountMACPortMovesInWindow(ctx context.Context, deviceID int64, mac string, since time.Time) (int, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return 0, nil
	}
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM mac_fdb_moves
		WHERE device_id = $1 AND mac = $2 AND seen_at >= $3
		  AND from_if_index IS NOT NULL AND to_if_index IS NOT NULL
		  AND from_if_index <> to_if_index`,
		deviceID, macNorm, since).Scan(&n)
	return n, err
}

// DistinctPortsInMoves — уникальные if_index (from/to), фигурировавшие в moves за окно.
func (s *Store) DistinctPortsInMoves(ctx context.Context, deviceID int64, mac string, since time.Time) ([]int, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT if_index FROM (
			SELECT from_if_index AS if_index FROM mac_fdb_moves
			WHERE device_id = $1 AND mac = $2 AND seen_at >= $3 AND from_if_index IS NOT NULL
			UNION
			SELECT to_if_index FROM mac_fdb_moves
			WHERE device_id = $1 AND mac = $2 AND seen_at >= $3 AND to_if_index IS NOT NULL
		) t
		ORDER BY if_index`, deviceID, macNorm, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var idx int
		if err := rows.Scan(&idx); err != nil {
			return nil, err
		}
		out = append(out, idx)
	}
	return out, rows.Err()
}

func (s *Store) PruneMACFDBMoves(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM mac_fdb_moves WHERE seen_at < $1`, olderThan)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// HasMACEventSince — debounce по типу события и MAC в payload.
func (s *Store) HasMACEventSince(ctx context.Context, deviceID int64, eventType, mac string, since time.Time) (bool, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return false, nil
	}
	var okExists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM events
			WHERE device_id = $1 AND event_type = $2 AND created_at >= $3
			  AND lower(COALESCE(payload->>'mac', '')) = $4
		)`, deviceID, eventType, since, macNorm).Scan(&okExists)
	return okExists, err
}

// EventBrief — краткое событие для корреляции в отчёте.
type EventBrief struct {
	ID        int64                  `json:"id"`
	DeviceID  int64                  `json:"device_id"`
	IfIndex   *int                   `json:"if_index,omitempty"`
	EventType string                 `json:"event_type"`
	Severity  string                 `json:"severity"`
	CreatedAt time.Time              `json:"created_at"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
}

func scanEventBriefs(rows pgx.Rows) ([]EventBrief, error) {
	var out []EventBrief
	for rows.Next() {
		var e EventBrief
		var raw []byte
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.IfIndex, &e.EventType, &e.Severity, &e.CreatedAt, &raw); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &e.Payload)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListMACFootprint — все текущие FDB-хиты по MAC.
func (s *Store) ListMACFootprint(ctx context.Context, mac string) ([]MACFootprintHit, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.name, COALESCE(d.host, ''), COALESCE(d.device_category, ''),
		       f.if_index, di.if_name,
		       COALESCE(NULLIF(btrim(di.descr_override), ''), NULLIF(btrim(di.cli_description), ''), di.if_descr) AS if_descr,
		       COALESCE(di.port_role, ''),
		       f.vlan_id, f.last_seen_at,
		       (
		         SELECT COUNT(*)::int FROM device_fdb_entries fx
		         WHERE fx.device_id = f.device_id AND fx.if_index = f.if_index
		       ) AS macs_on_port
		FROM device_fdb_entries f
		INNER JOIN devices d ON d.id = f.device_id
		LEFT JOIN device_interfaces di ON di.device_id = f.device_id AND di.if_index = f.if_index
		WHERE f.mac = $1
		ORDER BY d.name ASC, f.if_index ASC`, macNorm)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MACFootprintHit
	for rows.Next() {
		var h MACFootprintHit
		if err := rows.Scan(
			&h.DeviceID, &h.DeviceName, &h.DeviceHost, &h.Category,
			&h.IfIndex, &h.IfName, &h.IfDescr, &h.PortRole,
			&h.VLANID, &h.LastSeenAt, &h.MACCount,
		); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// ListMACMultiAccessCandidates — MAC, одновременно на ≥2 access-портах (разные device/if).
func (s *Store) ListMACMultiAccessCandidates(ctx context.Context) ([]MACAccessSighting, error) {
	rows, err := s.pool.Query(ctx, `
		WITH access_hits AS (
			SELECT f.mac, f.device_id, f.if_index
			FROM device_fdb_entries f
			INNER JOIN device_interfaces di ON di.device_id = f.device_id AND di.if_index = f.if_index
			WHERE COALESCE(di.port_role, 'access') = 'access'
		),
		dup AS (
			SELECT mac
			FROM access_hits
			GROUP BY mac
			HAVING COUNT(*) >= 2
		)
		SELECT a.mac, a.device_id, a.if_index
		FROM access_hits a
		INNER JOIN dup ON dup.mac = a.mac
		ORDER BY a.mac, a.device_id, a.if_index`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MACAccessSighting
	for rows.Next() {
		var r MACAccessSighting
		if err := rows.Scan(&r.MAC, &r.DeviceID, &r.IfIndex); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FindIfIndexByPortName — сопоставление имени порта из syslog с if_index.
func (s *Store) FindIfIndexByPortName(ctx context.Context, deviceID int64, portName string) (int, bool, error) {
	want := normalizePortNameKey(portName)
	if want == "" {
		return 0, false, nil
	}
	idx, err := s.ListInterfaceNameIndex(ctx, deviceID)
	if err != nil {
		return 0, false, err
	}
	for ifIndex, name := range idx {
		if normalizePortNameKey(name) == want {
			return ifIndex, true, nil
		}
	}
	// Частичное совпадение: gi1/0/23 ⊂ GigabitEthernet1/0/23
	for ifIndex, name := range idx {
		nk := normalizePortNameKey(name)
		if strings.HasSuffix(nk, want) || strings.HasSuffix(want, nk) {
			return ifIndex, true, nil
		}
	}
	return 0, false, nil
}

func normalizePortNameKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	repl := []struct{ old, new string }{
		{"gigabitethernet", "gi"},
		{"tengigabitethernet", "te"},
		{"fastethernet", "fa"},
		{"ethernet", "eth"},
	}
	for _, r := range repl {
		if strings.HasPrefix(s, r.old) {
			s = r.new + strings.TrimPrefix(s, r.old)
			break
		}
	}
	return s
}

// ListARPByMAC — IP из ARP по MAC (любой узел).
func (s *Store) ListARPByMAC(ctx context.Context, mac string) ([]string, error) {
	macNorm, ok := FormatFullMAC(mac)
	if !ok {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ip FROM device_arp_entries WHERE lower(mac) = $1 ORDER BY ip`,
		strings.ToLower(macNorm))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, rows.Err()
}

// ListPortNeighborsOnIfaces — соседи на указанных if_index устройства.
func (s *Store) ListPortNeighborsOnIfaces(ctx context.Context, deviceID int64, ifIndexes []int) ([]PortNeighbor, error) {
	if len(ifIndexes) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT device_id, if_index, rem_index, protocol,
			remote_sys_name, remote_port_id, remote_chassis_id, remote_mgmt_addr,
			stale, last_seen_at
		FROM port_neighbors
		WHERE device_id = $1 AND if_index = ANY($2) AND NOT stale
		ORDER BY if_index, protocol, rem_index`, deviceID, ifIndexes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PortNeighbor
	for rows.Next() {
		var n PortNeighbor
		if err := rows.Scan(
			&n.DeviceID, &n.IfIndex, &n.RemIndex, &n.Protocol,
			&n.RemoteSysName, &n.RemotePortID, &n.RemoteChassisID, &n.RemoteMgmtAddr,
			&n.Stale, &n.LastSeenAt,
		); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return CollapsePortNeighborsByIdentity(out), nil
}

// ListRecentDeviceEvents — события по списку узлов и типов (в т.ч. device-level, if_index IS NULL).
func (s *Store) ListRecentDeviceEvents(ctx context.Context, deviceIDs []int64, eventTypes []string, since time.Time, limit int) ([]EventBrief, error) {
	if len(deviceIDs) == 0 || len(eventTypes) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, device_id, if_index, event_type, severity, created_at, payload
		FROM events
		WHERE device_id = ANY($1)
		  AND event_type = ANY($2)
		  AND created_at >= $3
		ORDER BY created_at DESC
		LIMIT $4`, deviceIDs, eventTypes, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEventBriefs(rows)
}

// ListCorrelatedPortEvents — LINK_*/util на портах за окно.
func (s *Store) ListCorrelatedPortEvents(ctx context.Context, deviceID int64, ifIndexes []int, since time.Time, limit int) ([]EventBrief, error) {
	if len(ifIndexes) == 0 {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, device_id, if_index, event_type, severity, created_at, payload
		FROM events
		WHERE device_id = $1
		  AND if_index = ANY($2)
		  AND created_at >= $3
		  AND event_type IN (
		    'LINK_UP','LINK_DOWN','PORT_UTILIZATION_HIGH','PORT_UTILIZATION_OK',
		    'PORT_SPEED_DOWN','PORT_SPEED_OK','MAC_FLAPPING','MAC_MOVED','MAC_MULTI_ACCESS'
		  )
		ORDER BY created_at DESC
		LIMIT $4`, deviceID, ifIndexes, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEventBriefs(rows)
}

// FlappingMACSummary — MAC с частыми перемещениями за окно (для списка расследования).
type FlappingMACSummary struct {
	MAC                 string    `json:"mac"`
	MoveCount           int       `json:"move_count"`
	DeviceCount         int       `json:"device_count"`
	LastSeenAt          time.Time `json:"last_seen_at"`
	MacVendor           string    `json:"mac_vendor,omitempty"`
	HasFlapEvent        bool      `json:"has_flap_event"`
	InvestigationStatus string    `json:"investigation_status,omitempty"`
}

// ListFlappingMACs — топ MAC по числу смен порта за since.
// excludeWiFiPrefix — если задан, MAC с ARP-IP в этой подсети не попадают в список.
func (s *Store) ListFlappingMACs(ctx context.Context, since time.Time, minMoves, limit int, excludeWiFiPrefix *string) ([]FlappingMACSummary, error) {
	if minMoves < 2 {
		minMoves = 2
	}
	if limit <= 0 || limit > 100 {
		limit = 40
	}
	rows, err := s.pool.Query(ctx, `
		SELECT m.mac,
		       COUNT(*)::int AS move_count,
		       COUNT(DISTINCT m.device_id)::int AS device_count,
		       MAX(m.seen_at) AS last_seen,
		       EXISTS (
		         SELECT 1 FROM events e
		         WHERE e.event_type IN ('MAC_FLAPPING','MAC_MULTI_ACCESS')
		           AND e.created_at >= $1
		           AND lower(COALESCE(e.payload->>'mac','')) = m.mac
		       ) AS has_flap_event
		FROM mac_fdb_moves m
		WHERE m.seen_at >= $1
		  AND m.from_if_index IS NOT NULL
		  AND m.to_if_index IS NOT NULL
		  AND m.from_if_index <> m.to_if_index
		  AND (
		    $4::text IS NULL OR NOT EXISTS (
		      SELECT 1 FROM device_arp_entries a
		      WHERE lower(a.mac) = lower(m.mac)
		        AND a.ip ~ '^[0-9]+\\.[0-9]+\\.[0-9]+\\.[0-9]+$'
		        AND a.ip::inet << $4::cidr
		    )
		  )
		GROUP BY m.mac
		HAVING COUNT(*) >= $2
		ORDER BY move_count DESC, last_seen DESC
		LIMIT $3`, since, minMoves, limit, excludeWiFiPrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FlappingMACSummary
	for rows.Next() {
		var r FlappingMACSummary
		if err := rows.Scan(&r.MAC, &r.MoveCount, &r.DeviceCount, &r.LastSeenAt, &r.HasFlapEvent); err != nil {
			return nil, err
		}
		r.MacVendor = macvendor.Lookup(r.MAC)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	settings, err := s.GetMACInvestigationSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !settings.TrackWiFiClients {
		filtered := make([]FlappingMACSummary, 0, len(out))
		for _, r := range out {
			skip, err := s.ShouldSkipWiFiMACTracking(ctx, r.MAC)
			if err != nil {
				return nil, err
			}
			if !skip {
				filtered = append(filtered, r)
			}
		}
		out = filtered
	}
	macs := make([]string, len(out))
	for i, r := range out {
		macs[i] = r.MAC
	}
	statusMap, err := s.MapMACInvestigationStatuses(ctx, macs)
	if err != nil {
		return out, nil
	}
	for i := range out {
		if st, ok := statusMap[out[i].MAC]; ok {
			out[i].InvestigationStatus = st.Status
		} else {
			out[i].InvestigationStatus = MACInvestigationOpen
		}
	}
	return out, nil
}
