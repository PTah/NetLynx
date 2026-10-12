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
