package configsnapshot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/configssh"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

type Scheduler struct {
	log      *slog.Logger
	st       *store.Store
	cfg      config.Config
	reporter *configssh.Reporter
	mu       sync.Mutex
	busy     bool
}

func NewScheduler(log *slog.Logger, st *store.Store, cfg config.Config) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{log: log, st: st, cfg: cfg}
}

func (s *Scheduler) SetReporter(rep *configssh.Reporter) {
	s.reporter = rep
}

func (s *Scheduler) Run(ctx context.Context) {
	if !s.cfg.ConfigSnapshotEnabled {
		s.log.Info("config snapshots disabled")
		return
	}
	interval := s.cfg.ConfigSnapshotInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	s.log.Info("config snapshot scheduler", "interval", interval.String(), "retain_days", s.cfg.ConfigSnapshotRetentionDays)
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

	if n, err := PruneOld(ctx, s.st, s.cfg.ConfigSnapshotRetentionDays); err != nil {
		s.log.Warn("config snapshot prune", "err", err)
	} else if n > 0 {
		s.log.Info("config snapshot prune", "deleted", n)
	}

	bs, err := s.st.GetBackupSettings(ctx)
	if err != nil {
		s.log.Warn("config snapshot: backup settings", "err", err)
		return
	}
	devs, err := s.st.ListDevices(ctx)
	if err != nil {
		s.log.Warn("config snapshot: list devices", "err", err)
		return
	}
	var targets []models.Device
	for _, d := range devs {
		if !ShouldSnapshotDevice(d) || !d.IsOnline() {
			continue
		}
		targets = append(targets, d)
	}
	if len(targets) == 0 {
		return
	}
	s.log.Info("config snapshot run", "devices", len(targets))
	var saved, skipped, failed int
	for _, d := range targets {
		select {
		case <-ctx.Done():
			return
		default:
		}
		full, gerr := s.st.GetDevice(ctx, d.ID)
		if gerr != nil || full == nil {
			failed++
			s.log.Warn("config snapshot get device", "device_id", d.ID, "err", gerr)
			continue
		}
		ok, _, err := FetchAndStore(ctx, s.st, s.cfg, bs, full, "scheduled")
		if err != nil {
			failed++
			s.log.Warn("config snapshot fetch", "device_id", full.ID, "host", full.Host, "err", err)
			if s.reporter != nil {
				s.reporter.ReportFail(ctx, full.ID, full.Name, full.Host, err, "scheduled")
			}
			continue
		}
		if ok {
			saved++
		} else {
			skipped++
		}
		time.Sleep(2 * time.Second)
	}
	s.log.Info("config snapshot done", "saved", saved, "unchanged", skipped, "failed", failed)
}
