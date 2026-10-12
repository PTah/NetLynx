package postmortem

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// ScopeDevice — узел в области postmortem (центр + LLDP-соседи).
type ScopeDevice struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	Hop  int    `json:"hop"`
}

// TimelineItem — одна строка общего таймлайна.
type TimelineItem struct {
	At         time.Time              `json:"at"`
	Kind       string                 `json:"kind"` // event | trap | mac_move | config_snapshot
	DeviceID   int64                  `json:"device_id"`
	DeviceName string                 `json:"device_name,omitempty"`
	Summary    string                 `json:"summary"`
	Severity   string                 `json:"severity,omitempty"`
	IfIndex    *int                   `json:"if_index,omitempty"`
	Detail     map[string]interface{} `json:"detail,omitempty"`
}

// Report — postmortem вокруг момента на узле.
type Report struct {
	DeviceID     int64         `json:"device_id"`
	DeviceName   string        `json:"device_name"`
	DeviceHost   string        `json:"device_host"`
	Center       time.Time     `json:"center"`
	Window       string        `json:"window"`
	From         time.Time     `json:"from"`
	To           time.Time     `json:"to"`
	Hops         int           `json:"hops"`
	ScopeDevices []ScopeDevice `json:"scope_devices"`
	Timeline     []TimelineItem `json:"timeline"`
}

type Builder struct {
	St *store.Store
}

func (b Builder) Build(ctx context.Context, deviceID int64, center time.Time, window time.Duration, hops int) (*Report, error) {
	if b.St == nil {
		return nil, fmt.Errorf("store: nil")
	}
	if deviceID <= 0 {
		return nil, fmt.Errorf("device_id: обязателен")
	}
	if center.IsZero() {
		center = time.Now()
	}
	if window <= 0 {
		window = 5 * time.Minute
	}
	if window > 24*time.Hour {
		window = 24 * time.Hour
	}
	if hops < 0 {
		hops = 0
	}
	if hops > 3 {
		hops = 3
	}
	from := center.Add(-window / 2)
	to := center.Add(window / 2)

	dev, err := b.St.GetDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	if dev == nil {
		return nil, store.ErrDeviceNotFound
	}

	scope, deviceIDs, names := collectScope(ctx, b.St, deviceID, dev.Name, dev.Host, hops)

	events, err := b.St.ListEventsForDevicesInWindow(ctx, deviceIDs, from, to, 300)
	if err != nil {
		return nil, err
	}
	traps, err := b.St.ListSNMPTrapLogsInWindow(ctx, deviceIDs, from, to, 100)
	if err != nil {
		return nil, err
	}
	moves, err := b.St.ListMACMovesForDevicesInWindow(ctx, deviceIDs, from, to, 100)
	if err != nil {
		return nil, err
	}
	excludeWiFi, _ := b.St.WiFiTrackingExcludePrefix(ctx)
	wifiMACs := map[string]struct{}{}
	if excludeWiFi != nil {
		if set, err := b.St.ListMACsInIPPrefix(ctx, *excludeWiFi); err == nil {
			wifiMACs = set
		}
	}
	skipWiFiMAC := func(mac string) bool {
		if excludeWiFi == nil {
			return false
		}
		if norm, ok := store.FormatFullMAC(mac); ok {
			_, hit := wifiMACs[strings.ToLower(norm)]
			return hit
		}
		return false
	}

	var timeline []TimelineItem
	for _, e := range events {
		if skipWiFiMAC(store.EventPayloadMAC(e.EventType, e.Payload)) {
			continue
		}
		timeline = append(timeline, TimelineItem{
			At:         e.CreatedAt,
			Kind:       "event",
			DeviceID:   e.DeviceID,
			DeviceName: names[e.DeviceID],
			Summary:    e.EventType,
			Severity:   e.Severity,
			IfIndex:    e.IfIndex,
			Detail: map[string]interface{}{
				"event_id":   e.ID,
				"event_type": e.EventType,
				"payload":    e.Payload,
			},
		})
	}
	for _, t := range traps {
		devID := int64(0)
		if t.DeviceID != nil {
			devID = *t.DeviceID
		}
		summary := t.TrapOID
		if summary == "" {
			summary = "SNMP trap"
		}
		timeline = append(timeline, TimelineItem{
			At:         t.ReceivedAt,
			Kind:       "trap",
			DeviceID:   devID,
			DeviceName: names[devID],
			Summary:    summary,
			IfIndex:    t.IfIndex,
			Detail: map[string]interface{}{
				"trap_id":   t.ID,
				"source_ip": t.SourceIP,
				"trap_oid":  t.TrapOID,
				"payload":   t.Payload,
			},
		})
	}
	for _, m := range moves {
		if skipWiFiMAC(m.MAC) {
			continue
		}
		summary := fmt.Sprintf("MAC %s", m.MAC)
		if m.FromIfIndex != nil && m.ToIfIndex != nil {
			summary = fmt.Sprintf("MAC %s: %d → %d", m.MAC, *m.FromIfIndex, *m.ToIfIndex)
		}
		timeline = append(timeline, TimelineItem{
			At:         m.SeenAt,
			Kind:       "mac_move",
			DeviceID:   m.DeviceID,
			DeviceName: names[m.DeviceID],
			Summary:    summary,
			Detail: map[string]interface{}{
				"move_id":       m.ID,
				"mac":           m.MAC,
				"from_if_index": m.FromIfIndex,
				"to_if_index":   m.ToIfIndex,
				"source":        m.Source,
			},
		})
	}

	snaps, err := b.St.ListConfigSnapshotsInWindow(ctx, deviceID, from, to, 10)
	if err != nil {
		return nil, err
	}
	for _, snap := range snaps {
		timeline = append(timeline, TimelineItem{
			At:         snap.FetchedAt,
			Kind:       "config_snapshot",
			DeviceID:   snap.DeviceID,
			DeviceName: names[snap.DeviceID],
			Summary:    fmt.Sprintf("Config snapshot (%s)", snap.Source),
			Detail: map[string]interface{}{
				"snapshot_id": snap.ID,
				"hash":        snap.ConfigHash,
				"byte_size":   snap.ByteSize,
				"source":      snap.Source,
			},
		})
	}

	sort.Slice(timeline, func(i, j int) bool {
		if timeline[i].At.Equal(timeline[j].At) {
			return timeline[i].Kind < timeline[j].Kind
		}
		return timeline[i].At.Before(timeline[j].At)
	})
	if len(timeline) > 400 {
		timeline = timeline[:400]
	}

	return &Report{
		DeviceID:     deviceID,
		DeviceName:   dev.Name,
		DeviceHost:   dev.Host,
		Center:       center,
		Window:       window.String(),
		From:         from,
		To:           to,
		Hops:         hops,
		ScopeDevices: scope,
		Timeline:     timeline,
	}, nil
}

