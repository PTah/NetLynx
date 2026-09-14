package poller

import (
	"context"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// NotifyPortFlapAfterLink — после LINK_UP/DOWN (poll или trap): проверить bounce-окно.
func (e *Engine) NotifyPortFlapAfterLink(ctx context.Context, deviceID int64, ifIndex int) {
	if e == nil || ifIndex <= 0 {
		return
	}
	pd, err := e.st.GetPollDevice(ctx, deviceID)
	if err != nil || pd == nil {
		return
	}
	ignore, _ := e.st.GetPortEventIgnoreMap(ctx, deviceID)
	ifs, _ := e.st.ListInterfaceSnapshots(ctx, deviceID)
	if ifs == nil {
		ifs = map[int]store.InterfaceSnapshot{}
	}
	e.maybeEmitPortFlap(ctx, ignore, *pd, ifIndex, ifs, time.Now())
}

// maybeEmitPortFlap — после LINK_UP/DOWN: N bounce за окно → PORT_FLAP.
func (e *Engine) maybeEmitPortFlap(
	ctx context.Context,
	ignore map[int]store.PortEventIgnore,
	d store.PollDevice,
	ifIndex int,
	ifs map[int]store.InterfaceSnapshot,
	now time.Time,
) {
	if ifIndex <= 0 {
		return
	}
	minB := e.cfg.PortFlapMinBounces
	if minB < 2 {
		minB = 4
	}
	window := e.cfg.PortFlapWindow
	if window <= 0 {
		window = 10 * time.Minute
	}
	debounce := e.cfg.PortFlapDebounce
	if debounce <= 0 {
		debounce = 15 * time.Minute
	}
	since := now.Add(-window)
	n, sources, err := e.st.CountLinkEventsWithSources(ctx, d.ID, ifIndex, since)
	if err != nil {
		e.log.Warn("port flap count", "device_id", d.ID, "if_index", ifIndex, "err", err)
		return
	}
	if n < minB {
		return
	}
	dup, err := e.st.HasPortEventSince(ctx, d.ID, ifIndex, "PORT_FLAP", now.Add(-debounce))
	if err != nil {
		e.log.Warn("port flap debounce", "err", err)
		return
	}
	if dup {
		return
	}
	if len(sources) == 0 {
		sources = []string{"poll"}
	}
	pl := map[string]interface{}{
		"bounce_count": n,
		"window_sec":   int(window.Seconds()),
		"min_bounces":  minB,
		"sources":      sources,
		"investigate":  "/investigate/mac",
	}
	mergeIfacePayload(pl, ifs, ifIndex)
	idx := ifIndex
	e.emit(ctx, ignore, d, &idx, "PORT_FLAP", "warning", pl)
}
