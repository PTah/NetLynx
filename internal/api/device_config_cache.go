package api

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

type deviceRunningConfigEntry struct {
	raw string
	at  time.Time
}

func (s *Server) putDeviceRunningConfig(deviceID int64, raw []byte) {
	if s == nil || deviceID <= 0 {
		return
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return
	}
	s.runningConfigCache.Store(deviceID, deviceRunningConfigEntry{raw: text, at: time.Now()})
	s.runningConfigDirtyAt.Delete(deviceID)
}

func (s *Server) getDeviceRunningConfig(deviceID int64, maxAge time.Duration) (string, bool) {
	if s == nil || deviceID <= 0 || maxAge <= 0 {
		return "", false
	}
	v, ok := s.runningConfigCache.Load(deviceID)
	if !ok {
		return "", false
	}
	ent, ok := v.(deviceRunningConfigEntry)
	if !ok || strings.TrimSpace(ent.raw) == "" {
		return "", false
	}
	if time.Since(ent.at) > maxAge {
		s.runningConfigCache.Delete(deviceID)
		return "", false
	}
	return ent.raw, true
}

func (s *Server) invalidateDeviceRunningConfig(deviceID int64) {
	if s == nil || deviceID <= 0 {
		return
	}
	s.runningConfigCache.Delete(deviceID)
	// Пометка: snapshot в БД ещё со старым PoE/shutdown — не отдавать его в /settings,
	// пока не положим свежий show run в memory-кэш (putDeviceRunningConfig).
	s.runningConfigDirtyAt.Store(deviceID, time.Now())
}

func (s *Server) runningConfigDirtySince(deviceID int64) (time.Time, bool) {
	if s == nil || deviceID <= 0 {
		return time.Time{}, false
	}
	v, ok := s.runningConfigDirtyAt.Load(deviceID)
	if !ok {
		return time.Time{}, false
	}
	t, ok := v.(time.Time)
	if !ok || t.IsZero() || time.Since(t) > CardCLISyncMaxAge {
		return time.Time{}, false
	}
	return t, true
}

func (s *Server) runningConfigForPortSettings(ctx context.Context, deviceID int64) (text, via string, ok bool) {
	if raw, hit := s.getDeviceRunningConfig(deviceID, CardCLISyncMaxAge); hit {
		return raw, "config_cache", true
	}
	snap, err := s.st.GetLatestConfigSnapshot(ctx, deviceID)
	if err != nil {
		if !errors.Is(err, store.ErrConfigSnapshotNotFound) {
			return "", "", false
		}
		return "", "", false
	}
	if snap == nil || strings.TrimSpace(snap.ConfigText) == "" {
		return "", "", false
	}
	// После shutdown / PoE / VLAN через UI memory сброшен. Старый snapshot
	// (часто без poe opmode shutdown) даёт ложный PoE+ в модалке — нельзя
	// включить PoE повторно: UI считает, что уже poe+.
	if dirtyAt, dirty := s.runningConfigDirtySince(deviceID); dirty {
		if !snap.FetchedAt.After(dirtyAt) {
			return "", "", false
		}
		// Snapshot новее записи — можно снова доверять.
		s.runningConfigDirtyAt.Delete(deviceID)
	}
	fresh := time.Since(snap.FetchedAt) < CardCLISyncMaxAge
	if !fresh {
		syncedAt, serr := s.st.GetDeviceCLIModeSyncAt(ctx, deviceID)
		if serr == nil && syncedAt != nil && time.Since(*syncedAt) < CardCLISyncMaxAge {
			fresh = true
		}
	}
	if !fresh {
		return "", "", false
	}
	return snap.ConfigText, "snapshot", true
}
