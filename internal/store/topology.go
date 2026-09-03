package store

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

type TopologyNode struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Host         string  `json:"host"`
	SysName      *string `json:"sys_name,omitempty"`
	SysDescr     *string `json:"sys_descr,omitempty"`
	CPUProfile   *string `json:"cpu_profile,omitempty"`
	Location     *string `json:"location,omitempty"`
	SNMPOK         *bool   `json:"last_snmp_ok,omitempty"`
	PingOK         *bool   `json:"last_ping_ok,omitempty"`
	OnlineOverride *bool   `json:"online_override"`
	UISPDeviceID   *string `json:"uisp_device_id,omitempty"`
	UISPStatus     *string `json:"uisp_overview_status,omitempty"`
	Virtual        bool    `json:"virtual,omitempty"`
	Kind           string  `json:"kind"`
	LinkCount      int     `json:"link_count"`
	DiscoveredID   *int64  `json:"discovered_id,omitempty"`
}

type TopologyEdge struct {
	LocalDeviceID   int64      `json:"local_device_id"`
	LocalIfIndex    int        `json:"local_if_index"`
	LocalIfName     *string    `json:"local_if_name,omitempty"`
	LocalIfSpeed    *int64     `json:"local_if_speed,omitempty"`
	PoeActive       *bool      `json:"poe_active,omitempty"`
	PoePowerW       *float64   `json:"poe_power_w,omitempty"`
	VlanID          *int       `json:"vlan_id,omitempty"`
	RemoteDeviceID  *int64     `json:"remote_device_id,omitempty"`
	RemoteSysName   *string    `json:"remote_sys_name,omitempty"`
	RemotePortID    *string    `json:"remote_port_id,omitempty"`
	RemoteIfName    *string    `json:"remote_if_name,omitempty"`
	RemoteChassisID *string    `json:"remote_chassis_id,omitempty"`
	RemoteMgmtAddr  *string    `json:"remote_mgmt_addr,omitempty"`
	// Protocol — основной (первый) для совместимости; Protocols — все после дедупа LLDP+CDP.
	Protocol        string     `json:"protocol"`
	Protocols       []string   `json:"protocols,omitempty"`
	RemIndex        int        `json:"rem_index"`
	Stale           bool       `json:"stale"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	UnresolvedLabel string     `json:"unresolved_label,omitempty"`
	ManualLinkID    *int64     `json:"manual_link_id,omitempty"`
	ManualNote      *string    `json:"manual_note,omitempty"`
}

type TopologyGraph struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// TopologyFilter — серверная фильтрация графа.
type TopologyFilter struct {
	Q            string
	DeviceID     *int64
	Protocol     string // lldp | cdp | ""
	IncludeStale *bool  // nil = true
	Depth        *int
	VlanID       *int
	Location     string
	Dedup        bool // default true: слить LLDP+CDP между парой портов
}

type ifaceMeta struct {
	name    *string
	speed   *int64
	poe     *bool
	poeWatt *float64
}

// BuildTopologyGraph собирает узлы inventory и рёбра из port_neighbors с резолвом remote → device.
func (s *Store) BuildTopologyGraph(ctx context.Context) (*TopologyGraph, error) {
	return s.BuildTopologyGraphFiltered(ctx, TopologyFilter{Dedup: true})
}

func (s *Store) BuildTopologyGraphFiltered(ctx context.Context, f TopologyFilter) (*TopologyGraph, error) {
	devices, err := s.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	// Promote раньше мог не писать chassis_mac — подтянуть MAC из discovered (status=added).
	_ = s.applyPromotedChassisHints(ctx, devices)
	neighbors, err := s.ListAllPortNeighbors(ctx)
	if err != nil {
		return nil, err
	}
	ifMeta, err := s.loadAllIfaceMeta(ctx)
	if err != nil {
		return nil, err
	}
	vlanByPort, _ := s.loadDominantVLANs(ctx)
	discIndex, _ := s.loadDiscoveredNameIndex(ctx)

	nodes := make([]TopologyNode, 0, len(devices))
	index := buildDeviceNameIndex(devices)
	byID := make(map[int64]struct{}, len(devices))
	for _, d := range devices {
		nodes = append(nodes, TopologyNode{
			ID: d.ID, Name: d.Name, Host: d.Host,
			SysName: d.SysName, SysDescr: d.SysDescr, CPUProfile: d.CPUProfile,
			Location: d.Location, SNMPOK: d.LastSNMPOK, PingOK: d.LastPingOK,
			OnlineOverride: d.OnlineOverride,
			UISPDeviceID: d.UISPDeviceID, UISPStatus: d.UISPOverviewStatus,
			Kind: classifyTopologyKind(false, d.DeviceCategory, derefStr(d.CPUProfile), derefStr(d.SysDescr),
				strings.TrimSpace(d.Name+" "+derefStr(d.SysName)+" "+d.Host)),
		})
		byID[d.ID] = struct{}{}
	}

	// Active manuals pin both ends: suppress LLDP/CDP edges on those local ports
	// so a user-corrected link is not crowded out by wrong discovery (VLAN/bridge).
	manuals, err := s.ListActiveManualLinks(ctx)
	if err != nil {
		return nil, err
	}
	pinnedPorts := make(map[string]struct{}, len(manuals)*2)
	for _, ml := range manuals {
		pinnedPorts[portKey(ml.ADeviceID, ml.AIfIndex)] = struct{}{}
		pinnedPorts[portKey(ml.BDeviceID, ml.BIfIndex)] = struct{}{}
	}

	edges := make([]TopologyEdge, 0, len(neighbors))
	for _, nb := range neighbors {
		if _, ok := byID[nb.DeviceID]; !ok {
			continue
		}
		if _, pinned := pinnedPorts[portKey(nb.DeviceID, nb.IfIndex)]; pinned {
			continue
		}
		label := derefStr(nb.RemoteSysName)
		if label == "" {
			label = derefStr(nb.RemoteMgmtAddr)
		}
		if label == "" {
			label = derefStr(nb.RemoteChassisID)
		}

		var remoteID *int64
		if rid, ok := resolveRemoteDeviceID(index, nb); ok && rid != nb.DeviceID {
			remoteID = &rid
		}

		meta := ifMeta[nb.DeviceID][nb.IfIndex]
		ls := nb.LastSeenAt
		proto := strings.ToLower(strings.TrimSpace(nb.Protocol))
		if proto == "" {
			proto = "lldp"
		}
		e := TopologyEdge{
			LocalDeviceID:   nb.DeviceID,
			LocalIfIndex:    nb.IfIndex,
			LocalIfName:     meta.name,
			LocalIfSpeed:    meta.speed,
			PoeActive:       meta.poe,
			PoePowerW:       meta.poeWatt,
			VlanID:          vlanByPort[portKey(nb.DeviceID, nb.IfIndex)],
			RemoteDeviceID:  remoteID,
			RemoteSysName:   nb.RemoteSysName,
			RemotePortID:    nb.RemotePortID,
			RemoteIfName:    nb.RemotePortID,
			RemoteChassisID: nb.RemoteChassisID,
			RemoteMgmtAddr:  nb.RemoteMgmtAddr,
			Protocol:        proto,
			Protocols:       []string{proto},
			RemIndex:        nb.RemIndex,
			Stale:           nb.Stale,
			LastSeenAt:      &ls,
		}
		if remoteID == nil && label != "" {
			e.UnresolvedLabel = label
		}
		edges = append(edges, e)
	}

	kindByID := make(map[int64]string, len(nodes))
	for _, n := range nodes {
		kindByID[n.ID] = n.Kind
	}
	edges = filterSpuriousRouterRouterEdges(edges, kindByID)

	virtualSeen := make(map[string]int64)
	nextVirtual := int64(-1)
	for i := range edges {
		if edges[i].RemoteDeviceID != nil {
			continue
		}
		key, lab := virtualPeerIdentity(edges[i])
		if key == "" || lab == "" {
			continue
		}
		vid, ok := virtualSeen[key]
		if !ok {
			vid = nextVirtual
			nextVirtual--
			virtualSeen[key] = vid
			n := TopologyNode{
				ID: vid, Name: lab, Virtual: true,
				Kind: classifyTopologyKind(true, "", "", "", lab),
			}
			if did, ok := discIndex[key]; ok {
				n.DiscoveredID = &did
			} else if did, ok := discIndex[normalizeDeviceKey(lab)]; ok {
				n.DiscoveredID = &did
			}
			nodes = append(nodes, n)
		}
		edges[i].RemoteDeviceID = &vid
	}

	// Active manual links → inventory↔inventory edges
	for _, ml := range manuals {
		if _, ok := byID[ml.ADeviceID]; !ok {
			continue
		}
		if _, ok := byID[ml.BDeviceID]; !ok {
			continue
		}
		var metaA, metaB ifaceMeta
		if m := ifMeta[ml.ADeviceID]; m != nil {
			metaA = m[ml.AIfIndex]
		}
		if m := ifMeta[ml.BDeviceID]; m != nil {
			metaB = m[ml.BIfIndex]
		}
		bIfStr := fmt.Sprintf("%d", ml.BIfIndex)
		rid := ml.BDeviceID
		mid := ml.ID
		edges = append(edges, TopologyEdge{
			LocalDeviceID:  ml.ADeviceID,
			LocalIfIndex:   ml.AIfIndex,
			LocalIfName:    metaA.name,
			LocalIfSpeed:   metaA.speed,
			PoeActive:      metaA.poe,
			PoePowerW:      metaA.poeWatt,
			VlanID:         vlanByPort[portKey(ml.ADeviceID, ml.AIfIndex)],
			RemoteDeviceID: &rid,
			RemotePortID:   &bIfStr,
			RemoteIfName:   metaB.name,
			Protocol:       "manual",
			Protocols:      []string{"manual"},
			RemIndex:       0,
			Stale:          false,
			ManualLinkID:   &mid,
			ManualNote:     ml.Note,
		})
	}

	if f.Dedup {
		edges = dedupeTopologyEdges(edges)
	}

	linkCount := make(map[int64]int, len(nodes))
	for _, e := range edges {
		linkCount[e.LocalDeviceID]++
		if e.RemoteDeviceID != nil {
			linkCount[*e.RemoteDeviceID]++
		}
	}
	for i := range nodes {
		nodes[i].LinkCount = linkCount[nodes[i].ID]
	}

	g := &TopologyGraph{Nodes: nodes, Edges: edges}
	return ApplyTopologyFilter(g, f), nil
}

// FDBPortTopoContext — снимок FDB для attachment / эвристик trunk AP.
type FDBPortTopoContext struct {
	MACCount          map[string]int
	InventoryMACCount map[string]int
	APDeviceCount     map[string]int
	PortRole          map[string]string
	MaxSiblingTrunkAP map[string]int
}

// LoadFDBPortTopoContext — MAC/AP/inventory по портам + max AP на соседних trunk.
func (s *Store) LoadFDBPortTopoContext(ctx context.Context) (*FDBPortTopoContext, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dfe.device_id, dfe.if_index,
			COUNT(*)::int AS mac_n,
			COUNT(DISTINCT d.id) FILTER (WHERE d.id IS NOT NULL)::int AS inv_n,
			COUNT(DISTINCT d.id) FILTER (WHERE d.device_category = $1)::int AS ap_n,
			COALESCE(di.port_role, ''), di.cli_port_mode
		FROM device_fdb_entries dfe
		LEFT JOIN devices d ON d.chassis_mac IS NOT NULL AND btrim(d.chassis_mac) <> ''
			AND lower(replace(replace(d.chassis_mac, ':', ''), '-', ''))
				= lower(replace(replace(dfe.mac, ':', ''), '-', ''))
		LEFT JOIN device_interfaces di ON di.device_id = dfe.device_id AND di.if_index = dfe.if_index
		GROUP BY dfe.device_id, dfe.if_index, di.port_role, di.cli_port_mode`, DeviceCategoryAP)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &FDBPortTopoContext{
		MACCount:          make(map[string]int),
		InventoryMACCount: make(map[string]int),
		APDeviceCount:     make(map[string]int),
		PortRole:          make(map[string]string),
	}
	type portRec struct {
		deviceID int64
		ifIndex  int
		apN      int
		role     string
	}
	var ports []portRec
	for rows.Next() {
		var deviceID int64
		var ifIndex, macN, invN, apN int
		var portRole string
		var cliMode *string
		if err := rows.Scan(&deviceID, &ifIndex, &macN, &invN, &apN, &portRole, &cliMode); err != nil {
			return nil, err
		}
		key := portKey(deviceID, ifIndex)
		out.MACCount[key] = macN
		out.InventoryMACCount[key] = invN
		out.APDeviceCount[key] = apN
		out.PortRole[key] = ResolveInterfacePortRole(portRole, cliMode)
		ports = append(ports, portRec{deviceID: deviceID, ifIndex: ifIndex, apN: apN, role: out.PortRole[key]})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.MaxSiblingTrunkAP = make(map[string]int)
	byDevice := make(map[int64][]portRec)
	for _, p := range ports {
		byDevice[p.deviceID] = append(byDevice[p.deviceID], p)
	}
	for deviceID, list := range byDevice {
		for _, p := range list {
			if p.role != "trunk" {
				continue
			}
			max := 0
			for _, other := range list {
				if other.ifIndex == p.ifIndex || other.role != "trunk" {
					continue
				}
				if other.apN != fdbTopoMaxAPsOnTrunk {
					continue
				}
				max++
			}
			out.MaxSiblingTrunkAP[portKey(deviceID, p.ifIndex)] = max
		}
	}
	return out, nil
}

