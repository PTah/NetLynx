package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/configsnapshot"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListDeviceConfigSnapshots(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	snaps, err := s.st.ListConfigSnapshots(r.Context(), deviceID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if snaps == nil {
		snaps = []store.ConfigSnapshot{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"snapshots": snaps})
}

func (s *Server) handleGetDeviceConfigSnapshot(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	snapID, err := strconv.ParseInt(chi.URLParam(r, "snapId"), 10, 64)
	if err != nil || snapID <= 0 {
		writeError(w, http.StatusBadRequest, "неверный snapId")
		return
	}
	snap, err := s.st.GetConfigSnapshot(r.Context(), deviceID, snapID)
	if err != nil {
		if errors.Is(err, store.ErrConfigSnapshotNotFound) {
			writeError(w, http.StatusNotFound, "снимок не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleDeviceConfigDiff(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	ctx := r.Context()
	toID, _ := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	fromID, _ := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)

	var toSnap, fromSnap *store.ConfigSnapshotFull
	if toID > 0 {
		toSnap, err = s.st.GetConfigSnapshot(ctx, deviceID, toID)
	} else {
		list, lerr := s.st.ListConfigSnapshots(ctx, deviceID, 1)
		if lerr != nil {
			writeError(w, http.StatusInternalServerError, lerr.Error())
			return
		}
		if len(list) == 0 {
			writeError(w, http.StatusNotFound, "нет снимков конфига")
			return
		}
		toSnap, err = s.st.GetConfigSnapshot(ctx, deviceID, list[0].ID)
	}
	if err != nil {
		if errors.Is(err, store.ErrConfigSnapshotNotFound) {
			writeError(w, http.StatusNotFound, "снимок to не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if fromID > 0 {
		fromSnap, err = s.st.GetConfigSnapshot(ctx, deviceID, fromID)
	} else if since := r.URL.Query().Get("since"); since != "" {
		t, perr := time.Parse(time.RFC3339, since)
		if perr != nil {
			writeError(w, http.StatusBadRequest, "since: ожидается RFC3339")
			return
		}
		fromSnap, err = s.st.GetConfigSnapshotNear(ctx, deviceID, t)
	} else {
		fromSnap, err = s.st.GetPreviousConfigSnapshot(ctx, deviceID, toSnap.ID)
	}
	if err != nil {
		if errors.Is(err, store.ErrConfigSnapshotNotFound) {
			writeError(w, http.StatusNotFound, "снимок from не найден (нужен второй снимок или ?from=)")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	lines := swcfg.LineDiff(fromSnap.ConfigText, toSnap.ConfigText)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"from":  fromSnap.ConfigSnapshot,
		"to":    toSnap.ConfigSnapshot,
		"lines": lines,
		"stats": swcfg.DiffStatsFrom(lines),
	})
}

func (s *Server) handleFetchDeviceConfigSnapshot(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseDeviceID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	dev, err := s.st.GetDevice(r.Context(), deviceID)
	if err != nil {
		if errors.Is(err, store.ErrDeviceNotFound) {
			writeError(w, http.StatusNotFound, "узел не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	bs, _ := s.st.GetBackupSettings(r.Context())
	saved, id, err := configsnapshot.FetchAndStore(r.Context(), s.st, s.cfg, bs, dev, "manual")
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":    true,
		"saved": saved,
		"id":    id,
	})
}

func parseDeviceID(raw string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, err
	}
	return id, nil
}