func collectScope(ctx context.Context, st *store.Store, centerID int64, centerName, centerHost string, maxHops int) ([]ScopeDevice, []int64, map[int64]string) {
	type q struct {
		id   int64
		hop  int
		name string
		host string
	}
	seen := map[int64]bool{centerID: true}
	names := map[int64]string{centerID: centerName}
	scope := []ScopeDevice{{ID: centerID, Name: centerName, Host: centerHost, Hop: 0}}
	queue := []q{{id: centerID, hop: 0, name: centerName, host: centerHost}}
	deviceIDs := []int64{centerID}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.hop >= maxHops {
			continue
		}
		neighbors, err := st.ListPortNeighbors(ctx, cur.id)
		if err != nil {
			continue
		}
		neighbors, err = st.EnrichNeighborsRemoteDeviceID(ctx, neighbors)
		if err != nil {
			continue
		}
		for _, nb := range neighbors {
			if nb.Stale || nb.RemoteDeviceID == nil || *nb.RemoteDeviceID <= 0 {
				continue
			}
			rid := *nb.RemoteDeviceID
			if seen[rid] {
				continue
			}
			seen[rid] = true
			name := ""
			host := ""
			if d, err := st.GetDevice(ctx, rid); err == nil && d != nil {
				name = d.Name
				host = d.Host
			}
			if name == "" {
				if nb.RemoteSysName != nil {
					name = *nb.RemoteSysName
				} else {
					name = fmt.Sprintf("device %d", rid)
				}
			}
			names[rid] = name
			scope = append(scope, ScopeDevice{ID: rid, Name: name, Host: host, Hop: cur.hop + 1})
			deviceIDs = append(deviceIDs, rid)
			queue = append(queue, q{id: rid, hop: cur.hop + 1, name: name, host: host})
		}
	}
	return scope, deviceIDs, names
}
