package fdbsnapshot

import (
	"context"
	"log/slog"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

const defaultMinInterval = 20 * time.Hour

// ShouldSaveDaily — не чаще minInterval с последнего снимка.
func ShouldSaveDaily(last *time.Time, now time.Time, minInterval time.Duration) bool {
	if minInterval <= 0 {
		minInterval = defaultMinInterval
	}
	if last == nil {
		return true
	}
	return now.Sub(*last) >= minInterval
}

// MaybeSaveDaily сохраняет снимок FDB после успешного poll, если прошёл интервал.
func MaybeSaveDaily(
	ctx context.Context,
	log *slog.Logger,
	st *store.Store,
	cfg config.Config,
	deviceID int64,
	entries map[string]store.FDBLearnedEntry,
	at time.Time,
) error {
	if !cfg.FDBSnapshotEnabled {
		return nil
	}
	if len(entries) == 0 {
		return nil
	}
	interval := cfg.FDBSnapshotInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	if interval < defaultMinInterval {
		interval = defaultMinInterval
	}
	last, err := st.GetLatestFDBSnapshotTime(ctx, deviceID)
	if err != nil {
		return err
	}
	if !ShouldSaveDaily(last, at, interval) {
		return nil
	}
	id, err := st.InsertFDBSnapshot(ctx, deviceID, at, "fdb_poll", entries)
	if err != nil {
		return err
	}
	if log != nil {
		log.Debug("fdb daily snapshot", "device_id", deviceID, "snapshot_id", id, "entries", len(entries))
	}
	return nil
}

func PruneOld(ctx context.Context, st *store.Store, retainDays int) (int64, error) {
	if retainDays <= 0 {
		retainDays = 30
	}
	cutoff := time.Now().Add(-time.Duration(retainDays) * 24 * time.Hour)
	return st.PruneFDBSnapshots(ctx, cutoff)
}
