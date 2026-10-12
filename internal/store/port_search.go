package store

import (
	"context"
	"fmt"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/macvendor"
)

// PortSearchHit — результат поиска порта на узлах.
type PortSearchHit struct {
	DeviceID   int64   `json:"device_id"`
	DeviceName string  `json:"device_name"`
	DeviceHost string  `json:"device_host"`
	Location   *string `json:"location,omitempty"`
	IfIndex    int     `json:"if_index"`
	IfName     *string `json:"if_name,omitempty"`
	IfDescr    *string `json:"if_descr,omitempty"`
	PortRole   string  `json:"port_role"`
	MatchType  string  `json:"match_type"`
	MAC        *string `json:"mac,omitempty"`
	MacVendor  string  `json:"mac_vendor,omitempty"`
	IP         *string `json:"ip,omitempty"`
	ArpIfIndex *int    `json:"arp_if_index,omitempty"`
	Note       *string `json:"note,omitempty"`
}

func escapeILIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// SearchPorts ищет по подписи, MAC (FDB) или IP (ARP → MAC → FDB).
func (s *Store) SearchPorts(ctx context.Context, query string, limit int) ([]PortSearchHit, error) {
	kind, norm := ClassifySearchQuery(query)
	if norm == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var hits []PortSearchHit
	var err error
	switch kind {
	case SearchQueryIP:
		hits, err = s.searchPortsByIP(ctx, norm, limit)
	case SearchQueryMAC:
		hits, err = s.searchPortsByMAC(ctx, norm, limit)
	default:
		hits, err = s.searchPortsByLabel(ctx, norm, limit)
	}
	if err != nil {
		return nil, err
	}
	attachMACVendors(hits)
	return hits, nil
}

func attachMACVendors(hits []PortSearchHit) {
	for i := range hits {
		if hits[i].MAC == nil {
			continue
		}
		hits[i].MacVendor = macvendor.Lookup(*hits[i].MAC)
	}
}

func (s *Store) searchPortsByLabel(ctx context.Context, q string, limit int) ([]PortSearchHit, error) {
	pattern := "%" + escapeILIKE(q) + "%"
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.name, d.host, d.location,
		       di.if_index, di.if_name, COALESCE(NULLIF(btrim(di.descr_override), ''), NULLIF(btrim(di.cli_description), ''), di.if_descr) AS if_descr, di.port_role
		FROM device_interfaces di
		INNER JOIN devices d ON d.id = di.device_id
		WHERE di.if_descr ILIKE $1 ESCAPE '\'
		   OR di.if_name ILIKE $1 ESCAPE '\'
		   OR di.descr_override ILIKE $1 ESCAPE '\'
		   OR di.cli_description ILIKE $1 ESCAPE '\'
		ORDER BY d.name ASC, di.if_index ASC
		LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPortSearchRows(rows, "label", limit)
}

