package store

import (
	"context"
	"fmt"
	"time"
)

// TopologyBlastEdge — undirected device pair + порты (если известны).
type TopologyBlastEdge struct {
	ADeviceID int64
	BDeviceID int64
	AIfIndex  int // 0 = неизвестно
	BIfIndex  int
	Protocols string
}

// TopologyBlastCache — материализованный undirected adj + dist + VLAN на рёбрах.
type TopologyBlastCache struct {
	RootDeviceID int64
	RootSource   string
	EdgeCount    int
	NodeCount    int
	RebuiltAt    time.Time
	Adj          map[int64][]int64
	Dist         map[int64]int
	Edges        []TopologyBlastEdge
	// EdgeVLANs[lo][hi] → set of vlan IDs (lo < hi)
	EdgeVLANs map[int64]map[int64]map[int]struct{}
}

// EdgeCarriesVLAN — VLAN X на undirected ребре a↔b.
func (c *TopologyBlastCache) EdgeCarriesVLAN(a, b int64, vlanID int) bool {
	if c == nil || c.EdgeVLANs == nil || vlanID < 1 {
		return false
	}
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	m := c.EdgeVLANs[lo]
	if m == nil {
		return false
	}
	set := m[hi]
	if set == nil {
		return false
	}
	_, ok := set[vlanID]
	return ok
}

// HasEdgeVLANs — кэш уже содержит VLAN-on-edge (после 0.8.0 rebuild).
func (c *TopologyBlastCache) HasEdgeVLANs() bool {
	if c == nil {
		return false
	}
	for _, m := range c.EdgeVLANs {
		for _, set := range m {
			if len(set) > 0 {
				return true
			}
		}
	}
	return false
}

