package poller

import (
	"context"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/macvendor"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func (e *Engine) recordAndDetectMACMoves(
	ctx context.Context,
	ignore map[int]store.PortEventIgnore,
	d store.PollDevice,
	prev, cur map[string]int,
	entries map[string]snmp.FDBLearnedEntry,
	ifs map[int]store.InterfaceSnapshot,
	now time.Time,
) {
	if len(prev) == 0 {
		return
	}
	moves := computeMACFDBMoves(d.ID, prev, cur, entries, now, store.MACMoveSourceFDBPoll)
	if len(moves) == 0 {
		return
	}
	filtered := moves[:0]
	for _, m := range moves {
		if e.shouldTrackMAC(ctx, m.MAC) {
			filtered = append(filtered, m)
		}
	}
	moves = filtered
	if len(moves) == 0 {
		return
	}
	if err := e.st.InsertMACFDBMoves(ctx, moves); err != nil {
		e.log.Warn("mac fdb moves insert", "device_id", d.ID, "err", err)
		return
	}
	seen := map[string]struct{}{}
	for _, m := range moves {
		if m.FromIfIndex == nil || m.ToIfIndex == nil {
			continue
		}
		if _, ok := seen[m.MAC]; ok {
			continue
		}
		seen[m.MAC] = struct{}{}
		e.maybeEmitMACFlapping(ctx, ignore, d, m.MAC, ifs, now)
	}
}

func (e *Engine) maybeEmitMACFlapping(
	ctx context.Context,
	ignore map[int]store.PortEventIgnore,
	d store.PollDevice,
	mac string,
	ifs map[int]store.InterfaceSnapshot,
	now time.Time,
) {
	if !e.shouldTrackMAC(ctx, mac) {
		return
	}
	k := e.cfg.MACFlapMinMoves
	if k < 2 {
		k = 3
	}
	window := e.cfg.MACFlapWindow
	if window <= 0 {
		window = time.Hour
	}
	debounce := e.cfg.MACFlapDebounce
	if debounce <= 0 {
		debounce = 15 * time.Minute
	}
	since := now.Add(-window)
	n, err := e.st.CountMACPortMovesInWindow(ctx, d.ID, mac, since)
	if err != nil {
		e.log.Warn("mac flap count", "device_id", d.ID, "mac", mac, "err", err)
		return
	}
	if n < k {
		return
	}
	dup, err := e.st.HasMACEventSince(ctx, d.ID, "MAC_FLAPPING", mac, now.Add(-debounce))
	if err != nil {
		e.log.Warn("mac flap debounce", "err", err)
		return
	}
	if dup {
		return
	}
	ports, _ := e.st.DistinctPortsInMoves(ctx, d.ID, mac, since)
	pl := map[string]interface{}{
		"mac":         mac,
		"move_count":  n,
		"window_sec":  int(window.Seconds()),
		"ports":       ports,
		"source":      "fdb_poll",
		"mac_vendor":  macvendor.Lookup(mac),
		"investigate": "/investigate/mac?mac=" + mac,
	}
	var ifIdx *int
	if len(ports) > 0 {
		idx := ports[len(ports)-1]
		ifIdx = &idx
		mergeIfacePayload(pl, ifs, idx)
	}
	e.emit(ctx, ignore, d, ifIdx, "MAC_FLAPPING", "warning", pl)
}

