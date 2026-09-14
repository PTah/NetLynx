package topologycache

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/investigate"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

// RebuildResult — итог пересборки кэша.
type RebuildResult struct {
	RootID         int64
	RootSource     string
	Edges          int
	Nodes          int
	VLANEdgeCount  int
	Trigger        string
}

// Rebuild пересобирает кэш: LLDP+CDP+manual, root, dist, VLAN-on-edge.
func Rebuild(ctx context.Context, st *store.Store, trigger string) (RebuildResult, error) {
	var out RebuildResult
	out.Trigger = trigger
	if st == nil {
		return out, fmt.Errorf("store is nil")
	}
	includeStale := false
	g, err := st.BuildTopologyGraphFiltered(ctx, store.TopologyFilter{
		Dedup:        true,
		IncludeStale: &includeStale,
	})
	if err != nil {
		return out, err
	}
	if g == nil {
		return out, fmt.Errorf("empty topology graph")
	}

	filtered := &store.TopologyGraph{Nodes: g.Nodes, Edges: make([]store.TopologyEdge, 0, len(g.Edges))}
	for _, e := range g.Edges {
		if e.Stale {
			continue
		}
		if !blastEdgeProtocolOK(e) {
			continue
		}
		if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 {
			continue
		}
		filtered.Edges = append(filtered.Edges, e)
	}

	adj, _, linkCount := investigate.BuildLLDPDeviceAdj(filtered)
	preferSTP := int64(0)
	if states, serr := st.ListDeviceSTPStates(ctx); serr == nil && len(states) > 0 {
		if chassis, cerr := st.ListChassisMACIndex(ctx); cerr == nil {
			hexMap := map[string]int64{}
			for mac, ep := range chassis {
				hexMap[mac] = ep.ID
			}
			preferSTP = investigate.PreferSTPRootDeviceID(states, hexMap)
		}
	}
	rootID, rootSource := investigate.PickTopologyRootIDPrefer(filtered.Nodes, linkCount, preferSTP)
	dist := investigate.BFSDistances(adj, rootID)

	edges := materializeEdges(filtered.Edges)
	edgeVLANs, vlanCount, verr := computeEdgeVLANs(ctx, st, filtered.Edges)
	if verr != nil {
		return out, verr
	}

	if trigger == "" {
		trigger = "rebuild"
	}
	if err := st.ReplaceTopologyBlastCache(ctx, store.TopologyBlastReplaceInput{
		RootID:     rootID,
		RootSource: rootSource,
		Adj:        adj,
		Dist:       dist,
		Edges:      edges,
		EdgeVLANs:  edgeVLANs,
		Trigger:    trigger,
	}); err != nil {
		return out, err
	}
	out.RootID = rootID
	out.RootSource = rootSource
	out.Edges = countUndirected(adj)
	out.Nodes = len(adj)
	out.VLANEdgeCount = vlanCount
	out.Trigger = trigger
	return out, nil
}

func materializeEdges(directed []store.TopologyEdge) []store.TopologyBlastEdge {
	type acc struct {
		aIf, bIf int
		protos   map[string]struct{}
	}
	m := map[string]*acc{}
	order := make([]string, 0)
	for _, e := range directed {
		if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 {
			continue
		}
		a, b := e.LocalDeviceID, *e.RemoteDeviceID
		aIf, bIf := e.LocalIfIndex, parseRemoteIfIndex(e)
		if a > b {
			a, b = b, a
			aIf, bIf = bIf, aIf
		}
		key := fmt.Sprintf("%d:%d", a, b)
		ac, ok := m[key]
		if !ok {
			ac = &acc{protos: map[string]struct{}{}}
			m[key] = ac
			order = append(order, key)
		}
		if aIf > 0 && ac.aIf == 0 {
			ac.aIf = aIf
		}
		if bIf > 0 && ac.bIf == 0 {
			ac.bIf = bIf
		}
		for _, p := range e.Protocols {
			p = strings.ToLower(strings.TrimSpace(p))
			if p != "" {
				ac.protos[p] = struct{}{}
			}
		}
		if p := strings.ToLower(strings.TrimSpace(e.Protocol)); p != "" {
			ac.protos[p] = struct{}{}
		}
	}
	out := make([]store.TopologyBlastEdge, 0, len(order))
	for _, key := range order {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		a, _ := strconv.ParseInt(parts[0], 10, 64)
		b, _ := strconv.ParseInt(parts[1], 10, 64)
		ac := m[key]
		protos := make([]string, 0, len(ac.protos))
		for p := range ac.protos {
			protos = append(protos, p)
		}
		out = append(out, store.TopologyBlastEdge{
			ADeviceID: a,
			BDeviceID: b,
			AIfIndex:  ac.aIf,
			BIfIndex:  ac.bIf,
			Protocols: strings.Join(protos, ","),
		})
	}
	return out
}

