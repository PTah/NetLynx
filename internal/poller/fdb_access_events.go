package poller

import (
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

type fdbAccessEvent struct {
	eventType  string
	severity   string
	mac        string
	ifIndex    int
	oldIfIndex *int
}

func isInventoryChassisMAC(mac string, chassis map[string]store.ChassisEndpoint) bool {
	if len(chassis) == 0 {
		return false
	}
	macNorm, ok := store.FormatFullMAC(mac)
	if !ok {
		return false
	}
	hex := strings.ReplaceAll(strings.ToLower(macNorm), ":", "")
	if len(hex) != 12 {
		return false
	}
	_, ok = chassis[hex]
	return ok
}

// computeFDBAccessEvents — инциденты только на access-портах:
// появление на access, уход с access, переход access → access.
// MAC из inventory (chassis_mac) и привязанный MAC порта не дают UNKNOWN — меньше шума при пропадании из FDB.
// MAC из inventory не даёт MAC_REMOVED — меньше шума от FDB на uplink.
func computeFDBAccessEvents(
	prev, cur map[string]int,
	resolveRole func(int) string,
	isInventoryMAC func(string) bool,
	isKnownOnPort func(string, int) bool,
) []fdbAccessEvent {
	isAccess := func(ifIdx int) bool {
		return resolveRole(ifIdx) == "access"
	}
	skipUnknown := func(mac string, ifIndex int) bool {
		if isInventoryMAC != nil && isInventoryMAC(mac) {
			return true
		}
		if isKnownOnPort != nil && isKnownOnPort(mac, ifIndex) {
			return true
		}
		return false
	}
	var out []fdbAccessEvent

	for mac, ifIndex := range cur {
		if !isUnicastMAC(mac) {
			continue
		}
		if !isAccess(ifIndex) {
			continue
		}
		oldIf, had := prev[mac]
		if !had {
			if skipUnknown(mac, ifIndex) {
				continue
			}
			out = append(out, fdbAccessEvent{
				eventType: "UNKNOWN_MAC_ON_ACCESS_PORT",
				severity:  "warning",
				mac:       mac,
				ifIndex:   ifIndex,
			})
			continue
		}
		if oldIf == ifIndex {
			continue
		}
		oldRole := resolveRole(oldIf)
		switch oldRole {
		case "access":
			o := oldIf
			out = append(out, fdbAccessEvent{
				eventType:  "MAC_MOVED",
				severity:   "warning",
				mac:        mac,
				ifIndex:    ifIndex,
				oldIfIndex: &o,
			})
		case "trunk", "ignore":
			// Был на uplink/trunk — появился на access.
			if skipUnknown(mac, ifIndex) {
				continue
			}
			out = append(out, fdbAccessEvent{
				eventType: "UNKNOWN_MAC_ON_ACCESS_PORT",
				severity:  "warning",
				mac:       mac,
				ifIndex:   ifIndex,
			})
		}
	}

	for mac, oldIf := range prev {
		if !isUnicastMAC(mac) {
			continue
		}
		if !isAccess(oldIf) {
			continue
		}
		if isInventoryMAC != nil && isInventoryMAC(mac) {
			continue
		}
		newIf, stillPresent := cur[mac]
		if stillPresent && isAccess(newIf) {
			continue
		}
		out = append(out, fdbAccessEvent{
			eventType: "MAC_REMOVED",
			severity:  "info",
			mac:       mac,
			ifIndex:   oldIf,
		})
	}
	return out
}
