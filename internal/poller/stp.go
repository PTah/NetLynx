package poller

import (
	"context"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/gosnmp/gosnmp"
)

func (e *Engine) pollSTP(ctx context.Context, d store.PollDevice, g *gosnmp.GoSNMP) {
	cur, err := snmp.ReadBridgeSTP(g)
	if err != nil {
		e.log.Debug("stp mib", "device_id", d.ID, "err", err)
		return
	}
	prev, ok, err := e.st.GetDeviceSTPState(ctx, d.ID)
	if err != nil {
		e.log.Warn("stp state load", "device_id", d.ID, "err", err)
		return
	}
	var rootPtr *string
	if cur.DesignatedRoot != "" {
		r := cur.DesignatedRoot
		rootPtr = &r
	}
	var rootPortPtr *int
	if cur.RootPort >= 0 {
		rp := cur.RootPort
		rootPortPtr = &rp
	}

	if ok && prev != nil {
		if cur.TopChanges > prev.TopChanges {
			pl := map[string]interface{}{
				"top_changes":      cur.TopChanges,
				"prev_top_changes":   prev.TopChanges,
				"delta":              cur.TopChanges - prev.TopChanges,
				"designated_root":    cur.DesignatedRoot,
				"root_port":          cur.RootPort,
			}
			e.emit(ctx, nil, d, nil, "STP_TOPOLOGY_CHANGE", "warning", pl)
		} else {
			rootChanged := !strPtrEqual(prev.DesignatedRoot, rootPtr)
			portChanged := !intPtrEqual(prev.RootPort, rootPortPtr)
			if rootChanged || portChanged {
				pl := map[string]interface{}{
					"designated_root":      cur.DesignatedRoot,
					"prev_designated_root": derefStr(prev.DesignatedRoot),
					"root_port":            cur.RootPort,
					"prev_root_port":       derefInt(prev.RootPort),
				}
				e.emit(ctx, nil, d, nil, "STP_ROOT_CHANGED", "warning", pl)
			}
		}
	}

	if err := e.st.UpsertDeviceSTPState(ctx, d.ID, cur.TopChanges, rootPtr, rootPortPtr); err != nil {
		e.log.Warn("stp state save", "device_id", d.ID, "err", err)
	}
}

func strPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return -1
	}
	return *p
}