func (s *Store) searchPortsByMAC(ctx context.Context, mac string, limit int) ([]PortSearchHit, error) {
	exact := mac
	macHex := macHexDigits(mac)
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.name, d.host, d.location,
		       f.if_index, di.if_name, COALESCE(NULLIF(btrim(di.descr_override), ''), NULLIF(btrim(di.cli_description), ''), di.if_descr) AS if_descr, di.port_role,
		       f.mac,
		       EXISTS (
		         SELECT 1 FROM port_neighbors pn
		         WHERE pn.device_id = f.device_id
		           AND pn.if_index = f.if_index
		           AND NOT pn.stale
		           AND (
		             lower(replace(replace(COALESCE(pn.remote_chassis_id, ''), ':', ''), '-', '')) = $2
		             OR lower(replace(replace(COALESCE(pn.remote_port_id, ''), ':', ''), '-', '')) = $2
		           )
		       ) AS lldp_hit,
		       (
		         SELECT COUNT(*)::int FROM device_fdb_entries fx
		         WHERE fx.device_id = f.device_id AND fx.if_index = f.if_index
		       ) AS macs_on_port
		FROM device_fdb_entries f
		INNER JOIN devices d ON d.id = f.device_id
		LEFT JOIN device_interfaces di ON di.device_id = f.device_id AND di.if_index = f.if_index
		WHERE f.mac = $1
		ORDER BY lldp_hit DESC,
		         macs_on_port ASC,
		         d.name ASC,
		         f.if_index ASC
		LIMIT $3`, exact, macHex, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PortSearchHit
	for rows.Next() {
		var h PortSearchHit
		var macVal string
		var lldpHit bool
		var macsOnPort int
		if err := rows.Scan(
			&h.DeviceID, &h.DeviceName, &h.DeviceHost, &h.Location,
			&h.IfIndex, &h.IfName, &h.IfDescr, &h.PortRole,
			&macVal, &lldpHit, &macsOnPort,
		); err != nil {
			return nil, err
		}
		h.MAC = &macVal
		if lldpHit {
			h.MatchType = "lldp"
			n := "прямой сосед по LLDP (физический порт)"
			h.Note = &n
		} else {
			h.MatchType = "mac"
			if macsOnPort >= 8 {
				n := "FDB на порту с многими MAC (часто uplink/trunk в том же VLAN)"
				h.Note = &n
			} else {
				n := "запись FDB (MAC виден на свитче; не обязательно прямой кабель)"
				h.Note = &n
			}
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) searchPortsByIP(ctx context.Context, ip string, limit int) ([]PortSearchHit, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.name, d.host, d.location,
		       a.ip, a.mac, a.if_index
		FROM device_arp_entries a
		INNER JOIN devices d ON d.id = a.device_id
		WHERE a.ip = $1
		ORDER BY d.name ASC
		LIMIT $2`, ip, limit*3)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type arpRow struct {
		deviceID   int64
		deviceName string
		deviceHost string
		location   *string
		ip         string
		mac        string
		arpIf      *int
	}
	var arps []arpRow
	for rows.Next() {
		var r arpRow
		if err := rows.Scan(&r.deviceID, &r.deviceName, &r.deviceHost, &r.location, &r.ip, &r.mac, &r.arpIf); err != nil {
			return nil, err
		}
		arps = append(arps, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(arps) == 0 {
		return nil, nil
	}

	var out []PortSearchHit
	seen := make(map[string]struct{})
	for _, a := range arps {
		fdbRows, err := s.pool.Query(ctx, `
			SELECT f.device_id, d.name, d.host, d.location, f.if_index,
			       di.if_name, COALESCE(NULLIF(btrim(di.descr_override), ''), NULLIF(btrim(di.cli_description), ''), di.if_descr) AS if_descr, di.port_role, f.mac,
			       (f.device_id = $2) AS same_dev
			FROM device_fdb_entries f
			INNER JOIN devices d ON d.id = f.device_id
			LEFT JOIN device_interfaces di ON di.device_id = f.device_id AND di.if_index = f.if_index
			WHERE f.mac = $1
			ORDER BY same_dev DESC, d.name ASC, f.if_index ASC
			LIMIT 20`, a.mac, a.deviceID)
		if err != nil {
			return nil, err
		}
		fdbFound := false
		for fdbRows.Next() {
			fdbFound = true
			var h PortSearchHit
			var macVal string
			var sameDev bool
			if err := fdbRows.Scan(
				&h.DeviceID, &h.DeviceName, &h.DeviceHost, &h.Location,
				&h.IfIndex, &h.IfName, &h.IfDescr, &h.PortRole, &macVal, &sameDev,
			); err != nil {
				fdbRows.Close()
				return nil, err
			}
			key := fmt.Sprintf("%d:%d:%s:%s", h.DeviceID, h.IfIndex, a.ip, a.mac)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			h.MatchType = "ip"
			h.IP = &a.ip
			h.MAC = &macVal
			h.ArpIfIndex = a.arpIf
			if a.arpIf != nil && *a.arpIf != h.IfIndex {
				n := fmt.Sprintf("ARP ifIndex %d; порт из FDB %d", *a.arpIf, h.IfIndex)
				h.Note = &n
			}
			out = append(out, h)
			if len(out) >= limit {
				fdbRows.Close()
				return out, nil
			}
		}
		fdbRows.Close()
		if err := fdbRows.Err(); err != nil {
			return nil, err
		}
		if !fdbFound {
			key := fmt.Sprintf("arp:%d:%s:%s", a.deviceID, a.ip, a.mac)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			ifIdx := 0
			if a.arpIf != nil {
				ifIdx = *a.arpIf
			}
			note := "MAC в ARP; в FDB на коммутаторах не найден (опрос FDB или другой свич)"
			h := PortSearchHit{
				DeviceID: a.deviceID, DeviceName: a.deviceName, DeviceHost: a.deviceHost,
				Location: a.location, IfIndex: ifIdx, PortRole: "—",
				MatchType: "ip", IP: &a.ip, MAC: &a.mac, ArpIfIndex: a.arpIf, Note: &note,
			}
			out = append(out, h)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func scanPortSearchRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}, matchType string, limit int) ([]PortSearchHit, error) {
	defer rows.Close()
	var out []PortSearchHit
	for rows.Next() {
		var h PortSearchHit
		if err := rows.Scan(
			&h.DeviceID, &h.DeviceName, &h.DeviceHost, &h.Location,
			&h.IfIndex, &h.IfName, &h.IfDescr, &h.PortRole,
		); err != nil {
			return nil, err
		}
		h.MatchType = matchType
		out = append(out, h)
		if len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}
