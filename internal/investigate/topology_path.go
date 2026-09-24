package investigate

import (
	"context"
	"fmt"
	"sort"
)

// TopologyPathHop — узел на пути A→B.
type TopologyPathHop struct {
	DeviceID   int64  `json:"device_id"`
	DeviceName string `json:"device_name,omitempty"`
	DeviceHost string `json:"device_host,omitempty"`
}

// TopologyPathReport — кратчайший путь по blast-cache.
type TopologyPathReport struct {
	FromID      int64             `json:"from_device_id"`
	ToID        int64             `json:"to_device_id"`
	Hops        []TopologyPathHop `json:"hops"`
	Length      int               `json:"length"` // число рёбер
	Summary     string            `json:"summary"`
	Source      string            `json:"source"`
	RootID      int64             `json:"root_device_id,omitempty"`
	RootSource  string            `json:"root_source,omitempty"`
}

// TopologyReachabilityNode — узел в BFS-срезе от from.
type TopologyReachabilityNode struct {
	DeviceID   int64  `json:"device_id"`
	DeviceName string `json:"device_name,omitempty"`
	DeviceHost string `json:"device_host,omitempty"`
	Hop        int    `json:"hop"`
	DistRoot   *int   `json:"dist_from_root,omitempty"`
}

// TopologyReachabilityReport — BFS от from до maxDepth.
type TopologyReachabilityReport struct {
	FromID   int64                       `json:"from_device_id"`
	MaxDepth int                         `json:"max_depth"`
	Nodes    []TopologyReachabilityNode  `json:"nodes"`
	Source   string                      `json:"source"`
	RootID   int64                       `json:"root_device_id,omitempty"`
}

// BuildTopologyPath — кратчайший путь from→to по blast cache.
func (b *Builder) BuildTopologyPath(ctx context.Context, fromID, toID int64) (*TopologyPathReport, error) {
	g, ok, err := LoadBlastGraph(ctx, b.St)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBlastCacheEmpty
	}
	ids := ShortestPath(g.Adj, fromID, toID, 0)
	if len(ids) == 0 {
		return nil, ErrNoTopologyPath
	}
	names := b.DeviceNames(ctx)
	hosts := map[int64]string{}
	if devs, err := b.St.ListDevices(ctx); err == nil {
		for _, d := range devs {
			hosts[d.ID] = d.Host
			if names[d.ID] == "" {
				names[d.ID] = d.Name
				if names[d.ID] == "" {
					names[d.ID] = d.Host
				}
			}
		}
	}
	hops := make([]TopologyPathHop, 0, len(ids))
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		nm := names[id]
		if nm == "" {
			nm = fmt.Sprintf("#%d", id)
		}
		hops = append(hops, TopologyPathHop{DeviceID: id, DeviceName: nm, DeviceHost: hosts[id]})
		parts = append(parts, nm)
	}
	return &TopologyPathReport{
		FromID:     fromID,
		ToID:       toID,
		Hops:       hops,
		Length:     len(ids) - 1,
		Summary:    joinArrow(parts),
		Source:     g.Source,
		RootID:     g.RootID,
		RootSource: g.RootSource,
	}, nil
}

// BuildTopologyReachability — BFS от from, hop ≤ maxDepth (рёбра).
func (b *Builder) BuildTopologyReachability(ctx context.Context, fromID int64, maxDepth int) (*TopologyReachabilityReport, error) {
	g, ok, err := LoadBlastGraph(ctx, b.St)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrBlastCacheEmpty
	}
	if maxDepth <= 0 {
		maxDepth = 32
	}
	if maxDepth > 10000 {
		maxDepth = 10000
	}
	names := b.DeviceNames(ctx)
	hosts := map[int64]string{}
	if devs, err := b.St.ListDevices(ctx); err == nil {
		for _, d := range devs {
			hosts[d.ID] = d.Host
		}
	}
	type item struct {
		id  int64
		hop int
	}
	seen := map[int64]bool{fromID: true}
	q := []item{{id: fromID, hop: 0}}
	var nodes []TopologyReachabilityNode
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		nm := names[cur.id]
		if nm == "" {
			nm = fmt.Sprintf("#%d", cur.id)
		}
		var dRoot *int
		if g.Dist != nil {
			if d, ok := g.Dist[cur.id]; ok {
				dd := d
				dRoot = &dd
			}
		}
		nodes = append(nodes, TopologyReachabilityNode{
			DeviceID: cur.id, DeviceName: nm, DeviceHost: hosts[cur.id],
			Hop: cur.hop, DistRoot: dRoot,
		})
		if cur.hop >= maxDepth {
			continue
		}
		for _, nxt := range g.Adj[cur.id] {
			if seen[nxt] {
				continue
			}
			seen[nxt] = true
			q = append(q, item{id: nxt, hop: cur.hop + 1})
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Hop != nodes[j].Hop {
			return nodes[i].Hop < nodes[j].Hop
		}
		return nodes[i].DeviceID < nodes[j].DeviceID
	})
	return &TopologyReachabilityReport{
		FromID:   fromID,
		MaxDepth: maxDepth,
		Nodes:    nodes,
		Source:   g.Source,
		RootID:   g.RootID,
	}, nil
}
