package investigate

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// LoopReport — отдельный отчёт о петлях в LLDP/CDP-топологии (не смешивать с MAC flapping).
type LoopReport struct {
	Cycles      []TopologyCycle `json:"cycles"`
	NodeCount   int             `json:"node_count"`
	EdgeCount   int             `json:"edge_count"`
	Protocol    string          `json:"protocol"` // фильтр: lldp | "" = lldp+cdp resolved
	GeneratedAt time.Time       `json:"generated_at"`
}

type TopologyCycle struct {
	Length      int               `json:"length"`
	DeviceIDs   []int64           `json:"device_ids"`
	DeviceNames []string          `json:"device_names"`
	Hops        []TopologyCycleHop `json:"hops"`
	Summary     string            `json:"summary"`
}

type TopologyCycleHop struct {
	FromDeviceID   int64   `json:"from_device_id"`
	FromDeviceName string  `json:"from_device_name,omitempty"`
	FromIfIndex    int     `json:"from_if_index"`
	FromIfName     string  `json:"from_if_name,omitempty"`
	ToDeviceID     int64   `json:"to_device_id"`
	ToDeviceName   string  `json:"to_device_name,omitempty"`
	ToIfIndex      *int    `json:"to_if_index,omitempty"`
	ToIfName       string  `json:"to_if_name,omitempty"`
	Protocol       string  `json:"protocol"`
}

type topoLink struct {
	a, b           int64
	aIf, bIf       int
	aIfName        string
	bIfName        string
	protocol       string
}

func (b *Builder) BuildLoopReport(ctx context.Context, protocol string) (*LoopReport, error) {
	proto := strings.ToLower(strings.TrimSpace(protocol))
	if proto != "" && proto != "lldp" && proto != "cdp" {
		proto = "lldp"
	}
	f := store.TopologyFilter{Dedup: true, Protocol: proto}
	if proto == "" {
		// По умолчанию — только LLDP (явные L2-соседи); CDP часто дублирует.
		f.Protocol = "lldp"
		proto = "lldp"
	}
	includeStale := false
	f.IncludeStale = &includeStale

	g, err := b.St.BuildTopologyGraphFiltered(ctx, f)
	if err != nil {
		return nil, err
	}
	names := map[int64]string{}
	for _, n := range g.Nodes {
		names[n.ID] = n.Name
		if names[n.ID] == "" {
			names[n.ID] = n.Host
		}
	}

	links, adj := buildUndirectedLLDPGraph(g.Edges)
	cycles := findUndirectedCycles(adj, links, names)
	cycles = append(cycles, parallelLinkCyclesFromEdges(g.Edges, names)...)
	// дедуп по key
	seen := map[string]struct{}{}
	uniq := cycles[:0]
	for _, c := range cycles {
		k := cycleKey(c.DeviceIDs)
		if len(c.DeviceIDs) == 2 {
			k = "parallel:" + k
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, c)
	}
	cycles = uniq
	sort.Slice(cycles, func(i, j int) bool {
		if cycles[i].Length != cycles[j].Length {
			return cycles[i].Length < cycles[j].Length
		}
		return cycles[i].Summary < cycles[j].Summary
	})

	rep := &LoopReport{
		Cycles:      cycles,
		NodeCount:   len(adj),
		EdgeCount:   len(links),
		Protocol:    proto,
		GeneratedAt: time.Now(),
	}
	if rep.Cycles == nil {
		rep.Cycles = []TopologyCycle{}
	}
	return rep, nil
}

