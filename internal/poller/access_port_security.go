package poller

import (
	"context"
	"sort"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func normalizeMAC(mac string) string {
	return strings.ToLower(strings.TrimSpace(mac))
}

func buildMacsByAccessPort(entries map[string]int, ifs map[int]store.InterfaceSnapshot, resolveRole func(int) string) map[int][]string {
	by := make(map[int][]string)
	for mac, ifIdx := range entries {
		if !isUnicastMAC(mac) {
			continue
		}
		if _, ok := ifs[ifIdx]; !ok {
			continue
		}
		if resolveRole(ifIdx) != "access" {
			continue
		}
		by[ifIdx] = append(by[ifIdx], normalizeMAC(mac))
	}
	for ifIx := range by {
		sort.Strings(by[ifIx])
	}
	return by
}

func mergeIfacePayload(pl map[string]interface{}, ifs map[int]store.InterfaceSnapshot, ifIdx int) {
	if s, ok := ifs[ifIdx]; ok {
		if s.IfName != nil && strings.TrimSpace(*s.IfName) != "" {
			pl["if_name"] = *s.IfName
		}
		if d := s.DisplayDescr(); d != "" {
			pl["if_descr"] = d
		}
	}
}

// processAccessPortSecurity — привязка MAC к access-порту, смена MAC (подозрение на вторжение), долгое простое порта.
func (e *Engine) processAccessPortSecurity(ctx context.Context, ignore map[int]store.PortEventIgnore, d store.PollDevice, prev, cur map[string]int, ifs map[int]store.InterfaceSnapshot, effectiveRoleByIf map[int]string, now time.Time) error {
	state, err := e.st.GetPortAccessSecurity(ctx, d.ID)
	if err != nil {
		return err
	}
	resolveRole := makeResolvePortRole(ifs, effectiveRoleByIf, nil, e.cfg)
	prevBy := buildMacsByAccessPort(prev, ifs, resolveRole)
	curBy := buildMacsByAccessPort(cur, ifs, resolveRole)

	for ifIdx := range ifs {
		if resolveRole(ifIdx) != "access" {
			continue
		}

		prevM := prevBy[ifIdx]
		curM := curBy[ifIdx]
		prevHad := len(prevM) > 0
		curHas := len(curM) > 0

		st, hasRow := state[ifIdx]
		var bound *string
		var emptySince *time.Time
		if hasRow {
			bound = st.BoundMAC
			emptySince = st.EmptySince
		}

		if !curHas {
			if prevHad {
				var es *time.Time
				if emptySince != nil {
					es = emptySince
				} else {
					t := now
					es = &t
				}
				if err := e.st.UpsertPortAccessSecurity(ctx, d.ID, ifIdx, bound, es); err != nil {
					return err
				}
			}
			continue
		}

		if len(curM) > 1 {
			if err := e.st.UpsertPortAccessSecurity(ctx, d.ID, ifIdx, bound, nil); err != nil {
				return err
			}
			continue
		}

		primary := curM[0]
		if !e.shouldTrackMAC(ctx, primary) {
			b := primary
			if err := e.st.UpsertPortAccessSecurity(ctx, d.ID, ifIdx, &b, nil); err != nil {
				return err
			}
			continue
		}

		idx := ifIdx
		if emptySince != nil && now.Sub(*emptySince) >= e.cfg.AccessPortLongIdle {
			pl := map[string]interface{}{
				"mac":        primary,
				"idle_hours": now.Sub(*emptySince).Hours(),
			}
			mergeIfacePayload(pl, ifs, ifIdx)
			e.emit(ctx, ignore, d, &idx, "ACCESS_PORT_LONG_IDLE_DEVICE", "info", pl)
		}

		if bound != nil && *bound != primary {
			pl := map[string]interface{}{
				"old_mac": *bound,
				"new_mac": primary,
			}
			mergeIfacePayload(pl, ifs, ifIdx)
			e.emit(ctx, ignore, d, &idx, "ACCESS_PORT_MAC_SUBSTITUTED", "warning", pl)
		}

		b := primary
		if err := e.st.UpsertPortAccessSecurity(ctx, d.ID, ifIdx, &b, nil); err != nil {
			return err
		}
	}
	return nil
}