// PortAPDeviceCountsFromFDB — сколько уникальных AP (inventory) видно на порту по FDB.
func (s *Store) PortAPDeviceCountsFromFDB(ctx context.Context) (map[string]int, error) {
	ctxData, err := s.LoadFDBPortTopoContext(ctx)
	if err != nil {
		return nil, err
	}
	return ctxData.APDeviceCount, nil
}

func portKey(deviceID int64, ifIndex int) string {
	return fmt.Sprintf("%d:%d", deviceID, ifIndex)
}

// filterSpuriousRouterRouterEdges убирает ложные router↔router LLDP/CDP на одном L2-сегменте.
// MikroTik на порту uplink (ether2) часто видит и свитч, и другой роутер в том же VLAN — второе не кабель.
func filterSpuriousRouterRouterEdges(edges []TopologyEdge, kindByID map[int64]string) []TopologyEdge {
	if len(edges) == 0 {
		return edges
	}
	portPeerKinds := make(map[string]map[int64]string)
	for _, e := range edges {
		if e.RemoteDeviceID == nil || e.Protocol == "manual" {
			continue
		}
		pk := portKey(e.LocalDeviceID, e.LocalIfIndex)
		if portPeerKinds[pk] == nil {
			portPeerKinds[pk] = map[int64]string{}
		}
		portPeerKinds[pk][*e.RemoteDeviceID] = kindByID[*e.RemoteDeviceID]
	}
	out := make([]TopologyEdge, 0, len(edges))
	for _, e := range edges {
		if e.Protocol == "manual" || e.RemoteDeviceID == nil {
			out = append(out, e)
			continue
		}
		if kindByID[e.LocalDeviceID] != "router" || kindByID[*e.RemoteDeviceID] != "router" {
			out = append(out, e)
			continue
		}
		if portHasSwitchPeer(portPeerKinds[portKey(e.LocalDeviceID, e.LocalIfIndex)], *e.RemoteDeviceID) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func portHasSwitchPeer(peers map[int64]string, skipID int64) bool {
	for id, kind := range peers {
		if id == skipID {
			continue
		}
		if kind == "switch" {
			return true
		}
	}
	return false
}

func dedupeKey(e TopologyEdge) string {
	rid := int64(0)
	if e.RemoteDeviceID != nil {
		rid = *e.RemoteDeviceID
	}
	if rid != 0 {
		// Один порт → один сосед в inventory: не дробить по разным remote_port_id (MAC vs ether1).
		return fmt.Sprintf("%d|%d|%d", e.LocalDeviceID, e.LocalIfIndex, rid)
	}
	remoteIf := strings.ToLower(derefStr(e.RemotePortID))
	return fmt.Sprintf("%d|%d|0|%s", e.LocalDeviceID, e.LocalIfIndex, remoteIf)
}

func devicePairKey(e TopologyEdge) string {
	if e.RemoteDeviceID == nil {
		return ""
	}
	a, b := e.LocalDeviceID, *e.RemoteDeviceID
	if a <= 0 || b <= 0 {
		return ""
	}
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("p|%d|%d", a, b)
}

// undirectedLinkKey — канонический ключ, если имена портов с обеих сторон согласованы.
func undirectedLinkKey(e TopologyEdge) string {
	if e.RemoteDeviceID == nil || *e.RemoteDeviceID == 0 {
		return ""
	}
	localPort := strings.ToLower(strings.TrimSpace(derefStr(e.LocalIfName)))
	if localPort == "" {
		localPort = fmt.Sprintf("if%d", e.LocalIfIndex)
	}
	remotePort := strings.ToLower(strings.TrimSpace(derefStr(e.RemotePortID)))
	if remotePort == "" {
		remotePort = strings.ToLower(strings.TrimSpace(derefStr(e.RemoteIfName)))
	}
	if remotePort == "" {
		return ""
	}
	aID, aPort := e.LocalDeviceID, localPort
	bID, bPort := *e.RemoteDeviceID, remotePort
	if aID > bID || (aID == bID && aPort > bPort) {
		aID, aPort, bID, bPort = bID, bPort, aID, aPort
	}
	return fmt.Sprintf("u|%d|%s|%d|%s", aID, aPort, bID, bPort)
}

type topologyEdgeAcc struct {
	e         TopologyEdge
	protocols map[string]struct{}
}

func addTopologyEdgeProtocols(a *topologyEdgeAcc, e TopologyEdge) {
	for _, p := range e.Protocols {
		if p == "" {
			p = e.Protocol
		}
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			a.protocols[p] = struct{}{}
		}
	}
	if e.Protocol != "" {
		a.protocols[strings.ToLower(e.Protocol)] = struct{}{}
	}
}

func mergeTopologyEdgeMeta(dst *TopologyEdge, src TopologyEdge) {
	// Живая FDB/LLDP на паре устройств не должна «отравляться» stale ghost на том же порту
	// (типично: rem_index с MAC в port_id vs Ethernet + sysName).
	if !src.Stale {
		dst.Stale = false
	}
	if src.LastSeenAt != nil && (dst.LastSeenAt == nil || src.LastSeenAt.After(*dst.LastSeenAt)) {
		dst.LastSeenAt = src.LastSeenAt
	}
	if dst.VlanID == nil && src.VlanID != nil {
		dst.VlanID = src.VlanID
	}
	if dst.PoeActive == nil && src.PoeActive != nil {
		dst.PoeActive = src.PoeActive
	}
	if dst.PoePowerW == nil && src.PoePowerW != nil {
		dst.PoePowerW = src.PoePowerW
	}
	if dst.LocalIfSpeed == nil && src.LocalIfSpeed != nil {
		dst.LocalIfSpeed = src.LocalIfSpeed
	}
	if dst.ManualLinkID == nil && src.ManualLinkID != nil {
		dst.ManualLinkID = src.ManualLinkID
	}
	if (dst.ManualNote == nil || strings.TrimSpace(*dst.ManualNote) == "") && src.ManualNote != nil {
		dst.ManualNote = src.ManualNote
	}
}

func finalizeTopologyEdgeAcc(a *topologyEdgeAcc) TopologyEdge {
	protos := make([]string, 0, len(a.protocols))
	for p := range a.protocols {
		protos = append(protos, p)
	}
	sort.Strings(protos)
	a.e.Protocols = protos
	if len(protos) > 0 {
		a.e.Protocol = protos[0]
	}
	a.e.RemIndex = 1
	return a.e
}

func mergeEdgeGroup(list []TopologyEdge) TopologyEdge {
	best := list[0]
	for _, e := range list[1:] {
		if topologyEdgeOrientationBetter(best, e) {
			best = e
		}
	}
	a := &topologyEdgeAcc{e: best, protocols: map[string]struct{}{}}
	for _, e := range list {
		addTopologyEdgeProtocols(a, e)
		mergeTopologyEdgeMeta(&a.e, e)
	}
	return finalizeTopologyEdgeAcc(a)
}

// mergeUndirectedDevicePair склеивает A→B и B→A между одной парой узлов.
// Имена портов у MikroTik/EdgeSwitch часто не совпадают (MAC vs ether1 vs 0/17) —
// тогда встречающиеся направления всё равно парятся 1:1.
func mergeUndirectedDevicePair(edges []TopologyEdge) []TopologyEdge {
	if len(edges) < 2 {
		return edges
	}
	lo, hi := edges[0].LocalDeviceID, *edges[0].RemoteDeviceID
	if lo > hi {
		lo, hi = hi, lo
	}
	var fwd, rev []TopologyEdge
	for _, e := range edges {
		if e.LocalDeviceID == lo {
			fwd = append(fwd, e)
		} else {
			rev = append(rev, e)
		}
	}
	if len(fwd) == 0 || len(rev) == 0 {
		return edges
	}
	usedF := make([]bool, len(fwd))
	usedR := make([]bool, len(rev))
	out := make([]TopologyEdge, 0, len(edges))
	match := func(i, j int) {
		out = append(out, mergeEdgeGroup([]TopologyEdge{fwd[i], rev[j]}))
		usedF[i] = true
		usedR[j] = true
	}
	for i := range fwd {
		fk := undirectedLinkKey(fwd[i])
		if fk == "" {
			continue
		}
		for j := range rev {
			if usedR[j] {
				continue
			}
			if undirectedLinkKey(rev[j]) == fk {
				match(i, j)
				break
			}
		}
	}
	for i := range fwd {
		if usedF[i] {
			continue
		}
		for j := range rev {
			if usedR[j] {
				continue
			}
			match(i, j)
			break
		}
	}
	for i := range fwd {
		if !usedF[i] {
			out = append(out, fwd[i])
		}
	}
	for j := range rev {
		if !usedR[j] {
			out = append(out, rev[j])
		}
	}
	return out
}

func topologyEdgeOrientationBetter(cur, cand TopologyEdge) bool {
	if cand.LocalDeviceID < cur.LocalDeviceID {
		return true
	}
	if cand.LocalDeviceID == cur.LocalDeviceID && cand.LocalIfIndex < cur.LocalIfIndex {
		return true
	}
	return false
}

func dedupeTopologyEdges(edges []TopologyEdge) []TopologyEdge {
	// 1) Слить LLDP+CDP на одном направленном ребре.
	m := map[string]*topologyEdgeAcc{}
	order := make([]string, 0, len(edges))
	for _, e := range edges {
		k := dedupeKey(e)
		a, ok := m[k]
		if !ok {
			a = &topologyEdgeAcc{e: e, protocols: map[string]struct{}{}}
			m[k] = a
			order = append(order, k)
		}
		addTopologyEdgeProtocols(a, e)
		mergeTopologyEdgeMeta(&a.e, e)
	}
	directed := make([]TopologyEdge, 0, len(order))
	for _, k := range order {
		directed = append(directed, finalizeTopologyEdgeAcc(m[k]))
	}

	// 2) Слить A→B и B→A (один физический кабель), даже если имена портов не совпали.
	groups := map[string][]TopologyEdge{}
	pOrder := make([]string, 0, len(directed))
	passthrough := make([]TopologyEdge, 0)
	for _, e := range directed {
		pk := devicePairKey(e)
		if pk == "" {
			passthrough = append(passthrough, e)
			continue
		}
		if _, ok := groups[pk]; !ok {
			pOrder = append(pOrder, pk)
		}
		groups[pk] = append(groups[pk], e)
	}
	out := make([]TopologyEdge, 0, len(directed))
	for _, pk := range pOrder {
		out = append(out, mergeUndirectedDevicePair(groups[pk])...)
	}
	out = append(out, passthrough...)
	return out
}

func (s *Store) loadAllIfaceMeta(ctx context.Context) (map[int64]map[int]ifaceMeta, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT device_id, if_index, if_name,
			COALESCE(NULLIF(btrim(descr_override), ''), NULLIF(btrim(cli_description), ''), if_descr) AS if_descr,
			if_high_speed, poe_active, poe_power_w
		FROM device_interfaces`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[int]ifaceMeta{}
	for rows.Next() {
		var deviceID int64
		var ifIndex int
		var ifName, ifDescr *string
		var highSpeed *int64
		var poe *bool
		var poeW *float64
		if err := rows.Scan(&deviceID, &ifIndex, &ifName, &ifDescr, &highSpeed, &poe, &poeW); err != nil {
			return nil, err
		}
		name := ifName
		if name == nil || strings.TrimSpace(*name) == "" {
			name = ifDescr
		}
		if out[deviceID] == nil {
			out[deviceID] = map[int]ifaceMeta{}
		}
		out[deviceID][ifIndex] = ifaceMeta{name: name, speed: highSpeed, poe: poe, poeWatt: poeW}
	}
	return out, rows.Err()
}

func (s *Store) loadDominantVLANs(ctx context.Context) (map[string]*int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (device_id, if_index) device_id, if_index, vlan_id
		FROM (
			SELECT device_id, if_index, vlan_id, COUNT(*) AS c
			FROM device_fdb_entries
			WHERE vlan_id IS NOT NULL
			GROUP BY device_id, if_index, vlan_id
		) t
		ORDER BY device_id, if_index, c DESC`)
	if err != nil {
		return map[string]*int{}, err
	}
	defer rows.Close()
	out := map[string]*int{}
	for rows.Next() {
		var deviceID int64
		var ifIndex, vlan int
		if err := rows.Scan(&deviceID, &ifIndex, &vlan); err != nil {
			return out, err
		}
		v := vlan
		out[portKey(deviceID, ifIndex)] = &v
	}
	return out, rows.Err()
}

