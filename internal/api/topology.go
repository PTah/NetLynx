package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"

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
