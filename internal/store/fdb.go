package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/macvendor"
	"github.com/jackc/pgx/v5"
)

// FDBLearnedEntry одна запись FDB при опросе.
type FDBLearnedEntry struct {
	IfIndex int
	VLANID  *int
}

// PortClient устройство за портом (FDB + IP из ARP любого узла по MAC).
type PortClient struct {
	MAC                string    `json:"mac"`
	VLANID             *int      `json:"vlan_id,omitempty"`
	IP                 *string   `json:"ip,omitempty"`  // первый IP (совместимость)
	IPs                []string  `json:"ips,omitempty"` // все известные IP
	MacVendor          string    `json:"mac_vendor,omitempty"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	ExistingDeviceID   *int64    `json:"existing_device_id,omitempty"`
	ExistingDeviceName *string   `json:"existing_device_name,omitempty"`
}

func (s *Store) ReplaceFDBSnapshot(ctx context.Context, deviceID int64, snapshot map[string]FDBLearnedEntry, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	macs := make([]string, 0, len(snapshot))
	for mac, ent := range snapshot {
		m := strings.ToLower(strings.TrimSpace(mac))
		if m == "" || ent.IfIndex <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO device_fdb_entries (device_id, mac, if_index, vlan_id, first_seen_at, last_seen_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5, $5)
			ON CONFLICT (device_id, mac) DO UPDATE SET
				if_index = EXCLUDED.if_index,
				vlan_id = EXCLUDED.vlan_id,
				last_seen_at = EXCLUDED.last_seen_at,
				updated_at = EXCLUDED.updated_at,
				first_seen_at = CASE
					WHEN device_fdb_entries.if_index IS DISTINCT FROM EXCLUDED.if_index
					THEN EXCLUDED.first_seen_at
					ELSE device_fdb_entries.first_seen_at
				END`,
			deviceID, m, ent.IfIndex, ent.VLANID, at); err != nil {
			return err
		}
		macs = append(macs, m)
	}
	if len(macs) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM device_fdb_entries WHERE device_id = $1`, deviceID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			DELETE FROM device_fdb_entries
			WHERE device_id = $1 AND mac <> ALL($2)`, deviceID, macs); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE devices SET
			last_fdb_poll_at = $2,
			fdb_baseline_at = COALESCE(fdb_baseline_at, $2),
			updated_at = now()
		WHERE id = $1`, deviceID, at); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ListPortClients(ctx context.Context, deviceID int64, ifIndex int) ([]PortClient, error) {
	// IP ищем по MAC во всём ARP (шлюз/L3), не только на этом коммутаторе — EdgeSwitch обычно L2 без ARP клиентов.
	// Уже известный узел: chassis MAC, иначе host = ARP IP (не сам свитч).
	rows, err := s.pool.Query(ctx, `
		SELECT f.mac, f.vlan_id, f.last_seen_at,
			COALESCE((
				SELECT array_agg(DISTINCT a.ip ORDER BY a.ip)
				FROM device_arp_entries a
				WHERE a.mac = f.mac
			), '{}') AS ips,
			d.id, d.name
		FROM device_fdb_entries f
		LEFT JOIN LATERAL (
			SELECT id, name
			FROM devices
			WHERE id <> $1
			  AND (
				(
					chassis_mac IS NOT NULL AND btrim(chassis_mac) <> ''
					AND lower(replace(replace(chassis_mac, ':', ''), '-', ''))
					  = lower(replace(replace(f.mac, ':', ''), '-', ''))
				)
				OR (
					host IS NOT NULL AND btrim(host) <> ''
					AND EXISTS (
						SELECT 1 FROM device_arp_entries a
						WHERE a.mac = f.mac AND lower(btrim(a.ip)) = lower(btrim(devices.host))
					)
				)
			  )
			ORDER BY CASE
				WHEN chassis_mac IS NOT NULL AND btrim(chassis_mac) <> ''
				  AND lower(replace(replace(chassis_mac, ':', ''), '-', ''))
				    = lower(replace(replace(f.mac, ':', ''), '-', ''))
				THEN 0 ELSE 1 END
			LIMIT 1
		) d ON true
		WHERE f.device_id = $1 AND f.if_index = $2
		ORDER BY f.vlan_id NULLS LAST, f.mac ASC`, deviceID, ifIndex)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PortClient
	for rows.Next() {
		var c PortClient
		var ips []string
		if err := rows.Scan(&c.MAC, &c.VLANID, &c.LastSeenAt, &ips, &c.ExistingDeviceID, &c.ExistingDeviceName); err != nil {
			return nil, err
		}
		if len(ips) > 0 {
			c.IPs = ips
			first := ips[0]
			c.IP = &first
		}
		c.MacVendor = macvendor.Lookup(c.MAC)
		out = append(out, c)
	}
	return out, rows.Err()
}

// HasPortFDBEntry true, если MAC есть в текущем снимке FDB этого порта.
func (s *Store) HasPortFDBEntry(ctx context.Context, deviceID int64, ifIndex int, mac string) (bool, error) {
	full, ok := FormatFullMAC(mac)
	if !ok {
		return false, nil
	}
	hex := macHexDigits(full)
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT 1 FROM device_fdb_entries
		WHERE device_id = $1 AND if_index = $2
		  AND lower(replace(replace(mac, ':', ''), '-', '')) = $3
		LIMIT 1`, deviceID, ifIndex, hex).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetFDBEntryFirstSeen — first_seen_at MAC на свитче (для tie-break attachment).
func (s *Store) GetFDBEntryFirstSeen(ctx context.Context, switchID int64, macHex string) (time.Time, bool, error) {
	macHex = strings.ToLower(strings.TrimSpace(macHex))
	if switchID <= 0 || len(macHex) != 12 {
		return time.Time{}, false, nil
	}
	var ts time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT first_seen_at FROM device_fdb_entries
		WHERE device_id = $1
		  AND lower(replace(replace(mac, ':', ''), '-', '')) = $2`,
		switchID, macHex).Scan(&ts)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return ts, true, nil
}

// FDBVLANPort — VLAN, виденный на порту в live FDB.
type FDBVLANPort struct {
	IfIndex int
	VLANID  int
}

func (s *Store) ListFDBVLANPorts(ctx context.Context, deviceID int64) ([]FDBVLANPort, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT if_index, vlan_id
		FROM device_fdb_entries
		WHERE device_id = $1 AND vlan_id IS NOT NULL AND vlan_id BETWEEN 1 AND 4094
		GROUP BY if_index, vlan_id
		ORDER BY vlan_id, if_index`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FDBVLANPort
	for rows.Next() {
		var e FDBVLANPort
		if err := rows.Scan(&e.IfIndex, &e.VLANID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}