// applyPromotedChassisHints заполняет пустой ChassisMAC у devices из remote_chassis_id
// уже добавленных кандидатов (чтобы LLDP MAC → inventory без повторного virtual-узла).
func (s *Store) applyPromotedChassisHints(ctx context.Context, devices []models.Device) error {
	rows, err := s.pool.Query(ctx, `
		SELECT promoted_device_id, remote_chassis_id, identity_key
		FROM discovered_devices
		WHERE status = $1 AND promoted_device_id IS NOT NULL`, DiscoveredStatusAdded)
	if err != nil {
		return err
	}
	defer rows.Close()
	byID := make(map[int64]*string, len(devices))
	for i := range devices {
		byID[devices[i].ID] = devices[i].ChassisMAC
	}
	for rows.Next() {
		var deviceID int64
		var remChassis, identityKey *string
		if err := rows.Scan(&deviceID, &remChassis, &identityKey); err != nil {
			return err
		}
		cur, ok := byID[deviceID]
		if !ok || (cur != nil && strings.TrimSpace(*cur) != "") {
			continue
		}
		mac := ""
		if remChassis != nil {
			if n, ok := NormalizeMACQuery(*remChassis); ok {
				mac = n
			}
		}
		if mac == "" && identityKey != nil {
			key := strings.TrimSpace(strings.ToLower(*identityKey))
			if strings.HasPrefix(key, "chassis:") {
				if n, ok := NormalizeMACQuery(strings.TrimPrefix(key, "chassis:")); ok {
					mac = n
				}
			}
		}
		if mac == "" {
			continue
		}
		m := mac
		for i := range devices {
			if devices[i].ID == deviceID {
				devices[i].ChassisMAC = &m
				byID[deviceID] = &m
				break
			}
		}
	}
	return rows.Err()
}