func (e *Engine) scanMACMultiAccess(ctx context.Context) {
	hits, err := e.st.ListMACMultiAccessCandidates(ctx)
	if err != nil {
		e.log.Warn("mac multi-access scan", "err", err)
		return
	}
	if len(hits) == 0 {
		return
	}
	byMAC := map[string][]store.MACAccessSighting{}
	for _, h := range hits {
		byMAC[h.MAC] = append(byMAC[h.MAC], h)
	}
	debounce := e.cfg.MACFlapDebounce
	if debounce <= 0 {
		debounce = 15 * time.Minute
	}
	now := time.Now()
	devs, err := e.st.ListPollDevices(ctx)
	if err != nil {
		return
	}
	devByID := map[int64]store.PollDevice{}
	for _, d := range devs {
		devByID[d.ID] = d
	}
	for mac, list := range byMAC {
		if len(list) < 2 {
			continue
		}
		if !e.shouldTrackMAC(ctx, mac) {
			continue
		}
		// Глобальный дедуп по MAC: не эмитить повторно в окне debounce
		// (раньше HasMACEventSince только по list[0].DeviceID → дубли на разные свитчи).
		if v, ok := e.macMultiAccessLastEmit.Load(mac); ok {
			if last, ok := v.(time.Time); ok && now.Sub(last) < debounce {
				continue
			}
		}
		d, ok := devByID[list[0].DeviceID]
		if !ok {
			continue
		}
		dup, err := e.st.HasMACEventSince(ctx, d.ID, "MAC_MULTI_ACCESS", mac, now.Add(-debounce))
		if err != nil || dup {
			e.macMultiAccessLastEmit.Store(mac, now)
			continue
		}
		locs := make([]map[string]interface{}, 0, len(list))
		for _, h := range list {
			locs = append(locs, map[string]interface{}{
				"device_id": h.DeviceID,
				"if_index":  h.IfIndex,
			})
		}
		idx := list[0].IfIndex
		pl := map[string]interface{}{
			"mac":         mac,
			"locations":   locs,
			"count":       len(list),
			"mac_vendor":  macvendor.Lookup(mac),
			"investigate": "/investigate/mac?mac=" + mac,
		}
		e.emit(ctx, nil, d, &idx, "MAC_MULTI_ACCESS", "warning", pl)
		e.macMultiAccessLastEmit.Store(mac, now)
	}
}

func (e *Engine) pruneMACMoves(ctx context.Context) {
	e.pruneMACMultiAccessLastEmit()
	days := e.cfg.MACMovesRetentionDays
	if days <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	if n, err := e.st.PruneMACFDBMoves(ctx, cutoff); err != nil {
		e.log.Warn("prune mac moves", "err", err)
	} else if n > 0 {
		e.log.Info("prune mac moves", "deleted", n)
	}
}

// pruneMACMultiAccessLastEmit сбрасывает устаревшие метки дедупа MAC_MULTI_ACCESS.
func (e *Engine) pruneMACMultiAccessLastEmit() {
	debounce := e.cfg.MACFlapDebounce
	if debounce <= 0 {
		debounce = 15 * time.Minute
	}
	cutoff := time.Now().Add(-2 * debounce)
	e.macMultiAccessLastEmit.Range(func(key, value any) bool {
		if t, ok := value.(time.Time); ok && t.Before(cutoff) {
			e.macMultiAccessLastEmit.Delete(key)
		}
		return true
	})
}

// EmitMACFlappingFromSyslog — realtime flap из syslogrecv.
func (e *Engine) EmitMACFlappingFromSyslog(
	ctx context.Context,
	deviceID int64,
	mac string,
	portA, portB int,
	vlan *int,
	rawMsg string,
) {
	d, err := e.st.GetPollDevice(ctx, deviceID)
	if err != nil || d == nil {
		e.log.Warn("syslog mac flap: device", "device_id", deviceID, "err", err)
		return
	}
	if !e.shouldTrackMAC(ctx, mac) {
		return
	}
	now := time.Now()
	from, to := portA, portB
	moves := []store.MACFDBMoveInsert{{
		MAC: mac, DeviceID: deviceID, FromIfIndex: &from, ToIfIndex: &to,
		VLANID: vlan, SeenAt: now, Source: store.MACMoveSourceSyslog,
	}}
	if err := e.st.InsertMACFDBMoves(ctx, moves); err != nil {
		e.log.Warn("syslog mac moves", "err", err)
	}
	debounce := e.cfg.MACFlapDebounce
	if debounce <= 0 {
		debounce = 15 * time.Minute
	}
	dup, err := e.st.HasMACEventSince(ctx, deviceID, "MAC_FLAPPING", mac, now.Add(-debounce))
	if err != nil || dup {
		return
	}
	pl := map[string]interface{}{
		"mac":         strings.ToLower(mac),
		"ports":       []int{portA, portB},
		"source":      "syslog",
		"mac_vendor":  macvendor.Lookup(mac),
		"investigate": "/investigate/mac?mac=" + strings.ToLower(mac),
	}
	if vlan != nil {
		pl["vlan_id"] = *vlan
	}
	if rawMsg != "" {
		pl["syslog_msg"] = rawMsg
	}
	ifIdx := portA
	e.emit(ctx, nil, *d, &ifIdx, "MAC_FLAPPING", "warning", pl)
}