func parseRemoteIfIndex(e store.TopologyEdge) int {
	if e.RemoteIfName != nil {
		if n := ifIndexFromName(*e.RemoteIfName); n > 0 {
			return n
		}
	}
	if e.RemotePortID != nil {
		if n := ifIndexFromName(*e.RemotePortID); n > 0 {
			return n
		}
	}
	return 0
}

func ifIndexFromName(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// "0/14", "Gi0/14", "1"
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	if i := strings.LastIndexByte(s, '/'); i >= 0 && i+1 < len(s) {
		if n, err := strconv.Atoi(s[i+1:]); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

type deviceInvBundle struct {
	inv   []swcfg.VLANInventoryRow
	modes map[string]swcfg.ParsedInterfaceCLI
	names map[int]string
}

func computeEdgeVLANs(ctx context.Context, st *store.Store, directed []store.TopologyEdge) ([]store.TopologyBlastEdgeVLAN, int, error) {
	need := map[int64]struct{}{}
	for _, e := range directed {
		if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 {
			continue
		}
		need[e.LocalDeviceID] = struct{}{}
		need[*e.RemoteDeviceID] = struct{}{}
	}
	bundles := map[int64]*deviceInvBundle{}
	for id := range need {
		b, err := loadDeviceInvBundle(ctx, st, id)
		if err != nil {
			continue // нет конфига — пропустим VLAN на рёбрах этого узла
		}
		bundles[id] = b
	}

	seen := map[string]struct{}{}
	out := make([]store.TopologyBlastEdgeVLAN, 0)
	for _, e := range directed {
		if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 {
			continue
		}
		local := bundles[e.LocalDeviceID]
		remote := bundles[*e.RemoteDeviceID]
		if local == nil || remote == nil {
			continue
		}
		ifName := ""
		if e.LocalIfName != nil {
			ifName = *e.LocalIfName
		}
		if ifName == "" {
			ifName = local.names[e.LocalIfIndex]
		}
		for _, row := range local.inv {
			vid := row.VLANID
			if vid < 2 || vid > 4094 {
				continue
			}
			okPeer, _, _, _ := investigate.MatchVLANInInventory(remote.inv, vid)
			if !okPeer {
				continue
			}
			if !swcfg.VLANCarriedOnPort(local.inv, vid, e.LocalIfIndex, ifName, local.modes) {
				continue
			}
			lo, hi := e.LocalDeviceID, *e.RemoteDeviceID
			if lo > hi {
				lo, hi = hi, lo
			}
			key := fmt.Sprintf("%d:%d:%d", lo, hi, vid)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, store.TopologyBlastEdgeVLAN{
				ADeviceID: lo,
				BDeviceID: hi,
				VLANID:    vid,
			})
		}
	}
	return out, len(out), nil
}

func loadDeviceInvBundle(ctx context.Context, st *store.Store, deviceID int64) (*deviceInvBundle, error) {
	ifs, err := st.ListInterfacesByDevice(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	hints := make([]swcfg.PortVLANHint, 0, len(ifs))
	names := map[int]string{}
	for _, p := range ifs {
		if p.IfIndex <= 0 {
			continue
		}
		name := ifaceName(p)
		low := strings.ToLower(name)
		if strings.HasPrefix(low, "vlan") || strings.Contains(low, "loopback") {
			continue
		}
		h := swcfg.PortVLANHint{
			IfIndex: p.IfIndex,
			IfName:  name,
			Role:    store.ResolveInterfacePortRole(p.PortRole, p.CLIPortMode),
		}
		if p.CliAccessVlan != nil && *p.CliAccessVlan > 0 {
			h.AccessVLAN = p.CliAccessVlan
		}
		hints = append(hints, h)
		names[p.IfIndex] = name
	}
	cfg := ""
	if snap, err := st.GetLatestConfigSnapshot(ctx, deviceID); err == nil && snap != nil {
		cfg = snap.ConfigText
	}
	inv := swcfg.BuildVLANInventory(cfg, hints, nil)
	modes := swcfg.ParseRunningConfigPortModes(cfg)
	return &deviceInvBundle{inv: inv, modes: modes, names: names}, nil
}

func ifaceName(p models.DeviceInterface) string {
	if p.IfName != nil && strings.TrimSpace(*p.IfName) != "" {
		return strings.TrimSpace(*p.IfName)
	}
	return fmt.Sprintf("ifIndex %d", p.IfIndex)
}

func blastEdgeProtocolOK(e store.TopologyEdge) bool {
	check := func(p string) bool {
		p = strings.ToLower(strings.TrimSpace(p))
		switch p {
		case "lldp", "cdp", "manual":
			return true
		default:
			return false
		}
	}
	if check(e.Protocol) {
		return true
	}
	for _, p := range e.Protocols {
		if check(p) {
			return true
		}
	}
	return false
}

func countUndirected(adj map[int64][]int64) int {
	seen := map[string]struct{}{}
	n := 0
	for a, nbrs := range adj {
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
			n++
		}
	}
	return n
}