func (s *Store) loadDiscoveredNameIndex(ctx context.Context) (map[string]int64, error) {
	list, err := s.ListDiscovered(ctx, "new")
	if err != nil {
		return map[string]int64{}, err
	}
	out := map[string]int64{}
	sysCounts := map[string]int{}
	for _, d := range list {
		if k := normalizeDeviceKey(derefStr(d.RemoteSysName)); k != "" {
			sysCounts[k]++
		}
	}
	for _, d := range list {
		if d.IdentityKey != "" {
			if _, exists := out[d.IdentityKey]; !exists {
				out[d.IdentityKey] = d.ID
			}
		}
		// Уникальные mgmt/chassis — для связи virtual↔discovered.
		if mgmt := strings.ToLower(strings.TrimSpace(derefStr(d.RemoteMgmtAddr))); mgmt != "" {
			ak := "addr:" + mgmt
			if _, exists := out[ak]; !exists {
				out[ak] = d.ID
			}
			if _, exists := out[mgmt]; !exists {
				out[mgmt] = d.ID
			}
		}
		if ch := derefStr(d.RemoteChassisID); ch != "" {
			if h := macHexDigits(ch); len(h) == 12 {
				ck := "chassis:" + h
				if _, exists := out[ck]; !exists {
					out[ck] = d.ID
				}
			}
		}
		// Общие model-sysName (SIP-T41S и т.п.) не индексируем — иначе все телефоны → один discovered.
		if k := normalizeDeviceKey(derefStr(d.RemoteSysName)); k != "" && sysCounts[k] == 1 {
			if _, exists := out[k]; !exists {
				out[k] = d.ID
			}
		}
	}
	return out, nil
}

