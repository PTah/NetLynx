package poller

import (
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// computeMACFDBMoves — diff prev→cur FDB (все порты): appear / move / leave.
func computeMACFDBMoves(
	deviceID int64,
	prev, cur map[string]int,
	entries map[string]snmp.FDBLearnedEntry,
	seenAt time.Time,
	source string,
) []store.MACFDBMoveInsert {
	if source == "" {
		source = store.MACMoveSourceFDBPoll
	}
	normPrev := normalizeIfMap(prev)
	normCur := normalizeIfMap(cur)
	out := make([]store.MACFDBMoveInsert, 0)
	for mac, ifIndex := range normCur {
		if mac == "" || ifIndex <= 0 {
			continue
		}
		oldIf, had := normPrev[mac]
		if !had {
			to := ifIndex
			out = append(out, store.MACFDBMoveInsert{
				MAC: mac, DeviceID: deviceID, ToIfIndex: &to,
				VLANID: vlanOf(entries, mac), SeenAt: seenAt, Source: source,
			})
			continue
		}
		if oldIf == ifIndex {
			continue
		}
		from, to := oldIf, ifIndex
		out = append(out, store.MACFDBMoveInsert{
			MAC: mac, DeviceID: deviceID, FromIfIndex: &from, ToIfIndex: &to,
			VLANID: vlanOf(entries, mac), SeenAt: seenAt, Source: source,
		})
	}
	for mac, oldIf := range normPrev {
		if mac == "" || oldIf <= 0 {
			continue
		}
		if _, still := normCur[mac]; still {
			continue
		}
		from := oldIf
		out = append(out, store.MACFDBMoveInsert{
			MAC: mac, DeviceID: deviceID, FromIfIndex: &from,
			SeenAt: seenAt, Source: source,
		})
	}
	return out
}

func normalizeIfMap(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for mac, ifIndex := range in {
		m := strings.ToLower(strings.TrimSpace(mac))
		if m == "" {
			continue
		}
		out[m] = ifIndex
	}
	return out
}

func vlanOf(entries map[string]snmp.FDBLearnedEntry, mac string) *int {
	if ent, ok := entries[mac]; ok {
		return ent.VLANID
	}
	for k, ent := range entries {
		if strings.EqualFold(k, mac) {
			return ent.VLANID
		}
	}
	return nil
}
