package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/investigate"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

func (s *Server) handleGetTopology(w http.ResponseWriter, r *http.Request) {
	f := store.TopologyFilter{Dedup: true}
	q := r.URL.Query()
	f.Q = strings.TrimSpace(q.Get("q"))
	f.Protocol = strings.TrimSpace(q.Get("protocol"))
	f.Location = strings.TrimSpace(q.Get("location"))

	var vlanFilter *int
	if v := strings.TrimSpace(q.Get("device_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id == 0 {
			writeError(w, http.StatusBadRequest, "неверный device_id")
			return
		}
		f.DeviceID = &id
	}
	if v := strings.TrimSpace(q.Get("depth")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 32 {
			writeError(w, http.StatusBadRequest, "depth: 0–32")
			return
		}
		f.Depth = &n
	}
	if v := strings.TrimSpace(q.Get("vlan_id")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 4094 {
			writeError(w, http.StatusBadRequest, "неверный vlan_id (1–4094)")
			return
		}
		vlanFilter = &n
	}
	if v := strings.TrimSpace(q.Get("include_stale")); v != "" {
		b := !(strings.EqualFold(v, "0") || strings.EqualFold(v, "false") || strings.EqualFold(v, "no"))
		f.IncludeStale = &b
	}
	if v := strings.TrimSpace(q.Get("dedup")); v != "" {
		f.Dedup = !(strings.EqualFold(v, "0") || strings.EqualFold(v, "false") || strings.EqualFold(v, "no"))
	}

	g, err := s.st.BuildTopologyGraphFiltered(r.Context(), f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if g.Nodes == nil {
		g.Nodes = []store.TopologyNode{}
	}
	if g.Edges == nil {
		g.Edges = []store.TopologyEdge{}
	}
	if vlanFilter != nil {
		g.VLANFilterID = vlanFilter
		g.VLANMatchDeviceIDs = s.deviceIDsWithVLANInDatabase(r.Context(), *vlanFilter, g.Nodes)
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleGetTopologyPath(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fromID, err1 := strconv.ParseInt(strings.TrimSpace(q.Get("from")), 10, 64)
	toID, err2 := strconv.ParseInt(strings.TrimSpace(q.Get("to")), 10, 64)
	if err1 != nil || err2 != nil || fromID <= 0 || toID <= 0 {
		writeError(w, http.StatusBadRequest, "нужны from и to (device id > 0)")
		return
	}
	b := investigate.Builder{St: s.st}
	rep, err := b.BuildTopologyPath(r.Context(), fromID, toID)
	if err != nil {
		if errors.Is(err, investigate.ErrBlastCacheEmpty) {
			writeError(w, http.StatusNotFound, "topology blast cache empty")
			return
		}
		if errors.Is(err, investigate.ErrNoTopologyPath) {
			writeError(w, http.StatusNotFound, "no path between devices")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleGetTopologyReachability(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	fromID, err := strconv.ParseInt(strings.TrimSpace(q.Get("from")), 10, 64)
	if err != nil || fromID <= 0 {
		writeError(w, http.StatusBadRequest, "нужен from (device id > 0)")
		return
	}
	maxDepth := 32
	if v := strings.TrimSpace(q.Get("max_depth")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 10000 {
			writeError(w, http.StatusBadRequest, "max_depth: 0–10000")
			return
		}
		maxDepth = n
	}
	b := investigate.Builder{St: s.st}
	rep, err := b.BuildTopologyReachability(r.Context(), fromID, maxDepth)
	if err != nil {
		if errors.Is(err, investigate.ErrBlastCacheEmpty) {
			writeError(w, http.StatusNotFound, "topology blast cache empty")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// deviceIDsWithVLANInDatabase — узлы inventory, у которых VLAN есть в show run (vlan database).
func (s *Server) deviceIDsWithVLANInDatabase(ctx context.Context, vlanID int, nodes []store.TopologyNode) []int64 {
	out := make([]int64, 0)
	seen := map[int64]struct{}{}
	for _, n := range nodes {
		if n.ID <= 0 || n.Virtual {
			continue
		}
		if _, ok := seen[n.ID]; ok {
			continue
		}
		seen[n.ID] = struct{}{}
		cfg, _ := s.deviceConfigTextForVLAN(ctx, n.ID)
		if strings.TrimSpace(cfg) == "" {
			continue
		}
		db := swcfg.ParseVLANDatabase(cfg)
		if _, ok := db[vlanID]; ok {
			out = append(out, n.ID)
		}
	}
	return out
}
