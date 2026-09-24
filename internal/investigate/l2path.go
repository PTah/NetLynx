package investigate

import (
	"context"
	"fmt"
	"sort"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// L2Path — путь по топологии от корня (blast root / эвристика) до access с MAC.
type L2Path struct {
	RootDeviceID   int64       `json:"root_device_id"`
	RootDeviceName string      `json:"root_device_name,omitempty"`
	TargetDeviceID int64       `json:"target_device_id"`
	TargetIfIndex  int         `json:"target_if_index"`
	TargetIfName   string      `json:"target_if_name,omitempty"`
	TargetPortRole string      `json:"target_port_role,omitempty"`
	Hops           []L2PathHop `json:"hops"`
	Summary        string      `json:"summary"`
	Note           string      `json:"note,omitempty"`
	Source         string      `json:"source,omitempty"` // blast_cache | live_lldp
}

type L2PathHop struct {
	DeviceID   int64  `json:"device_id"`
	DeviceName string `json:"device_name,omitempty"`
	ViaIfIndex *int   `json:"via_if_index,omitempty"`
	ViaIfName  string `json:"via_if_name,omitempty"`
}

type l2AdjEdge struct {
	to        int64
	localIf   int
	localName string
}

func (b *Builder) buildL2Paths(ctx context.Context, foot []FootprintEntry) []L2Path {
	access := make([]FootprintEntry, 0)
	for _, f := range foot {
		role := f.PortRole
		if role == "" {
			role = "access"
		}
		if role != "access" {
			continue
		}
		if f.MACCount >= 8 {
			continue
		}
		access = append(access, f)
	}
	if len(access) == 0 {
		return nil
	}

	names := b.DeviceNames(ctx)
	adj := map[int64][]l2AdjEdge{}
	root := int64(0)
	source := "live_lldp"

	if g, ok, err := LoadBlastGraph(ctx, b.St); err == nil && ok && g != nil {
		source = "blast_cache"
		root = g.RootID
		for _, e := range g.Edges {
			a, bID := e.ADeviceID, e.BDeviceID
			if a <= 0 || bID <= 0 {
				continue
			}
			adj[a] = append(adj[a], l2AdjEdge{to: bID, localIf: e.AIfIndex})
			adj[bID] = append(adj[bID], l2AdjEdge{to: a, localIf: e.BIfIndex})
		}
		if root == 0 {
			root = pickRootFromL2Adj(adj)
		}
	} else {
		includeStale := false
		tg, err := b.St.BuildTopologyGraphFiltered(ctx, store.TopologyFilter{
			Protocol:     "lldp",
			Dedup:        true,
			IncludeStale: &includeStale,
		})
		if err != nil || tg == nil || len(tg.Edges) == 0 {
			return nil
		}
		linkCount := map[int64]int{}
		for _, n := range tg.Nodes {
			names[n.ID] = n.Name
			if names[n.ID] == "" {
				names[n.ID] = n.Host
			}
			linkCount[n.ID] = n.LinkCount
		}
		for _, e := range tg.Edges {
			if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 || e.Stale {
				continue
			}
			name := ""
			if e.LocalIfName != nil {
				name = *e.LocalIfName
			}
			to := *e.RemoteDeviceID
			adj[e.LocalDeviceID] = append(adj[e.LocalDeviceID], l2AdjEdge{to: to, localIf: e.LocalIfIndex, localName: name})
			adj[to] = append(adj[to], l2AdjEdge{to: e.LocalDeviceID})
		}
		root = pickTopologyRoot(tg.Nodes, linkCount)
	}

	if root == 0 || len(adj) == 0 {
		return nil
	}

	var out []L2Path
	seenTarget := map[string]struct{}{}
	for _, f := range access {
		key := fmt.Sprintf("%d:%d", f.DeviceID, f.IfIndex)
		if _, ok := seenTarget[key]; ok {
			continue
		}
		seenTarget[key] = struct{}{}

		ifName := ""
		if f.IfName != nil {
			ifName = *f.IfName
		} else if f.IfDescr != nil {
			ifName = *f.IfDescr
		}
		path := L2Path{
			RootDeviceID:   root,
			RootDeviceName: names[root],
			TargetDeviceID: f.DeviceID,
			TargetIfIndex:  f.IfIndex,
			TargetIfName:   ifName,
			TargetPortRole: f.PortRole,
			Source:         source,
		}
		if path.RootDeviceName == "" {
			path.RootDeviceName = fmt.Sprintf("#%d", root)
		}
		if path.TargetPortRole == "" {
			path.TargetPortRole = "access"
		}
		if f.DeviceID == root {
			path.Hops = []L2PathHop{{DeviceID: root, DeviceName: path.RootDeviceName}}
			path.Summary = fmt.Sprintf("%s → port %s", path.RootDeviceName, labelPort(ifName, f.IfIndex))
			path.Note = "MAC на access того же узла, что и корень топологии"
			out = append(out, path)
			continue
		}
		hops, ok := bfsDevicePath(adj, names, root, f.DeviceID)
		if !ok {
			path.Hops = []L2PathHop{{DeviceID: f.DeviceID, DeviceName: names[f.DeviceID]}}
			path.Summary = fmt.Sprintf("%s · %s (нет пути от корня)", names[f.DeviceID], labelPort(ifName, f.IfIndex))
			path.Note = "Нет непрерывного пути в blast-cache/LLDP от корня — смотрите локальный access-порт"
			out = append(out, path)
			continue
		}
		path.Hops = hops
		parts := make([]string, 0, len(hops)+1)
		for _, h := range hops {
			parts = append(parts, h.DeviceName)
		}
		parts = append(parts, "port "+labelPort(ifName, f.IfIndex))
		path.Summary = joinArrow(parts)
		if source == "blast_cache" {
			path.Note = "Путь по topology blast cache (LLDP+CDP+manual)"
		}
		out = append(out, path)
	}
	return out
}

func pickRootFromAdj(adj map[int64][]int64) int64 {
	best := int64(0)
	bestScore := -1
	for id, ns := range adj {
		score := len(ns)
		if score > bestScore || (score == bestScore && (best == 0 || id < best)) {
			bestScore = score
			best = id
		}
	}
	return best
}

func pickRootFromL2Adj(adj map[int64][]l2AdjEdge) int64 {
	best := int64(0)
	bestScore := -1
	for id, ns := range adj {
		score := len(ns)
		if score > bestScore || (score == bestScore && (best == 0 || id < best)) {
			bestScore = score
			best = id
		}
	}
	return best
}

func pickTopologyRoot(nodes []store.TopologyNode, linkCount map[int64]int) int64 {
	if len(nodes) == 0 {
		return 0
	}
	best := nodes[0].ID
	bestScore := -1
	for _, n := range nodes {
		score := linkCount[n.ID]
		if n.LinkCount > score {
			score = n.LinkCount
		}
		switch n.Kind {
		case "router", "core":
			score += 100
		}
		if score > bestScore || (score == bestScore && n.ID < best) {
			bestScore = score
			best = n.ID
		}
	}
	return best
}

func bfsDevicePath(adj map[int64][]l2AdjEdge, names map[int64]string, from, to int64) ([]L2PathHop, bool) {
	if from == to {
		return []L2PathHop{{DeviceID: from, DeviceName: names[from]}}, true
	}
	type prev struct {
		id  int64
		via l2AdjEdge
	}
	parent := map[int64]prev{}
	q := []int64{from}
	seen := map[int64]bool{from: true}
	found := false
	for len(q) > 0 {
		u := q[0]
		q = q[1:]
		for _, e := range adj[u] {
			if seen[e.to] {
				continue
			}
			seen[e.to] = true
			parent[e.to] = prev{id: u, via: e}
			if e.to == to {
				found = true
				q = nil
				break
			}
			q = append(q, e.to)
		}
	}
	if !found {
		return nil, false
	}
	var rev []L2PathHop
	cur := to
	for cur != from {
		p := parent[cur]
		hop := L2PathHop{DeviceID: cur, DeviceName: names[cur]}
		if hop.DeviceName == "" {
			hop.DeviceName = fmt.Sprintf("#%d", cur)
		}
		if p.via.localIf > 0 {
			vi := p.via.localIf
			hop.ViaIfIndex = &vi
			hop.ViaIfName = p.via.localName
		}
		rev = append(rev, hop)
		cur = p.id
	}
	fromName := names[from]
	if fromName == "" {
		fromName = fmt.Sprintf("#%d", from)
	}
	rev = append(rev, L2PathHop{DeviceID: from, DeviceName: fromName})
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	out := make([]L2PathHop, len(rev))
	for i := range rev {
		out[i] = L2PathHop{DeviceID: rev[i].DeviceID, DeviceName: rev[i].DeviceName}
		if i+1 < len(rev) && rev[i+1].ViaIfIndex != nil {
			out[i].ViaIfIndex = rev[i+1].ViaIfIndex
			out[i].ViaIfName = rev[i+1].ViaIfName
		}
	}
	return out, true
}

func labelPort(name string, ifIndex int) string {
	if name != "" {
		return name
	}
	return fmt.Sprintf("if%d", ifIndex)
}

func joinArrow(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	s := parts[0]
	for i := 1; i < len(parts); i++ {
		s += " → " + parts[i]
	}
	return s
}

func sortPathsStable(paths []L2Path) {
	sort.SliceStable(paths, func(i, j int) bool {
		if len(paths[i].Hops) != len(paths[j].Hops) {
			return len(paths[i].Hops) < len(paths[j].Hops)
		}
		return paths[i].Summary < paths[j].Summary
	})
}
