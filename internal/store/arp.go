package store

import (
	"context"
	"strings"
	"time"
)

// ARPEntry снимок ARP на узле.
type ARPEntry struct {
	IP      string
	MAC     string
	IfIndex int
}

func (s *Store) ReplaceARPSnapshot(ctx context.Context, deviceID int64, entries []ARPEntry, at time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM device_arp_entries WHERE device_id = $1`, deviceID); err != nil {
		return err
	}
	for _, e := range entries {
		ip := strings.TrimSpace(e.IP)
		mac := strings.ToLower(strings.TrimSpace(e.MAC))
		if ip == "" || mac == "" {
			continue
		}
		var ifIdx *int
		if e.IfIndex > 0 {
			v := e.IfIndex
			ifIdx = &v
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO device_arp_entries (device_id, ip, mac, if_index, last_seen_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $5)`,
			deviceID, ip, mac, ifIdx, at); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListDistinctARPMACsForHost — уникальные MAC из ARP-снимков L3-узлов по IP/host inventory.
func (s *Store) ListDistinctARPMACsForHost(ctx context.Context, host string) ([]string, error) {
	hexKeys, err := s.listARPMACHexForHost(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(hexKeys))
	for _, hex := range hexKeys {
		if mac, ok := FormatFullMAC(hex); ok {
			out = append(out, mac)
		}
	}
	return out, nil
}
