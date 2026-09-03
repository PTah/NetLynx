package investigate

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// MoveGraph — карта перемещений MAC между портами (для UI).
type MoveGraph struct {
	Nodes []MoveGraphNode `json:"nodes"`
	Edges []MoveGraphEdge `json:"edges"`
}

type MoveGraphNode struct {
	ID         string `json:"id"`
	DeviceID   int64  `json:"device_id"`
	DeviceName string `json:"device_name,omitempty"`
	DeviceHost string `json:"device_host,omitempty"`
	IfIndex    int    `json:"if_index"`
	IfName     string `json:"if_name,omitempty"`
	Label      string `json:"label"`
}

type MoveGraphEdge struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
	Sources  []string  `json:"sources,omitempty"`
}

func buildMoveGraph(timeline []TimelineEntry, footprint []FootprintEntry, ifNames map[string]string) MoveGraph {
	if ifNames == nil {
		ifNames = map[string]string{}
	}
	for _, f := range footprint {
		key := nodeKey(f.DeviceID, f.IfIndex)
		if ifNames[key] != "" {
			continue
		}
		if f.IfName != nil && *f.IfName != "" {
			ifNames[key] = *f.IfName
		} else if f.IfDescr != nil && *f.IfDescr != "" {
			ifNames[key] = *f.IfDescr
		}
	}

	nodes := map[string]MoveGraphNode{}
	type edgeKey struct{ a, b string }
	edges := map[edgeKey]*MoveGraphEdge{}

	ensureNode := func(deviceID int64, ifIndex int, name, host string) string {
		id := nodeKey(deviceID, ifIndex)
		if _, ok := nodes[id]; ok {
			return id
		}
		portLabel := ifNames[id]
		if portLabel == "" {
			portLabel = fmt.Sprintf("if%d", ifIndex)
		}
		devLabel := name
		if devLabel == "" {
			devLabel = fmt.Sprintf("#%d", deviceID)
		}
		nodes[id] = MoveGraphNode{
			ID: id, DeviceID: deviceID, DeviceName: name, DeviceHost: host,
			IfIndex: ifIndex, IfName: portLabel,
			Label: fmt.Sprintf("%s · %s", shortName(devLabel), portLabel),
		}
		return id
	}

	for _, m := range timeline {
		if m.FromIfIndex == nil || m.ToIfIndex == nil {
			continue
		}
		if *m.FromIfIndex == *m.ToIfIndex {
			continue
		}
		fromID := ensureNode(m.DeviceID, *m.FromIfIndex, m.DeviceName, m.DeviceHost)
		toID := ensureNode(m.DeviceID, *m.ToIfIndex, m.DeviceName, m.DeviceHost)
		// неориентированное ребро для толщины (a<b), но храним направление последнего хода
		a, b := fromID, toID
		undirected := edgeKey{a: a, b: b}
		if a > b {
			undirected = edgeKey{a: b, b: a}
		}
		e, ok := edges[undirected]
		if !ok {
			e = &MoveGraphEdge{From: fromID, To: toID, Count: 0}
			edges[undirected] = e
		}
		e.Count++
		e.From = fromID
		e.To = toID
		if m.SeenAt.After(e.LastSeen) {
			e.LastSeen = m.SeenAt
		}
		src := m.Source
		if src == "" {
			src = "fdb_poll"
		}
		found := false
		for _, s := range e.Sources {
			if s == src {
				found = true
				break
			}
		}
		if !found {
			e.Sources = append(e.Sources, src)
		}
	}

	// footprint-only nodes (без moves) — чтобы видеть текущее положение
	for _, f := range footprint {
		ensureNode(f.DeviceID, f.IfIndex, f.DeviceName, f.DeviceHost)
	}

	out := MoveGraph{}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, n)
	}
	sort.Slice(out.Nodes, func(i, j int) bool {
		if out.Nodes[i].DeviceID != out.Nodes[j].DeviceID {
			return out.Nodes[i].DeviceID < out.Nodes[j].DeviceID
		}
		return out.Nodes[i].IfIndex < out.Nodes[j].IfIndex
	})
	for _, e := range edges {
		out.Edges = append(out.Edges, *e)
	}
	sort.Slice(out.Edges, func(i, j int) bool {
		return out.Edges[i].Count > out.Edges[j].Count
	})
	return out
}

func nodeKey(deviceID int64, ifIndex int) string {
	return fmt.Sprintf("%d:%d", deviceID, ifIndex)
}

func shortName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 18 {
		return s
	}
	return s[:16] + "…"
}
