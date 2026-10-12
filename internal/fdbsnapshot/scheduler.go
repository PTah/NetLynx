package fdbsnapshot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

type Scheduler struct {
	log *slog.Logger
	st  *store.Store
	cfg config.Config
	mu  sync.Mutex
	busy bool
}

func NewScheduler(log *slog.Logger, st *store.Store, cfg config.Config) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{log: log, st: st, cfg: cfg}
}

func (s *Scheduler) Run(ctx context.Context) {
	if !s.cfg.FDBSnapshotEnabled {
		s.log.Info("fdb snapshots disabled")
		return
	}
	interval := 6 * time.Hour
	s.log.Info("fdb snapshot prune scheduler", "interval", interval.String(), "retain_days", s.cfg.FDBSnapshotRetentionDays)
	t := time.NewTicker(interval)
	defer t.Stop()
	s.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.busy = false
		s.mu.Unlock()
	}()
	n, err := PruneOld(ctx, s.st, s.cfg.FDBSnapshotRetentionDays)
	if err != nil {
		s.log.Warn("fdb snapshot prune", "err", err)
		return
	}
	if n > 0 {
		s.log.Info("fdb snapshot prune", "deleted", n)
	}
}