// virtualPeerIdentity — ключ и подпись виртуального соседа.
// Нельзя ключевать только по sysName: SIP-T41S / X210-V2 — модель, не экземпляр.
func virtualPeerIdentity(e TopologyEdge) (key, label string) {
	sys := derefStr(e.RemoteSysName)
	mgmt := derefStr(e.RemoteMgmtAddr)
	ch := derefStr(e.RemoteChassisID)
	port := derefStr(e.RemotePortID)
	key = DiscoveredIdentityKeyWithPort(sys, mgmt, ch, port)
	if key == "" {
		return "", ""
	}
	switch {
	case sys != "" && mgmt != "":
		label = sys + " · " + mgmt
	case sys != "" && ch != "":
		label = sys + " · " + ch
	case sys != "" && port != "" && key != "name:"+normalizeDeviceKey(sys):
		label = sys + " · " + port
	case sys != "":
		label = sys
	case mgmt != "":
		label = mgmt
	case ch != "":
		label = ch
	default:
		label = port
	}
	if label == "" {
		label = key
	}
	return key, label
}

// ApplyTopologyFilter применяет q / device_id / protocol / stale / depth / vlan / location.
func ApplyTopologyFilter(g *TopologyGraph, f TopologyFilter) *TopologyGraph {
	if g == nil {
		return g
	}
	includeStale := true
	if f.IncludeStale != nil {
		includeStale = *f.IncludeStale
	}

	nodes := g.Nodes
	edges := g.Edges

	if loc := strings.TrimSpace(f.Location); loc != "" {
		locL := strings.ToLower(loc)
		keep := map[int64]bool{}
		for _, n := range nodes {
			if strings.Contains(strings.ToLower(derefStr(n.Location)), locL) {
				keep[n.ID] = true
			}
		}
		// 1 hop: AP в «ПТО», свитч в «Серверная» — иначе ребро отбрасывается и точка «без линка».
		for _, e := range edges {
			if e.RemoteDeviceID == nil {
				continue
			}
			a, b := e.LocalDeviceID, *e.RemoteDeviceID
			if keep[a] {
				keep[b] = true
			}
			if keep[b] {
				keep[a] = true
			}
		}
		nodes, edges = filterGraphByNodes(nodes, edges, keep, true)
	}

	if f.DeviceID != nil {
		keep := map[int64]bool{*f.DeviceID: true}
		if f.Depth != nil && *f.Depth >= 0 {
			keep = bfsKeep(nodes, edges, *f.DeviceID, *f.Depth)
		} else {
			// без depth — все узлы, но рёбра инцидентные device_id + соседи 1 hop для контекста
			keep = bfsKeep(nodes, edges, *f.DeviceID, 1)
		}
		nodes, edges = filterGraphByNodes(nodes, edges, keep, false)
	} else if f.Depth != nil && *f.Depth >= 0 {
		// depth без device_id игнорируем
	}

	proto := strings.ToLower(strings.TrimSpace(f.Protocol))
	if proto != "" || !includeStale || f.VlanID != nil {
		filtered := make([]TopologyEdge, 0, len(edges))
		for _, e := range edges {
			if !includeStale && e.Stale {
				continue
			}
			if proto != "" && !edgeHasProtocol(e, proto) {
				continue
			}
			if f.VlanID != nil && (e.VlanID == nil || *e.VlanID != *f.VlanID) {
				continue
			}
			filtered = append(filtered, e)
		}
		edges = filtered
		// оставить узлы, связанные оставшимися рёбрами + изолированные inventory при отсутствии edge-фильтров по vlan?
		if f.VlanID != nil || proto != "" {
			keep := map[int64]bool{}
			for _, e := range edges {
				keep[e.LocalDeviceID] = true
				if e.RemoteDeviceID != nil {
					keep[*e.RemoteDeviceID] = true
				}
			}
			nodes, edges = filterGraphByNodes(nodes, edges, keep, false)
		} else if !includeStale {
			// узлы без изменений
		}
	}

	if q := strings.TrimSpace(f.Q); q != "" {
		return FilterTopologyGraph(&TopologyGraph{Nodes: nodes, Edges: edges}, q)
	}
	return &TopologyGraph{Nodes: nodes, Edges: edges}
}