// LoadTopologyBlastCache читает кэш; ok=false если ещё не собирали или пусто.
func (s *Store) LoadTopologyBlastCache(ctx context.Context) (*TopologyBlastCache, bool, error) {
	var rootID *int64
	var rootSource string
	var edgeCount, nodeCount int
	var rebuiltAt time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT root_device_id, COALESCE(root_source, ''), edge_count, node_count, rebuilt_at
		FROM topology_blast_meta WHERE id = 1`).Scan(&rootID, &rootSource, &edgeCount, &nodeCount, &rebuiltAt)
	if err != nil {
		return nil, false, fmt.Errorf("topology_blast_meta: %w", err)
	}
	if edgeCount <= 0 && (rootID == nil || *rootID <= 0) {
		return nil, false, nil
	}

	out := &TopologyBlastCache{
		RootSource: rootSource,
		EdgeCount:  edgeCount,
		NodeCount:  nodeCount,
		RebuiltAt:  rebuiltAt,
		Adj:        map[int64][]int64{},
		Dist:       map[int64]int{},
		Edges:      nil,
		EdgeVLANs:  map[int64]map[int64]map[int]struct{}{},
	}
	if rootID != nil {
		out.RootDeviceID = *rootID
	}

	erows, err := s.pool.Query(ctx, `
		SELECT a_device_id, b_device_id,
		       COALESCE(a_if_index, 0), COALESCE(b_if_index, 0), COALESCE(protocols, '')
		FROM topology_blast_edges`)
	if err != nil {
		return nil, false, fmt.Errorf("topology_blast_edges: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var e TopologyBlastEdge
		if err := erows.Scan(&e.ADeviceID, &e.BDeviceID, &e.AIfIndex, &e.BIfIndex, &e.Protocols); err != nil {
			return nil, false, err
		}
		out.Edges = append(out.Edges, e)
		out.Adj[e.ADeviceID] = append(out.Adj[e.ADeviceID], e.BDeviceID)
		out.Adj[e.BDeviceID] = append(out.Adj[e.BDeviceID], e.ADeviceID)
	}
	if err := erows.Err(); err != nil {
		return nil, false, err
	}

	drows, err := s.pool.Query(ctx, `SELECT device_id, dist FROM topology_blast_dist`)
	if err != nil {
		return nil, false, fmt.Errorf("topology_blast_dist: %w", err)
	}
	defer drows.Close()
	for drows.Next() {
		var id int64
		var d int
		if err := drows.Scan(&id, &d); err != nil {
			return nil, false, err
		}
		out.Dist[id] = d
	}
	if err := drows.Err(); err != nil {
		return nil, false, err
	}

	vrows, err := s.pool.Query(ctx, `SELECT a_device_id, b_device_id, vlan_id FROM topology_blast_edge_vlans`)
	if err != nil {
		// миграция 071 ещё не применена на старом процессе — adj всё равно полезен
		if len(out.Adj) == 0 {
			return out, false, nil
		}
		return out, true, nil
	}
	defer vrows.Close()
	for vrows.Next() {
		var a, b int64
		var vid int
		if err := vrows.Scan(&a, &b, &vid); err != nil {
			return nil, false, err
		}
		if out.EdgeVLANs[a] == nil {
			out.EdgeVLANs[a] = map[int64]map[int]struct{}{}
		}
		if out.EdgeVLANs[a][b] == nil {
			out.EdgeVLANs[a][b] = map[int]struct{}{}
		}
		out.EdgeVLANs[a][b][vid] = struct{}{}
	}
	if err := vrows.Err(); err != nil {
		return nil, false, err
	}

	if len(out.Adj) == 0 {
		return out, false, nil
	}
	return out, true, nil
}

// TopologyBlastReplaceInput — полный снимок для ReplaceTopologyBlastCache.
type TopologyBlastReplaceInput struct {
	RootID     int64
	RootSource string
	Adj        map[int64][]int64
	Dist       map[int64]int
	Edges      []TopologyBlastEdge
	EdgeVLANs  []TopologyBlastEdgeVLAN // flattened
	Trigger    string
}

// TopologyBlastEdgeVLAN — VLAN на undirected ребре.
type TopologyBlastEdgeVLAN struct {
	ADeviceID int64
	BDeviceID int64
	VLANID    int
}

// ReplaceTopologyBlastCache атомарно перезаписывает meta + edges + dist + edge VLANs + history row.
func (s *Store) ReplaceTopologyBlastCache(ctx context.Context, in TopologyBlastReplaceInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM topology_blast_edge_vlans`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_blast_edges`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM topology_blast_dist`); err != nil {
		return err
	}

	edges := in.Edges
	if len(edges) == 0 {
		seen := map[string]struct{}{}
		for a, nbrs := range in.Adj {
			for _, b := range nbrs {
				if a <= 0 || b <= 0 || a == b {
					continue
				}
				lo, hi := a, b
				if lo > hi {
					lo, hi = hi, lo
				}
				key := fmt.Sprintf("%d:%d", lo, hi)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				edges = append(edges, TopologyBlastEdge{ADeviceID: lo, BDeviceID: hi})
			}
		}
	}

	for _, e := range edges {
		lo, hi := e.ADeviceID, e.BDeviceID
		aIf, bIf := e.AIfIndex, e.BIfIndex
		if lo > hi {
			lo, hi = hi, lo
			aIf, bIf = bIf, aIf
		}
		if lo <= 0 || hi <= 0 || lo == hi {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO topology_blast_edges (a_device_id, b_device_id, a_if_index, b_if_index, protocols)
			VALUES ($1, $2, NULLIF($3, 0), NULLIF($4, 0), $5)
			ON CONFLICT (a_device_id, b_device_id) DO UPDATE SET
				a_if_index = COALESCE(EXCLUDED.a_if_index, topology_blast_edges.a_if_index),
				b_if_index = COALESCE(EXCLUDED.b_if_index, topology_blast_edges.b_if_index),
				protocols = CASE
					WHEN topology_blast_edges.protocols = '' THEN EXCLUDED.protocols
					WHEN EXCLUDED.protocols = '' THEN topology_blast_edges.protocols
					WHEN position(EXCLUDED.protocols in topology_blast_edges.protocols) > 0 THEN topology_blast_edges.protocols
					ELSE topology_blast_edges.protocols || ',' || EXCLUDED.protocols
				END`,
			lo, hi, aIf, bIf, e.Protocols); err != nil {
			return err
		}
	}
	var edgeCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM topology_blast_edges`).Scan(&edgeCount); err != nil {
		return err
	}

	vlanEdgeCount := 0
	seenVLAN := map[string]struct{}{}
	for _, v := range in.EdgeVLANs {
		if v.VLANID < 1 || v.VLANID > 4094 {
			continue
		}
		lo, hi := v.ADeviceID, v.BDeviceID
		if lo > hi {
			lo, hi = hi, lo
		}
		key := fmt.Sprintf("%d:%d:%d", lo, hi, v.VLANID)
		if _, ok := seenVLAN[key]; ok {
			continue
		}
		seenVLAN[key] = struct{}{}
		if _, err := tx.Exec(ctx, `
			INSERT INTO topology_blast_edge_vlans (a_device_id, b_device_id, vlan_id)
			VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, lo, hi, v.VLANID); err != nil {
			return err
		}
		vlanEdgeCount++
	}

	for id, d := range in.Dist {
		if id <= 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO topology_blast_dist (device_id, dist) VALUES ($1, $2)`, id, d); err != nil {
			return err
		}
	}

	nodeCount := len(in.Adj)
	var rootArg interface{}
	if in.RootID > 0 {
		rootArg = in.RootID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE topology_blast_meta SET
			root_device_id = $1,
			root_source = $2,
			edge_count = $3,
			node_count = $4,
			rebuilt_at = now()
		WHERE id = 1`, rootArg, in.RootSource, edgeCount, nodeCount); err != nil {
		return err
	}

	trigger := in.Trigger
	if trigger == "" {
		trigger = "rebuild"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO topology_blast_history
			(trigger, root_device_id, root_source, edge_count, node_count, vlan_edge_count)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		trigger, rootArg, in.RootSource, edgeCount, nodeCount, vlanEdgeCount); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// PruneTopologyBlastHistory удаляет записи старше retainDays. Возвращает число удалённых.
func (s *Store) PruneTopologyBlastHistory(ctx context.Context, retainDays int) (int64, error) {
	if retainDays < 1 {
		retainDays = 30
	}
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM topology_blast_history
		WHERE rebuilt_at < now() - make_interval(days => $1)`, retainDays)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