func buildUndirectedLLDPGraph(edges []store.TopologyEdge) ([]topoLink, map[int64][]int64) {
	seen := map[string]topoLink{}
	for _, e := range edges {
		if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 {
			continue
		}
		if e.Stale {
			continue
		}
		a, b := e.LocalDeviceID, *e.RemoteDeviceID
		aIf := e.LocalIfIndex
		bIf := 0
		bIfName := ""
		if e.RemoteIfName != nil {
			bIfName = *e.RemoteIfName
		}
		// Remote if_index часто неизвестен — 0.
		aIfName := ""
		if e.LocalIfName != nil {
			aIfName = *e.LocalIfName
		}
		if a > b {
			a, b = b, a
			aIf, bIf = bIf, aIf
			aIfName, bIfName = bIfName, aIfName
		}
		key := fmt.Sprintf("%d:%d-%d:%d", a, aIf, b, bIf)
		if aIf == 0 && bIf == 0 {
			key = fmt.Sprintf("%d-%d", a, b)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = topoLink{
			a: a, b: b, aIf: aIf, bIf: bIf,
			aIfName: aIfName, bIfName: bIfName,
			protocol: e.Protocol,
		}
	}
	links := make([]topoLink, 0, len(seen))
	adj := map[int64][]int64{}
	pairSeen := map[[2]int64]struct{}{}
	for _, l := range seen {
		links = append(links, l)
		p := [2]int64{l.a, l.b}
		if _, ok := pairSeen[p]; ok {
			// Несколько портов между той же парой — всё равно одно ребро для DFS устройств.
			continue
		}
		pairSeen[p] = struct{}{}
		adj[l.a] = append(adj[l.a], l.b)
		adj[l.b] = append(adj[l.b], l.a)
	}
	for id := range adj {
		sort.Slice(adj[id], func(i, j int) bool { return adj[id][i] < adj[id][j] })
	}
	return links, adj
}

func findUndirectedCycles(adj map[int64][]int64, links []topoLink, names map[int64]string) []TopologyCycle {
	linkByPair := map[[2]int64]topoLink{}
	for _, l := range links {
		p := [2]int64{l.a, l.b}
		if _, ok := linkByPair[p]; !ok {
			linkByPair[p] = l
		}
	}

	visited := map[int64]bool{}
	var cycles []TopologyCycle
	seenCycle := map[string]struct{}{}

	var dfs func(u, parent int64, path []int64)
	dfs = func(u, parent int64, path []int64) {
		visited[u] = true
		path = append(path, u)
		for _, v := range adj[u] {
			if v == parent {
				continue
			}
			// back-edge → цикл
			inPath := -1
			for i, id := range path {
				if id == v {
					inPath = i
					break
				}
			}
			if inPath >= 0 {
				cyclePath := append([]int64{}, path[inPath:]...)
				if c, ok := materializeCycle(cyclePath, linkByPair, names); ok {
					key := cycleKey(c.DeviceIDs)
					if _, dup := seenCycle[key]; !dup {
						seenCycle[key] = struct{}{}
						cycles = append(cycles, c)
					}
				}
				continue
			}
			if !visited[v] {
				dfs(v, u, path)
			}
		}
	}

	ids := make([]int64, 0, len(adj))
	for id := range adj {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if !visited[id] {
			dfs(id, -1, nil)
		}
	}

	sort.Slice(cycles, func(i, j int) bool {
		if cycles[i].Length != cycles[j].Length {
			return cycles[i].Length < cycles[j].Length
		}
		return cycles[i].Summary < cycles[j].Summary
	})
	return cycles
}

func materializeCycle(path []int64, linkByPair map[[2]int64]topoLink, names map[int64]string) (TopologyCycle, bool) {
	if len(path) < 3 {
		return TopologyCycle{}, false
	}
	// замкнуть: последний → первый
	closed := append(append([]int64{}, path...), path[0])
	hops := make([]TopologyCycleHop, 0, len(path))
	devNames := make([]string, 0, len(path))
	for _, id := range path {
		n := names[id]
		if n == "" {
			n = fmt.Sprintf("#%d", id)
		}
		devNames = append(devNames, n)
	}
	for i := 0; i < len(closed)-1; i++ {
		from, to := closed[i], closed[i+1]
		a, b := from, to
		if a > b {
			a, b = b, a
		}
		l, ok := linkByPair[[2]int64{a, b}]
		hop := TopologyCycleHop{
			FromDeviceID: from, ToDeviceID: to,
			FromDeviceName: names[from], ToDeviceName: names[to],
			Protocol: "lldp",
		}
		if hop.FromDeviceName == "" {
			hop.FromDeviceName = fmt.Sprintf("#%d", from)
		}
		if hop.ToDeviceName == "" {
			hop.ToDeviceName = fmt.Sprintf("#%d", to)
		}
		if ok {
			hop.Protocol = l.protocol
			if from == l.a {
				hop.FromIfIndex = l.aIf
				hop.FromIfName = l.aIfName
				if l.bIf > 0 {
					bi := l.bIf
					hop.ToIfIndex = &bi
				}
				hop.ToIfName = l.bIfName
			} else {
				hop.FromIfIndex = l.bIf
				hop.FromIfName = l.bIfName
				if l.aIf > 0 {
					ai := l.aIf
					hop.ToIfIndex = &ai
				}
				hop.ToIfName = l.aIfName
			}
		}
		hops = append(hops, hop)
	}
	summary := strings.Join(devNames, " → ") + " → " + devNames[0]
	return TopologyCycle{
		Length:      len(path),
		DeviceIDs:   append([]int64{}, path...),
		DeviceNames: devNames,
		Hops:        hops,
		Summary:     summary,
	}, true
}

func cycleKey(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	// нормализация: ротация к минимальному id, оба направления
	minI := 0
	for i := 1; i < len(ids); i++ {
		if ids[i] < ids[minI] {
			minI = i
		}
	}
	n := len(ids)
	fwd := make([]int64, n)
	rev := make([]int64, n)
	for i := 0; i < n; i++ {
		fwd[i] = ids[(minI+i)%n]
		rev[i] = ids[(minI-i+n)%n]
	}
	sf := fmt.Sprint(fwd)
	sr := fmt.Sprint(rev)
	if sf < sr {
		return sf
	}
	return sr
}

// parallelLinkCyclesFromEdges — ≥2 разных локальных порта с одной стороны пары устройств.
// Обратный LLDP того же кабеля (A:10→B и B:20→A) не считается параллельным линком.
func parallelLinkCyclesFromEdges(edges []store.TopologyEdge, names map[int64]string) []TopologyCycle {
	type pair struct{ a, b int64 }
	type sideIfs map[int64]map[int]string // device -> ifIndex -> ifName
	sides := map[pair]sideIfs{}
	protoByPair := map[pair]string{}
	for _, e := range edges {
		if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 || e.Stale {
			continue
		}
		a, b := e.LocalDeviceID, *e.RemoteDeviceID
		if a > b {
			a, b = b, a
		}
		p := pair{a, b}
		if sides[p] == nil {
			sides[p] = sideIfs{}
		}
		if sides[p][e.LocalDeviceID] == nil {
			sides[p][e.LocalDeviceID] = map[int]string{}
		}
		ifName := ""
		if e.LocalIfName != nil {
			ifName = *e.LocalIfName
		}
		sides[p][e.LocalDeviceID][e.LocalIfIndex] = ifName
		if protoByPair[p] == "" {
			protoByPair[p] = e.Protocol
		}
	}
	var out []TopologyCycle
	for p, side := range sides {
		nA, nB := len(side[p.a]), len(side[p.b])
		if nA < 2 && nB < 2 {
			continue
		}
		na, nb := names[p.a], names[p.b]
		if na == "" {
			na = fmt.Sprintf("#%d", p.a)
		}
		if nb == "" {
			nb = fmt.Sprintf("#%d", p.b)
		}
		hops := make([]TopologyCycleHop, 0)
		for devID, ifs := range side {
			fromName := names[devID]
			if fromName == "" {
				fromName = fmt.Sprintf("#%d", devID)
			}
			toID := p.b
			toName := nb
			if devID == p.b {
				toID = p.a
				toName = na
			}
			for ifIdx, ifName := range ifs {
				hops = append(hops, TopologyCycleHop{
					FromDeviceID: devID, FromDeviceName: fromName,
					FromIfIndex: ifIdx, FromIfName: ifName,
					ToDeviceID: toID, ToDeviceName: toName,
					Protocol: protoByPair[p],
				})
			}
		}
		out = append(out, TopologyCycle{
			Length:      2,
			DeviceIDs:   []int64{p.a, p.b},
			DeviceNames: []string{na, nb},
			Hops:        hops,
			Summary:     fmt.Sprintf("%s ⇄ %s (параллельные порты: %d + %d)", na, nb, nA, nB),
		})
	}
	return out
}

// parallelLinkCycles — legacy helper for unit tests on topoLink slices.
func parallelLinkCycles(links []topoLink, names map[int64]string) []TopologyCycle {
	type pair struct{ a, b int64 }
	byPair := map[pair][]topoLink{}
	for _, l := range links {
		byPair[pair{l.a, l.b}] = append(byPair[pair{l.a, l.b}], l)
	}
	var out []TopologyCycle
	for p, ls := range byPair {
		if len(ls) < 2 {
			continue
		}
		na, nb := names[p.a], names[p.b]
		if na == "" {
			na = fmt.Sprintf("#%d", p.a)
		}
		if nb == "" {
			nb = fmt.Sprintf("#%d", p.b)
		}
		out = append(out, TopologyCycle{
			Length:      2,
			DeviceIDs:   []int64{p.a, p.b},
			DeviceNames: []string{na, nb},
			Summary:     fmt.Sprintf("%s ⇄ %s (%d параллельных LLDP-линков)", na, nb, len(ls)),
		})
	}
	return out
}
