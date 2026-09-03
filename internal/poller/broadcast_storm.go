package poller

import (
	"context"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// stormUtilPort — порт с высокой утилизацией в текущем опросе (для эвристики broadcast storm).
type stormUtilPort struct {
	IfIndex int
	UtilMax float32
	UtilIn  float32
	UtilOut float32
	IfDescr string
}

func fdbGrowing(cfg config.Config, prev, cur int) (bool, int) {
	if prev <= 0 || cur <= prev {
		return false, 0
	}
	delta := cur - prev
	if delta >= cfg.BroadcastStormFDBMinGrowth {
		return true, delta
	}
	pct := float64(delta) / float64(prev) * 100
	if pct >= cfg.BroadcastStormFDBMinGrowthPct {
		return true, delta
	}
	return false, delta
}

func broadcastStormSuspected(cfg config.Config, highPorts []stormUtilPort, prevFDB, curFDB int) (bool, int) {
	if len(highPorts) < cfg.BroadcastStormMinPorts {
		return false, 0
	}
	growing, delta := fdbGrowing(cfg, prevFDB, curFDB)
	return growing, delta
}

func (e *Engine) maybeDetectBroadcastStorm(
	ctx context.Context,
	ignore map[int]store.PortEventIgnore,
	d store.PollDevice,
	ifs map[int]store.InterfaceSnapshot,
	prevFDB, curFDB int,
	highPorts []stormUtilPort,
	now time.Time,
) {
	suspected, fdbDelta := broadcastStormSuspected(e.cfg, highPorts, prevFDB, curFDB)
	_, active := e.broadcastStormActive.Load(d.ID)

	if suspected {
		if active {
			return
		}
		debounce := e.cfg.BroadcastStormDebounce
		if debounce > 0 {
			dup, err := e.st.HasEventSince(ctx, d.ID, "BROADCAST_STORM_SUSPECTED", now.Add(-debounce))
			if err != nil {
				e.log.Warn("broadcast storm debounce", "device_id", d.ID, "err", err)
			} else if dup {
				e.broadcastStormActive.Store(d.ID, true)
				return
			}
		}
		ports := make([]map[string]interface{}, 0, len(highPorts))
		for _, p := range highPorts {
			desc := p.IfDescr
			if desc == "" {
				if snap, ok := ifs[p.IfIndex]; ok {
					desc = snap.DisplayDescr()
				}
			}
			ports = append(ports, map[string]interface{}{
				"if_index":     p.IfIndex,
				"if_descr":     desc,
				"util_max_pct": p.UtilMax,
				"util_in_pct":  p.UtilIn,
				"util_out_pct": p.UtilOut,
			})
		}
		pl := map[string]interface{}{
			"high_util_ports":      len(highPorts),
			"util_threshold_pct":   e.cfg.BroadcastStormUtilPct,
			"min_ports":            e.cfg.BroadcastStormMinPorts,
			"fdb_prev_count":       prevFDB,
			"fdb_cur_count":        curFDB,
			"fdb_delta":            fdbDelta,
			"ports":                ports,
			"source":               "fdb_poll",
			"heuristic":            "multi_port_high_util_plus_fdb_growth",
		}
		e.emit(ctx, ignore, d, nil, "BROADCAST_STORM_SUSPECTED", "warning", pl)
		e.broadcastStormActive.Store(d.ID, true)
		return
	}

	if !active {
		return
	}
	pl := map[string]interface{}{
		"high_util_ports": len(highPorts),
		"min_ports":       e.cfg.BroadcastStormMinPorts,
		"fdb_prev_count":  prevFDB,
		"fdb_cur_count":   curFDB,
		"fdb_delta":       curFDB - prevFDB,
	}
	e.emit(ctx, ignore, d, nil, "BROADCAST_STORM_OK", "info", pl)
	e.broadcastStormActive.Delete(d.ID)
}