func edgeHasProtocol(e TopologyEdge, proto string) bool {
	if strings.EqualFold(e.Protocol, proto) {
		return true
	}
	for _, p := range e.Protocols {
		if strings.EqualFold(p, proto) {
			return true
		}
	}
	return false
}

func bfsKeep(nodes []TopologyNode, edges []TopologyEdge, start int64, depth int) map[int64]bool {
	adj := map[int64][]int64{}
	ids := map[int64]bool{}
	for _, n := range nodes {
		ids[n.ID] = true
	}
	for _, e := range edges {
		if e.RemoteDeviceID == nil {
			continue
		}
		a, b := e.LocalDeviceID, *e.RemoteDeviceID
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	keep := map[int64]bool{}
	if !ids[start] {
		return keep
	}
	type item struct {
		id int64
		d  int
	}
	q := []item{{start, 0}}
	keep[start] = true
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if cur.d >= depth {
			continue
		}
		for _, nb := range adj[cur.id] {
			if keep[nb] {
				continue
			}
			keep[nb] = true
			q = append(q, item{nb, cur.d + 1})
		}
	}
	return keep
}

func filterGraphByNodes(nodes []TopologyNode, edges []TopologyEdge, keep map[int64]bool, dropIsolated bool) ([]TopologyNode, []TopologyEdge) {
	outE := make([]TopologyEdge, 0, len(edges))
	for _, e := range edges {
		if !keep[e.LocalDeviceID] {
			continue
		}
		if e.RemoteDeviceID != nil && !keep[*e.RemoteDeviceID] {
			continue
		}
		outE = append(outE, e)
		if e.RemoteDeviceID != nil {
			keep[*e.RemoteDeviceID] = true
		}
	}
	if dropIsolated {
		linked := map[int64]bool{}
		for _, e := range outE {
			linked[e.LocalDeviceID] = true
			if e.RemoteDeviceID != nil {
				linked[*e.RemoteDeviceID] = true
			}
		}
		for id := range keep {
			if !linked[id] {
				// оставляем узлы, явно попавшие в keep по location даже без рёбер
			}
		}
	}
	outN := make([]TopologyNode, 0, len(keep))
	for _, n := range nodes {
		if keep[n.ID] {
			outN = append(outN, n)
		}
	}
	return outN, outE
}

