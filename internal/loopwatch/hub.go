package loopwatch

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/investigate"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/live"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/notify"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// Hub — периодический DFS петель; L2_LOOP_APPEARED только на новый cycle key.
type Hub struct {
	log  *slog.Logger
	st   *store.Store
	cfg  config.Config
	hub  *live.Hub
	hook *notify.EventHook

	mu        sync.Mutex
	busy      bool
	knownKeys map[string]struct{}
	seeded    bool
}

func NewHub(log *slog.Logger, st *store.Store, cfg config.Config, liveHub *live.Hub, hook *notify.EventHook) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{
		log:       log,
		st:        st,
		cfg:       cfg,
		hub:       liveHub,
		hook:      hook,
		knownKeys: map[string]struct{}{},
	}
}

func (h *Hub) Run(ctx context.Context) {
	if !h.cfg.LoopWatchEnabled {
		h.log.Info("loop watch disabled")
		return
	}
	interval := h.cfg.LoopWatchInterval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	h.log.Info("loop watch scheduler", "interval", interval.String())
	h.runOnce(ctx, true) // seed known keys, no events
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			h.runOnce(ctx, false)
		}
	}
}

func (h *Hub) runOnce(ctx context.Context, seedOnly bool) {
	h.mu.Lock()
	if h.busy {
		h.mu.Unlock()
		return
	}
	h.busy = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		h.busy = false
		h.mu.Unlock()
	}()

	b := &investigate.Builder{St: h.st}
	rep, err := b.BuildLoopReport(ctx, "")
	if err != nil {
		h.log.Warn("loop watch build", "err", err)
		return
	}
	newEvents := 0
	for _, c := range rep.Cycles {
		key := investigate.CycleKeyFor(c)
		if key == "" {
			continue
		}
		h.mu.Lock()
		_, seen := h.knownKeys[key]
		if !seen {
			h.knownKeys[key] = struct{}{}
		}
		h.mu.Unlock()
		if seedOnly || seen {
			continue
		}
		if h.emitAppeared(ctx, c, key) {
			newEvents++
		}
	}
	if seedOnly {
		h.mu.Lock()
		h.seeded = true
		h.mu.Unlock()
	}
	h.log.Info("loop watch",
		"cycles", len(rep.Cycles),
		"source", rep.Source,
		"new_events", newEvents,
		"seed", seedOnly,
	)
}

func (h *Hub) emitAppeared(ctx context.Context, c investigate.TopologyCycle, key string) bool {
	pl := map[string]interface{}{
		"cycle_key":   key,
		"length":      c.Length,
		"summary":     c.Summary,
		"device_ids":  c.DeviceIDs,
		"investigate": "/investigate/loops",
	}
	var deviceID int64
	if len(c.DeviceIDs) > 0 {
		deviceID = c.DeviceIDs[0]
	}
	if deviceID <= 0 {
		return false
	}
	since := time.Now().Add(-24 * time.Hour)
	dup, err := h.st.HasLoopCycleEventSince(ctx, "L2_LOOP_APPEARED", key, since)
	if err != nil {
		h.log.Warn("loop watch debounce", "err", err)
		return false
	}
	if dup {
		return false
	}
	evID, err := h.st.InsertEvent(ctx, deviceID, nil, "L2_LOOP_APPEARED", "warning", pl)
	if err != nil {
		h.log.Warn("loop watch insert", "err", err)
		return false
	}
	devName, devHost := "", ""
	if d, err := h.st.GetDevice(ctx, deviceID); err == nil && d != nil {
		devName, devHost = d.Name, d.Host
	}
	if h.hub != nil {
		h.hub.Publish(live.EventPayload{
			EventID: evID, DeviceID: deviceID, DeviceName: devName, DeviceHost: devHost,
			EventType: "L2_LOOP_APPEARED", Severity: "warning", Payload: pl,
		})
	}
	if h.hook != nil {
		h.hook.DispatchEvent(deviceID, devName, devHost, evID, nil, "L2_LOOP_APPEARED", "warning", pl)
	}
	return true
}
