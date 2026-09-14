package topologycache

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

const defaultDebounce = 45 * time.Second

// Hub — debounced NotifyDirty + periodic/night rebuild.
type Hub struct {
	log  *slog.Logger
	st   *store.Store
	cfg  config.Config
	mu   sync.Mutex
	busy bool

	debMu    sync.Mutex
	debTimer *time.Timer
	debReason string

	lastNightKey string
}

func NewHub(log *slog.Logger, st *store.Store, cfg config.Config) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{log: log, st: st, cfg: cfg}
}

// NotifyDirty ставит пересборку через debounce (poller / manual links).
func (h *Hub) NotifyDirty(reason string) {
	if h == nil || !h.cfg.TopologyBlastCacheEnabled {
		return
	}
	h.debMu.Lock()
	defer h.debMu.Unlock()
	if reason == "" {
		reason = "event"
	}
	h.debReason = reason
	if h.debTimer != nil {
		h.debTimer.Stop()
	}
	h.debTimer = time.AfterFunc(defaultDebounce, func() {
		h.debMu.Lock()
		r := h.debReason
		h.debMu.Unlock()
		h.runOnce(context.Background(), "event:"+r)
	})
}

// Run — hourly ticker + night wall-clock + стартовый rebuild.
func (h *Hub) Run(ctx context.Context) {
	if !h.cfg.TopologyBlastCacheEnabled {
		h.log.Info("topology blast cache disabled")
		return
	}
	interval := h.cfg.TopologyBlastCacheInterval
	if interval <= 0 {
		interval = time.Hour
	}
	nightHour := h.cfg.TopologyBlastNightHour
	if nightHour < 0 || nightHour > 23 {
		nightHour = 3
	}
	h.log.Info("topology blast cache scheduler",
		"interval", interval.String(),
		"night_hour", nightHour,
	)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	nightTick := time.NewTicker(30 * time.Second)
	defer nightTick.Stop()

	h.runOnce(ctx, "startup")
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			h.runOnce(ctx, "hourly")
		case now := <-nightTick.C:
			if now.Hour() != nightHour || now.Minute() != 0 {
				continue
			}
			key := now.Format("2006-01-02")
			if h.lastNightKey == key {
				continue
			}
			h.lastNightKey = key
			h.runOnce(ctx, "night")
		}
	}
}

func (h *Hub) runOnce(ctx context.Context, trigger string) {
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

	if n, err := h.st.PruneTopologyBlastHistory(ctx, h.cfg.TopologyBlastHistoryDays); err != nil {
		h.log.Warn("topology blast history prune", "err", err)
	} else if n > 0 {
		h.log.Info("topology blast history prune", "deleted", n)
	}

	res, err := Rebuild(ctx, h.st, trigger)
	if err != nil {
		h.log.Warn("topology blast cache rebuild", "trigger", trigger, "err", err)
		return
	}
	h.log.Info("topology blast cache rebuilt",
		"trigger", res.Trigger,
		"root_device_id", res.RootID,
		"root_source", res.RootSource,
		"edges", res.Edges,
		"nodes", res.Nodes,
		"vlan_edges", res.VLANEdgeCount,
	)
}
