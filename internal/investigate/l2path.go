package investigate

import (
	"context"
	"fmt"
	"sort"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// L2Path — путь по LLDP от «корня» (core-эвристика) до свитча, где MAC на access.
type L2Path struct {
	RootDeviceID   int64      `json:"root_device_id"`
	RootDeviceName string     `json:"root_device_name,omitempty"`
	TargetDeviceID int64      `json:"target_device_id"`
	TargetIfIndex  int        `json:"target_if_index"`
	TargetIfName   string     `json:"target_if_name,omitempty"`
	TargetPortRole string     `json:"target_port_role,omitempty"`
	Hops           []L2PathHop `json:"hops"`
	Summary        string     `json:"summary"`
	Note           string     `json:"note,omitempty"`
}

type L2PathHop struct {
	DeviceID   int64  `json:"device_id"`
	DeviceName string `json:"device_name,omitempty"`
	ViaIfIndex *int   `json:"via_if_index,omitempty"`
	ViaIfName  string `json:"via_if_name,omitempty"`
}

type l2AdjEdge struct {
	to       int64
	localIf  int
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
			continue // похоже на misclassified uplink
		}
		access = append(access, f)
	}
	if len(access) == 0 {
		return nil
	}

	includeStale := false
	g, err := b.St.BuildTopologyGraphFiltered(ctx, store.TopologyFilter{
		Protocol:     "lldp",
		Dedup:        true,
		IncludeStale: &includeStale,
	})
	if err != nil || g == nil || len(g.Edges) == 0 {
		return nil
	}

	names := map[int64]string{}
	linkCount := map[int64]int{}
	for _, n := range g.Nodes {
		names[n.ID] = n.Name
		if names[n.ID] == "" {
			names[n.ID] = n.Host
		}
		linkCount[n.ID] = n.LinkCount
	}
	adj := map[int64][]l2AdjEdge{}
	for _, e := range g.Edges {
		if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 || e.Stale {
			continue
		}
		name := ""
		if e.LocalIfName != nil {
			name = *e.LocalIfName
		}
		to := *e.RemoteDeviceID
		adj[e.LocalDeviceID] = append(adj[e.LocalDeviceID], l2AdjEdge{to: to, localIf: e.LocalIfIndex, localName: name})
		// обратное ребро без if на remote стороне
		adj[to] = append(adj[to], l2AdjEdge{to: e.LocalDeviceID, localIf: 0, localName: ""})
	}

	root := pickTopologyRoot(g.Nodes, linkCount)
	if root == 0 {
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
		}
		if path.TargetPortRole == "" {
			path.TargetPortRole = "access"
		}
		if f.DeviceID == root {
			path.Hops = []L2PathHop{{DeviceID: root, DeviceName: names[root]}}
			path.Summary = fmt.Sprintf("%s → port %s", names[root], labelPort(ifName, f.IfIndex))
			path.Note = "MAC на access того же узла, что и выбранный корень топологии"
			out = append(out, path)
			continue
		}
		hops, ok := bfsDevicePath(adj, names, root, f.DeviceID)
		if !ok {
			path.Hops = []L2PathHop{{DeviceID: f.DeviceID, DeviceName: names[f.DeviceID]}}
			path.Summary = fmt.Sprintf("%s · %s (нет LLDP-пути от корня)", names[f.DeviceID], labelPort(ifName, f.IfIndex))
			path.Note = "Нет непрерывного LLDP-пути от эвристического core — смотрите локальный access-порт"
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
		out = append(out, path)
	}
	return out
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
		// лёгкий бонус router/core по kind
		switch n.Kind {
		case "router", "core", "switch":
			if n.Kind == "router" || n.Kind == "core" {
				score += 100
			}
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
	// восстановить путь from → to
	var rev []L2PathHop
	cur := to
	for cur != from {
		p := parent[cur]
		hop := L2PathHop{DeviceID: cur, DeviceName: names[cur]}
		if p.via.localIf > 0 {
			vi := p.via.localIf
			hop.ViaIfIndex = &vi
			hop.ViaIfName = p.via.localName
		}
		rev = append(rev, hop)
		cur = p.id
	}
	rev = append(rev, L2PathHop{DeviceID: from, DeviceName: names[from]})
	// reverse
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	// ViaIf на hop i — порт на предыдущем устройстве; переносим на предыдущий hop для читаемости
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