// FilterTopologyGraph фильтрует граф по подстроке q (имя/host/sysName/порт/сосед/chassis).
func FilterTopologyGraph(g *TopologyGraph, q string) *TopologyGraph {
	q = strings.ToLower(strings.TrimSpace(q))
	if g == nil || q == "" {
		return g
	}
	matchNode := map[int64]bool{}
	for _, n := range g.Nodes {
		blob := strings.ToLower(strings.Join([]string{
			n.Name, n.Host, derefStr(n.SysName), derefStr(n.Location),
		}, " "))
		if strings.Contains(blob, q) {
			matchNode[n.ID] = true
		}
	}
	matchEdge := make([]bool, len(g.Edges))
	keepNode := map[int64]bool{}
	for i, e := range g.Edges {
		blob := strings.ToLower(strings.Join([]string{
			derefStr(e.LocalIfName),
			derefStr(e.RemoteSysName),
			derefStr(e.RemotePortID),
			derefStr(e.RemoteChassisID),
			derefStr(e.RemoteMgmtAddr),
			e.UnresolvedLabel,
			e.Protocol,
			strings.Join(e.Protocols, " "),
		}, " "))
		hit := strings.Contains(blob, q) || matchNode[e.LocalDeviceID] ||
			(e.RemoteDeviceID != nil && matchNode[*e.RemoteDeviceID])
		matchEdge[i] = hit
		if hit {
			keepNode[e.LocalDeviceID] = true
			if e.RemoteDeviceID != nil {
				keepNode[*e.RemoteDeviceID] = true
			}
		}
	}
	for id := range matchNode {
		keepNode[id] = true
	}
	nodes := make([]TopologyNode, 0, len(keepNode))
	for _, n := range g.Nodes {
		if keepNode[n.ID] {
			nodes = append(nodes, n)
		}
	}
	edges := make([]TopologyEdge, 0)
	for i, e := range g.Edges {
		if matchEdge[i] {
			edges = append(edges, e)
		}
	}
	return &TopologyGraph{Nodes: nodes, Edges: edges}
}

// EnrichNeighborsRemoteDeviceID проставляет remote_device_id через тот же резолв, что и топология.
func (s *Store) EnrichNeighborsRemoteDeviceID(ctx context.Context, neighbors []PortNeighbor) ([]PortNeighbor, error) {
	if len(neighbors) == 0 {
		return neighbors, nil
	}
	devices, err := s.ListDevices(ctx)
	if err != nil {
		return neighbors, err
	}
	index := buildDeviceNameIndex(devices)
	out := make([]PortNeighbor, len(neighbors))
	copy(out, neighbors)
	for i := range out {
		if rid, ok := resolveRemoteDeviceID(index, out[i]); ok {
			id := rid
			out[i].RemoteDeviceID = &id
		}
	}
	return out, nil
}

func classifyTopologyKind(virtual bool, deviceCategory, cpuProfile, sysDescr, name string) string {
	if virtual {
		return "virtual"
	}
	cat := NormalizeDeviceCategory(deviceCategory)
	switch cat {
	case DeviceCategoryRouter:
		return "router"
	case DeviceCategoryServer:
		return "server"
	case DeviceCategoryComputer:
		return "computer"
	case DeviceCategoryPhone:
		return "phone"
	case DeviceCategoryMFU:
		return "mfu"
	case DeviceCategoryCamera:
		return "camera"
	case DeviceCategoryAP:
		return "ap"
	case DeviceCategoryOther:
		return "other"
	case DeviceCategorySwitch:
		// ниже — эвристика router vs switch по sysDescr
	default:
		// Пользовательский тип (tv, rack, ilo, …) — как в справочнике, без эвристик.
		if cat != "" {
			return cat
		}
	}
	blob := strings.ToLower(strings.TrimSpace(cpuProfile + " " + sysDescr + " " + name))
	phoneHints := []string{
		"sip-t", "sip ", "yealink", "grandstream", "ip phone", "voip phone",
		"snom", "fanvil", "cisco ip phone", "polycom",
	}
	for _, h := range phoneHints {
		if strings.Contains(blob, h) {
			return "phone"
		}
	}
	if looksLikeIPCameraHint(blob) {
		return "camera"
	}
	if strings.Contains(blob, "printer") || strings.Contains(blob, "mfp") || strings.Contains(blob, "мфу") {
		return "mfu"
	}
	apHints := []string{
		"access point", "uap-", "unifi ap", "unifi 6", "unifi 7",
		"nanostation", "litebeam", "powerbeam", "eap-", "wifi ap", "wlan ap",
	}
	for _, h := range apHints {
		if strings.Contains(blob, h) {
			return "ap"
		}
	}
	otherHints := []string{
		"workstation", "windows", "linux host", "desktop", "notebook",
	}
	for _, h := range otherHints {
		if strings.Contains(blob, h) {
			return "other"
		}
	}
	// Роутеры / шлюзы — отдельно от L2-свитчей (для корня дерева топологии).
	routerHints := []string{
		"ccr", "chr", "edgerouter", "edge router", "gateway", "firewall",
		"rb4011", "rb5009", "rb1100", "rb3011", "hex s", "hex poe",
	}
	for _, h := range routerHints {
		if strings.Contains(blob, h) {
			return "router"
		}
	}
	// «router» как отдельное слово (не routerboard / routeswitch).
	if strings.Contains(blob, "router") && !strings.Contains(blob, "routerboard") && !strings.Contains(blob, "edgeswitch") {
		return "router"
	}
	switchHints := []string{
		"switch", "edgeswitch", "cisco", "ios", "nx-os", "catalyst",
		"mikrotik", "routerboard", "snr", "eltex", "juniper", "junos", "ubiquiti",
		"es-", "usw-", "pro max", "fastpath", "comware", "crs", "css",
	}
	for _, h := range switchHints {
		if strings.Contains(blob, h) {
			return "switch"
		}
	}
	if NormalizeDeviceCategory(deviceCategory) == DeviceCategorySwitch {
		return "switch"
	}
	return "switch"
}

