package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/portrolesync"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
)

// CardCLISyncMaxAge — не перечитывать show run при открытии карточки, если poller/бэкап недавно синхронизировали роли.
const CardCLISyncMaxAge = 6 * time.Hour

func (s *Server) handleSyncPortRolesFromConfig(w http.ResponseWriter, r *http.Request) {
	deviceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || deviceID <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	force := r.URL.Query().Get("force") == "1" || strings.EqualFold(r.URL.Query().Get("force"), "true")
	updated, err := s.syncPortRolesFromConfig(r.Context(), deviceID, nil, force)
	if err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		if strings.Contains(err.Error(), "нет SSH") {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	resp := map[string]interface{}{
		"ok":           true,
		"updated":      updated.Total(),
		"roles":        updated.Roles,
		"descriptions": updated.Descriptions,
		"skipped":      updated.Skipped,
	}
	if updated.SyncedAt != nil {
		resp["synced_at"] = updated.SyncedAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, resp)
}

// syncPortRolesFromConfig читает show run (если configRaw nil) и обновляет port_role + description из CLI.
func (s *Server) syncPortRolesFromConfig(ctx context.Context, deviceID int64, configRaw []byte, force bool) (portrolesync.SyncResult, error) {
	var empty portrolesync.SyncResult
	dev, err := s.st.GetDevice(ctx, deviceID)
	if err != nil {
		return empty, err
	}
	if dev == nil {
		return empty, store.ErrDeviceNotFound
	}
	if !portrolesync.ShouldSyncPortRolesFromConfig(dev) {
		syncedAt, _ := s.st.GetDeviceCLIModeSyncAt(ctx, deviceID)
		return portrolesync.SyncResult{Skipped: true, SyncedAt: syncedAt}, nil
	}
	bs, _ := s.st.GetBackupSettings(ctx)
	result, err := portrolesync.SyncDevicePortRoles(ctx, s.st, s.cfg, bs, dev, configRaw, portrolesync.SyncOpts{
		Force:  force,
		MaxAge: CardCLISyncMaxAge,
	})
	if err != nil {
		return result, err
	}
	if len(result.ConfigRaw) > 0 {
		s.putDeviceRunningConfig(deviceID, result.ConfigRaw)
	}
	return result, nil
}