type deviceNameIndex struct {
	byKey map[string]int64
}

func buildDeviceNameIndex(devices []models.Device) deviceNameIndex {
	counts := map[string]map[int64]struct{}{}
	addCount := func(key string, id int64) {
		if key == "" {
			return
		}
		m := counts[key]
		if m == nil {
			m = map[int64]struct{}{}
			counts[key] = m
		}
		m[id] = struct{}{}
	}
	for _, d := range devices {
		// host / IP — сильный идентификатор
		for _, nk := range expandNameKeys(d.Host) {
			addCount(nk, d.ID)
			if looksLikeIPKey(nk) {
				addCount("addr:"+nk, d.ID)
			}
		}
		// chassis MAC — главный ключ для LLDP rem chassis / port-MAC
		if mac := derefStr(d.ChassisMAC); mac != "" {
			if norm, ok := NormalizeMACQuery(mac); ok {
				h := macHexDigits(norm)
				if len(h) == 12 {
					addCount("chassis:"+h, d.ID)
					addCount(h, d.ID)
					addCount(norm, d.ID)
				}
			}
		}
		// name / sysName — только если не дублируются между устройствами
		keys := []string{d.Name}
		if d.SysName != nil {
			keys = append(keys, *d.SysName)
		}
		for _, k := range keys {
			for _, nk := range expandNameKeys(k) {
				addCount(nk, d.ID)
			}
		}
	}
	idx := deviceNameIndex{byKey: make(map[string]int64, len(counts))}
	for key, ids := range counts {
		if len(ids) != 1 {
			continue // SIP-T41S / EdgeSwitch на нескольких узлах — не матчим по имени
		}
		for id := range ids {
			idx.byKey[key] = id
		}
	}
	return idx
}

// resolveRemoteDeviceID: MAC → mgmt IP → sysName/hostname.
func resolveRemoteDeviceID(idx deviceNameIndex, nb PortNeighbor) (int64, bool) {
	try := func(keys ...string) (int64, bool) {
		for _, k := range keys {
			k = strings.ToLower(strings.TrimSpace(k))
			if k == "" {
				continue
			}
			if id, ok := idx.byKey[k]; ok {
				return id, true
			}
		}
		return 0, false
	}
	// 1) MAC (chassis LLDP или Port ID у телефонов)
	for _, raw := range []string{derefStr(nb.RemoteChassisID), derefStr(nb.RemotePortID)} {
		if mac, ok := NormalizeMACQuery(raw); ok {
			h := macHexDigits(mac)
			if len(h) == 12 {
				if id, ok := try("chassis:"+h, h, mac); ok {
					return id, true
				}
			}
		}
	}
	// 2) mgmt IP
	if mgmt := strings.ToLower(strings.TrimSpace(derefStr(nb.RemoteMgmtAddr))); mgmt != "" {
		if id, ok := try(mgmt, "addr:"+mgmt); ok {
			return id, true
		}
		for _, nk := range expandNameKeys(mgmt) {
			if id, ok := try(nk); ok {
				return id, true
			}
		}
	}
	// 3) sysName / hostname
	if sys := derefStr(nb.RemoteSysName); sys != "" {
		for _, nk := range expandNameKeys(sys) {
			if id, ok := try(nk); ok {
				return id, true
			}
		}
	}
	return 0, false
}

func expandNameKeys(raw string) []string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	lower := strings.ToLower(s)
	out := []string{lower}
	// Для IP не отрезаем «домен» по первой точке: иначе 192.168.1.10 и 48.47.50.52
	// дают ключи "192"/"48" и ложно схлопывают соседей на первый device с таким префиксом.
	if looksLikeIPKey(lower) {
		return out
	}
	if i := strings.IndexByte(lower, '.'); i > 0 {
		out = append(out, lower[:i])
	}
	return out
}

// looksLikeIPCameraHint — камера; video-LAN свитчи (DS-3E, DH-PFS, …) не считаем камерами.
func looksLikeIPCameraHint(blob string) bool {
	if strings.Contains(blob, "dh-pfs") || strings.Contains(blob, "pfs42") ||
		strings.Contains(blob, "ds-3e") || strings.Contains(blob, "poe switch") ||
		strings.Contains(blob, "managed switch") ||
		(strings.Contains(blob, "switch") && (strings.Contains(blob, "dahua") ||
			strings.Contains(blob, "hikvision") || strings.Contains(blob, "hiwatch") ||
			strings.Contains(blob, "trassir"))) {
		return false
	}
	for _, h := range []string{
		"camera", "ipcam", "ip camera", "ipc-", "ds-2cd", "ds-2de", "dh-ipc",
	} {
		if strings.Contains(blob, h) {
			return true
		}
	}
	// Бренд video без маркеров свитча (типичный OUI/sysDescr камеры в FDB).
	if (strings.Contains(blob, "hikvision") || strings.Contains(blob, "dahua") ||
		strings.Contains(blob, "hiwatch") || strings.Contains(blob, "trassir")) &&
		!strings.Contains(blob, "switch") && !strings.Contains(blob, "pfs") {
		return true
	}
	return false
}

func looksLikeIPKey(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	// IPv4 без полной валидации ParseIP (редко), но с 3 точками — не hostname.
	if strings.Count(s, ".") == 3 {
		for _, p := range strings.Split(s, ".") {
			if p == "" {
				return false
			}
			for _, c := range p {
				if c < '0' || c > '9' {
					return false
				}
			}
		}
		return true
	}
	return false
}

func normalizeDeviceKey(s string) string {
	keys := expandNameKeys(s)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}
