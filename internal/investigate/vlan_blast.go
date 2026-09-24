package investigate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

// Глубина обхода нижестоящих с VLAN (fallback, если MaxDepth не задан).
// 0 в конфиге = без лимита (см. EffectiveVLANBlastMaxDepth).
const DefaultVLANBlastMaxDepth = 32

// VLANBlastMaxDepth — устаревший алиас константы (совместимость вызовов).
const VLANBlastMaxDepth = DefaultVLANBlastMaxDepth

// EffectiveVLANBlastMaxDepth нормализует depth: <0 → default 32; 0 → «без лимита» (большой cap).
func EffectiveVLANBlastMaxDepth(configured int) int {
	if configured < 0 {
		return DefaultVLANBlastMaxDepth
	}
	if configured == 0 {
		return 1_000_000
	}
	return configured
}

// Направления линка для delete-impact / blast-radius.
const (
	LinkDirUp           = "up"
	LinkDirDown         = "down"
	LinkDirPeer         = "peer"
	LinkDirUnknown      = "unknown"
	LinkDirOverrideUp   = "override_up"
	LinkDirOverrideDown = "override_down"
)

// VLANBlastHit — затронутый узел при удалении VLAN с устройства Y.
type VLANBlastHit struct {
	VLANID           int     `json:"vlan_id"`
	VLANName         string  `json:"vlan_name,omitempty"`
	LocalIfIndex     int     `json:"local_if_index"`
	LocalIfName      string  `json:"local_if_name,omitempty"`
	LinkDirection    string  `json:"link_direction,omitempty"`
	Reason           string  `json:"reason,omitempty"` // neighbor_has_vlan | descendant
	Hop              int     `json:"hop"`               // 1 = прямой сосед, >1 = нижестоящий
	Redundant        bool    `json:"redundant,omitempty"` // есть путь к root в обход Y
	Protocol         string  `json:"protocol,omitempty"`
	RemotePortID     *string `json:"remote_port_id,omitempty"`
	RemoteSysName    *string `json:"remote_sys_name,omitempty"`
	NeighborDeviceID int64   `json:"neighbor_device_id"`
	NeighborName     string  `json:"neighbor_name"`
	NeighborHost     string  `json:"neighbor_host,omitempty"`
	InDatabase       bool    `json:"in_database"`
	OnPorts          bool    `json:"on_ports"`
	PathViaY         []int64 `json:"path_via_y,omitempty"`      // neighbor→root (кратчайший)
	AlternatePath    []int64 `json:"alternate_path,omitempty"` // neighbor→root в обход Y
}

// VLANBlastSkip — почему сосед/линк не дал warning.
type VLANBlastSkip struct {
	VLANID           int     `json:"vlan_id,omitempty"`
	IfIndex          int     `json:"if_index,omitempty"`
	IfName           string  `json:"if_name,omitempty"`
	Reason           string  `json:"reason"` // toward_core|vlan_not_on_link|neighbor_no_vlan|unresolved_neighbor|not_downward
	Protocol         string  `json:"protocol,omitempty"`
	RemoteSysName    *string `json:"remote_sys_name,omitempty"`
	NeighborDeviceID int64   `json:"neighbor_device_id,omitempty"`
}

// VLANBlastFDBClient — MAC в удаляемом VLAN на access / down-порту Y.
type VLANBlastFDBClient struct {
	MAC      string `json:"mac"`
	VLANID   int    `json:"vlan_id"`
	IfIndex  int    `json:"if_index"`
	IfName   string `json:"if_name,omitempty"`
	PortRole string `json:"port_role,omitempty"`
}

// VLANBlastMgmtRisk — удаляемый VLAN совпадает с SVI, чей IP = devices.host.
type VLANBlastMgmtRisk struct {
	VLANID int    `json:"vlan_id"`
	Host   string `json:"host"`
	SVIIP  string `json:"svi_ip"`
}

// VLANBlastGateway — роутер/L3 с SVI удаляемого VLAN (возможный шлюз сегмента).
type VLANBlastGateway struct {
	VLANID     int    `json:"vlan_id"`
	DeviceID   int64  `json:"device_id"`
	DeviceName string `json:"device_name"`
	Host       string `json:"host,omitempty"`
	SVIIP      string `json:"svi_ip"`
}

// VLANBlastResult — итог blast-radius (Phase 1–4 + NMS 0.8).
type VLANBlastResult struct {
	RootDeviceID      int64                 `json:"root_device_id,omitempty"`
	RootSource        string                `json:"root_source,omitempty"` // stp | heuristic
	Hits              []VLANBlastHit        `json:"hits"`
	Skips             []VLANBlastSkip       `json:"skips,omitempty"`
	TowardCoreSkipped int                   `json:"toward_core_skipped"`
	AffectedDeviceIDs []int64               `json:"affected_device_ids"`
	RedundantHitCount int                   `json:"redundant_hit_count"`
	FDBClients        []VLANBlastFDBClient  `json:"fdb_clients,omitempty"`
	MgmtRisks         []VLANBlastMgmtRisk   `json:"mgmt_risks,omitempty"`
	Gateways          []VLANBlastGateway    `json:"gateways,omitempty"`
	BlocksDelete      bool                  `json:"blocks_delete"` // true при mgmt SVI = host
	Severity          string                `json:"severity"`
	Warnings          []string              `json:"warnings"`
	Summary           string                `json:"summary"`
}

// LocalFDBHint — FDB-запись на Y для фильтра клиентов.
type LocalFDBHint struct {
	MAC     string
	IfIndex int
	VLANID  int
}

// TrunkPortMeta — локальный порт Y + опциональный override UPLINK/DOWNLINK из описания.
// Имя историческое: сюда попадают и trunk, и general с tagging (EdgeSwitch).
type TrunkPortMeta struct {
	Name     string
	Override string // uplink | downlink | ""
}

// BlastNeighborEdge — LLDP/CDP-сосед на порту Y.
type BlastNeighborEdge struct {
	IfIndex       int
	Protocol      string
	RemoteID      int64
	RemotePortID  *string
	RemoteSysName *string
}

// DeviceVLANInv — кэш inventory узла.
type DeviceVLANInv struct {
	Name string
	Host string
	Inv  []swcfg.VLANInventoryRow
}

// VLANBlastInput — всё нужное для анализа без SSH.
type VLANBlastInput struct {
	DeviceID     int64
	VLANIDs      []int
	RootID       int64
	RootSource   string // stp | heuristic | ""
	Dist         map[int64]int
	Adj          map[int64][]int64
	LocalInv     []swcfg.VLANInventoryRow
	TrunkPorts   map[int]TrunkPortMeta
	Neighbors    []BlastNeighborEdge
	LookupDevice func(deviceID int64) (DeviceVLANInv, bool)
	MaxDepth     int
	// Phase 2
	LocalPortRoles map[int]string // ifIndex → access|trunk|…
	LocalIfNames   map[int]string // ifIndex → name
	LocalFDB       []LocalFDBHint
	LocalPortModes map[string]swcfg.ParsedInterfaceCLI // show run — для allowed vlan all
	// Phase 3
	DeviceHost string
	LocalSVIs  []swcfg.SVIAddress
	Gateways   []VLANBlastGateway // заранее найденные роутеры с SVI удаляемых VLAN
	// NMS 0.8: VLAN-on-edge из topology blast cache (опционально).
	// EdgeCarriesVLAN(a,b,vid) — если задан и HasEdgeVLANs, используем вместо локального VLANCarriedOnPort∩peer.
	EdgeCarriesVLAN func(a, b int64, vlanID int) bool
	HasEdgeVLANs    bool
}

// PortDescrLinkOverride — ручной UPLINK/DOWNLINK в имени/описании (override, не основа модели).
func PortDescrLinkOverride(ifName string, ifDescr, cliDescr, descrOverride *string) string {
	parts := make([]string, 0, 4)
	if t := strings.TrimSpace(ifName); t != "" {
		parts = append(parts, t)
	}
	for _, p := range []*string{descrOverride, cliDescr, ifDescr} {
		if p == nil {
			continue
		}
		if t := strings.TrimSpace(*p); t != "" {
			parts = append(parts, t)
		}
	}
	joined := strings.ToLower(strings.Join(parts, " "))
	if strings.Contains(joined, "downlink") {
		return "downlink"
	}
	if strings.Contains(joined, "uplink") {
		return "uplink"
	}
	return ""
}

// OrientLink — направление к соседу относительно корня топологии.
// override: uplink|downlink сильнее эвристики.
func OrientLink(distY, distN int, yOK, nOK bool, override string) string {
	switch strings.ToLower(strings.TrimSpace(override)) {
	case "uplink", "up":
		return LinkDirOverrideUp
	case "downlink", "down":
		return LinkDirOverrideDown
	}
	if !yOK || !nOK {
		return LinkDirUnknown
	}
	if distN < distY {
		return LinkDirUp
	}
	if distN > distY {
		return LinkDirDown
	}
	return LinkDirPeer
}

// IsTowardCore — «вверх» к ядру: не предупреждаем о транзите вниз.
func IsTowardCore(dir string) bool {
	return dir == LinkDirUp || dir == LinkDirOverrideUp
}

// IsDownwardRisk — вниз / peer / unknown / явный DOWNLINK — кандидат на warning.
func IsDownwardRisk(dir string) bool {
	switch dir {
	case LinkDirDown, LinkDirPeer, LinkDirUnknown, LinkDirOverrideDown:
		return true
	default:
		return false
	}
}

// VLANOnLocalPort — VLAN X прописан на ifIndex в access/tagged inventory Y (не только FDB).
func VLANOnLocalPort(inv []swcfg.VLANInventoryRow, vlanID, ifIndex int) bool {
	return swcfg.InventoryVLANOnPort(inv, vlanID, ifIndex)
}

// MatchVLANInInventory — VLAN в database и/или на портах (не только FDB).
func MatchVLANInInventory(inv []swcfg.VLANInventoryRow, vlanID int) (ok bool, inDB, onPorts bool, name string) {
	for _, r := range inv {
		if r.VLANID != vlanID {
			continue
		}
		inDB = r.InDatabase
		onPorts = swcfg.VLANConfiguredOnPorts(r)
		if inDB || onPorts {
			return true, inDB, onPorts, strings.TrimSpace(r.Name)
		}
		return false, false, false, ""
	}
	return false, false, false, ""
}

// BuildLLDPDeviceAdj — undirected device adjacency из топологии (как L2-path).
func BuildLLDPDeviceAdj(g *store.TopologyGraph) (adj map[int64][]int64, names map[int64]string, linkCount map[int64]int) {
	adj = map[int64][]int64{}
	names = map[int64]string{}
	linkCount = map[int64]int{}
	if g == nil {
		return adj, names, linkCount
	}
	for _, n := range g.Nodes {
		names[n.ID] = n.Name
		if names[n.ID] == "" {
			names[n.ID] = n.Host
		}
		linkCount[n.ID] = n.LinkCount
	}
	seenEdge := map[string]struct{}{}
	add := func(a, b int64) {
		if a <= 0 || b <= 0 || a == b {
			return
		}
		lo, hi := a, b
		if lo > hi {
			lo, hi = hi, lo
		}
		key := fmt.Sprintf("%d:%d", lo, hi)
		if _, ok := seenEdge[key]; ok {
			return
		}
		seenEdge[key] = struct{}{}
		adj[a] = append(adj[a], b)
		adj[b] = append(adj[b], a)
	}
	for _, e := range g.Edges {
		if e.RemoteDeviceID == nil || *e.RemoteDeviceID <= 0 || e.Stale {
			continue
		}
		add(e.LocalDeviceID, *e.RemoteDeviceID)
	}
	return adj, names, linkCount
}

// PickTopologyRootID — обёртка над pickTopologyRoot для API.
func PickTopologyRootID(nodes []store.TopologyNode, linkCount map[int64]int) int64 {
	return pickTopologyRoot(nodes, linkCount)
}

// PickTopologyRootIDPrefer — STP-корень, если он есть в графе; иначе эвристика.
func PickTopologyRootIDPrefer(nodes []store.TopologyNode, linkCount map[int64]int, preferID int64) (rootID int64, source string) {
	if preferID > 0 {
		for _, n := range nodes {
			if n.ID == preferID && !n.Virtual {
				return preferID, "stp"
			}
		}
	}
	return pickTopologyRoot(nodes, linkCount), "heuristic"
}

// BridgeIDMACHex — последние 6 октетов Bridge ID (STP designated root) → hex MAC без разделителей.
func BridgeIDMACHex(bridgeID string) string {
	h := strings.ToLower(strings.TrimSpace(bridgeID))
	h = strings.ReplaceAll(h, ":", "")
	h = strings.ReplaceAll(h, "-", "")
	h = strings.ReplaceAll(h, " ", "")
	if len(h) >= 12 {
		return h[len(h)-12:]
	}
	return h
}

// PreferSTPRootDeviceID — устройство, чьё chassis_mac чаще всего встречается как STP designated root.
// chassisHex: нормализованный hex MAC (12 символов) → device_id.
func PreferSTPRootDeviceID(states []store.DeviceSTPState, chassisHex map[string]int64) int64 {
	if len(states) == 0 || len(chassisHex) == 0 {
		return 0
	}
	votes := map[int64]int{}
	for _, st := range states {
		if st.DesignatedRoot == nil {
			continue
		}
		mac := BridgeIDMACHex(*st.DesignatedRoot)
		if mac == "" {
			continue
		}
		if id, ok := chassisHex[mac]; ok && id > 0 {
			votes[id]++
		}
	}
	var best int64
	bestN := 0
	for id, n := range votes {
		if n > bestN || (n == bestN && (best == 0 || id < best)) {
			best = id
			bestN = n
		}
	}
	return best
}

// BFSDistances — расстояние от root по undirected adj; отсутствующий ключ = недостижим.
func BFSDistances(adj map[int64][]int64, root int64) map[int64]int {
	out := map[int64]int{}
	if root <= 0 {
		return out
	}
	out[root] = 0
	q := []int64{root}
	for len(q) > 0 {
		u := q[0]
		q = q[1:]
		for _, v := range adj[u] {
			if _, ok := out[v]; ok {
				continue
			}
			out[v] = out[u] + 1
			q = append(q, v)
		}
	}
	return out
}

// HasPathAvoiding — есть ли путь from→to по adj, не заходя в avoid.
func HasPathAvoiding(adj map[int64][]int64, from, to, avoid int64) bool {
	return len(ShortestPath(adj, from, to, avoid)) > 0
}

// ShortestPath — BFS путь from→to; avoid>0 исключается. Пустой slice = нет пути.
func ShortestPath(adj map[int64][]int64, from, to, avoid int64) []int64 {
	if from <= 0 || to <= 0 {
		return nil
	}
	if from == avoid || to == avoid {
		return nil
	}
	if from == to {
		return []int64{from}
	}
	parent := map[int64]int64{from: 0}
	seen := map[int64]bool{from: true}
	if avoid > 0 {
		seen[avoid] = true
	}
	q := []int64{from}
	found := false
	for len(q) > 0 && !found {
		u := q[0]
		q = q[1:]
		for _, v := range adj[u] {
			if seen[v] {
				continue
			}
			seen[v] = true
			parent[v] = u
			if v == to {
				found = true
				break
			}
			q = append(q, v)
		}
	}
	if !found {
		return nil
	}
	var rev []int64
	for cur := to; cur != 0; cur = parent[cur] {
		rev = append(rev, cur)
		if cur == from {
			break
		}
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// WalkDownWithVLAN — BFS от start до maxDepth, исключая exclude (обычно Y).
// Если dist задан — расширяем только к узлам дальше от root (dist больше), т.е. «вниз».
// Depth в результате — от start (1 = сосед start).
func WalkDownWithVLAN(adj map[int64][]int64, start, exclude int64, maxDepth int, dist map[int64]int, hasVLAN func(int64) bool) []struct {
	ID    int64
	Depth int
} {
	type item struct {
		id    int64
		depth int
	}
	var out []struct {
		ID    int64
		Depth int
	}
	if start <= 0 || maxDepth < 1 {
		return out
	}
	seen := map[int64]bool{start: true, exclude: true}
	q := []item{{id: start, depth: 0}}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if cur.id != start && hasVLAN(cur.id) {
			out = append(out, struct {
				ID    int64
				Depth int
			}{ID: cur.id, Depth: cur.depth})
		}
		if cur.depth >= maxDepth {
			continue
		}
		dCur, curOK := 0, false
		if dist != nil {
			dCur, curOK = dist[cur.id]
		}
		for _, nxt := range adj[cur.id] {
			if seen[nxt] {
				continue
			}
			if dist != nil && curOK {
				if dNxt, ok := dist[nxt]; ok && dNxt <= dCur {
					continue // к ядру или peer — не «вниз»
				}
			}
			seen[nxt] = true
			q = append(q, item{id: nxt, depth: cur.depth + 1})
		}
	}
	return out
}

// AnalyzeVLANDeleteBlast — blast-radius: топология + VLAN на линке + потомки + Phase2 (обход/FDB).
func AnalyzeVLANDeleteBlast(in VLANBlastInput) VLANBlastResult {
	maxDepth := in.MaxDepth
	if maxDepth < 0 {
		maxDepth = DefaultVLANBlastMaxDepth
	} else {
		maxDepth = EffectiveVLANBlastMaxDepth(maxDepth)
	}
	res := VLANBlastResult{
		RootDeviceID: in.RootID,
		RootSource:   in.RootSource,
		Hits:         make([]VLANBlastHit, 0),
		Skips:        make([]VLANBlastSkip, 0),
		Severity:     "info",
		Warnings:     []string{},
		FDBClients:   []VLANBlastFDBClient{},
		MgmtRisks:    []VLANBlastMgmtRisk{},
		Gateways:     []VLANBlastGateway{},
		BlocksDelete: false,
	}
	if in.LookupDevice == nil || len(in.VLANIDs) == 0 {
		res.Summary = vlanBlastEmptySummary(0)
		return res
	}

	distY, yOK := 0, false
	if in.Dist != nil {
		distY, yOK = in.Dist[in.DeviceID]
	}

	type invCache struct {
		info DeviceVLANInv
		ok   bool
		done bool
	}
	cache := map[int64]*invCache{}
	lookup := func(id int64) (DeviceVLANInv, bool) {
		c, ok := cache[id]
		if !ok {
			info, found := in.LookupDevice(id)
			c = &invCache{info: info, ok: found, done: true}
			cache[id] = c
		}
		return c.info, c.ok
	}
	hasVLAN := func(id int64, vid int) (bool, bool, bool, string) {
		info, ok := lookup(id)
		if !ok {
			return false, false, false, ""
		}
		return MatchVLANInInventory(info.Inv, vid)
	}

	seenHit := map[string]struct{}{}
	affected := map[int64]struct{}{}
	downIf := map[int]struct{}{} // локальные ifIndex с риск-хитами (для FDB)
	addSkip := func(sk VLANBlastSkip) {
		res.Skips = append(res.Skips, sk)
	}

	for _, n := range in.Neighbors {
		if n.RemoteID <= 0 || n.RemoteID == in.DeviceID {
			meta, ok := in.TrunkPorts[n.IfIndex]
			name := ""
			if ok {
				name = meta.Name
			}
			for _, vid := range in.VLANIDs {
				addSkip(VLANBlastSkip{
					VLANID: vid, IfIndex: n.IfIndex, IfName: name,
					Reason: "unresolved_neighbor", Protocol: n.Protocol, RemoteSysName: n.RemoteSysName,
				})
			}
			continue
		}
		meta, ok := in.TrunkPorts[n.IfIndex]
		if !ok {
			meta = TrunkPortMeta{Name: fmt.Sprintf("ifIndex %d", n.IfIndex)}
		}

		distN, nOK := 0, false
		if in.Dist != nil {
			distN, nOK = in.Dist[n.RemoteID]
		}
		dir := OrientLink(distY, distN, yOK, nOK, meta.Override)

		for _, vid := range in.VLANIDs {
			onLink := false
			if in.HasEdgeVLANs && in.EdgeCarriesVLAN != nil {
				onLink = in.EdgeCarriesVLAN(in.DeviceID, n.RemoteID, vid)
			} else {
				onLink = swcfg.VLANCarriedOnPort(in.LocalInv, vid, n.IfIndex, meta.Name, in.LocalPortModes)
			}
			if !onLink {
				addSkip(VLANBlastSkip{
					VLANID: vid, IfIndex: n.IfIndex, IfName: meta.Name,
					Reason: "vlan_not_on_link", Protocol: n.Protocol,
					RemoteSysName: n.RemoteSysName, NeighborDeviceID: n.RemoteID,
				})
				continue
			}
			okMatch, inDB, onPorts, vname := hasVLAN(n.RemoteID, vid)
			if !okMatch {
				addSkip(VLANBlastSkip{
					VLANID: vid, IfIndex: n.IfIndex, IfName: meta.Name,
					Reason: "neighbor_no_vlan", Protocol: n.Protocol,
					RemoteSysName: n.RemoteSysName, NeighborDeviceID: n.RemoteID,
				})
				continue
			}
			dedupe := fmt.Sprintf("%d:%d:%d:%s", vid, n.IfIndex, n.RemoteID, n.Protocol)
			if _, dup := seenHit[dedupe]; dup {
				continue
			}
			seenHit[dedupe] = struct{}{}

			if IsTowardCore(dir) {
				res.TowardCoreSkipped++
				addSkip(VLANBlastSkip{
					VLANID: vid, IfIndex: n.IfIndex, IfName: meta.Name,
					Reason: "toward_core", Protocol: n.Protocol,
					RemoteSysName: n.RemoteSysName, NeighborDeviceID: n.RemoteID,
				})
				continue
			}
			if !IsDownwardRisk(dir) {
				addSkip(VLANBlastSkip{
					VLANID: vid, IfIndex: n.IfIndex, IfName: meta.Name,
					Reason: "not_downward", Protocol: n.Protocol,
					RemoteSysName: n.RemoteSysName, NeighborDeviceID: n.RemoteID,
				})
				continue
			}

			redundant := false
			var pathVia, altPath []int64
			if in.RootID > 0 && in.Adj != nil {
				pathVia = ShortestPath(in.Adj, n.RemoteID, in.RootID, 0)
				redundant = HasPathAvoiding(in.Adj, n.RemoteID, in.RootID, in.DeviceID)
				if redundant {
					altPath = ShortestPath(in.Adj, n.RemoteID, in.RootID, in.DeviceID)
				}
			}

			info, _ := lookup(n.RemoteID)
			hit := VLANBlastHit{
				VLANID:           vid,
				VLANName:         vname,
				LocalIfIndex:     n.IfIndex,
				LocalIfName:      meta.Name,
				LinkDirection:    dir,
				Reason:           "neighbor_has_vlan",
				Hop:              1,
				Redundant:        redundant,
				Protocol:         n.Protocol,
				RemotePortID:     n.RemotePortID,
				RemoteSysName:    n.RemoteSysName,
				NeighborDeviceID: n.RemoteID,
				NeighborName:     info.Name,
				NeighborHost:     info.Host,
				InDatabase:       inDB,
				OnPorts:          onPorts,
				PathViaY:         pathVia,
				AlternatePath:    altPath,
			}
			res.Hits = append(res.Hits, hit)
			affected[n.RemoteID] = struct{}{}
			downIf[n.IfIndex] = struct{}{}

			desc := WalkDownWithVLAN(in.Adj, n.RemoteID, in.DeviceID, maxDepth, in.Dist, func(id int64) bool {
				ok, _, _, _ := hasVLAN(id, vid)
				return ok
			})
			for _, d := range desc {
				dd := fmt.Sprintf("%d:desc:%d:%d", vid, n.IfIndex, d.ID)
				if _, dup := seenHit[dd]; dup {
					continue
				}
				seenHit[dd] = struct{}{}
				dinfo, _ := lookup(d.ID)
				dok, dinDB, donPorts, dvname := hasVLAN(d.ID, vid)
				if !dok {
					continue
				}
				hop := 1 + d.Depth
				if hop < 2 {
					hop = 2
				}
				dRedundant := redundant
				var dPath, dAlt []int64
				if in.RootID > 0 && in.Adj != nil {
					dPath = ShortestPath(in.Adj, d.ID, in.RootID, 0)
					if !dRedundant {
						dRedundant = HasPathAvoiding(in.Adj, d.ID, in.RootID, in.DeviceID)
					}
					if dRedundant {
						dAlt = ShortestPath(in.Adj, d.ID, in.RootID, in.DeviceID)
					}
				}
				res.Hits = append(res.Hits, VLANBlastHit{
					VLANID:           vid,
					VLANName:         dvname,
					LocalIfIndex:     n.IfIndex,
					LocalIfName:      meta.Name,
					LinkDirection:    dir,
					Reason:           "descendant",
					Hop:              hop,
					Redundant:        dRedundant,
					Protocol:         n.Protocol,
					NeighborDeviceID: d.ID,
					NeighborName:     dinfo.Name,
					NeighborHost:     dinfo.Host,
					InDatabase:       dinDB,
					OnPorts:          donPorts,
					PathViaY:         dPath,
					AlternatePath:    dAlt,
				})
				affected[d.ID] = struct{}{}
			}
		}
	}

	sort.Slice(res.Hits, func(i, j int) bool {
		if res.Hits[i].VLANID != res.Hits[j].VLANID {
			return res.Hits[i].VLANID < res.Hits[j].VLANID
		}
		if res.Hits[i].Redundant != res.Hits[j].Redundant {
			return !res.Hits[i].Redundant && res.Hits[j].Redundant // critical first
		}
		if res.Hits[i].Hop != res.Hits[j].Hop {
			return res.Hits[i].Hop < res.Hits[j].Hop
		}
		if res.Hits[i].NeighborName != res.Hits[j].NeighborName {
			return res.Hits[i].NeighborName < res.Hits[j].NeighborName
		}
		return res.Hits[i].LocalIfIndex < res.Hits[j].LocalIfIndex
	})

	res.AffectedDeviceIDs = make([]int64, 0, len(affected))
	for id := range affected {
		res.AffectedDeviceIDs = append(res.AffectedDeviceIDs, id)
	}
	sort.Slice(res.AffectedDeviceIDs, func(i, j int) bool { return res.AffectedDeviceIDs[i] < res.AffectedDeviceIDs[j] })

	for _, h := range res.Hits {
		if h.Redundant {
			res.RedundantHitCount++
		}
	}

	res.FDBClients = collectVLANBlastFDBClients(in, downIf)

	// Phase 3: mgmt SVI + gateways
	wantVLAN := map[int]struct{}{}
	for _, id := range in.VLANIDs {
		wantVLAN[id] = struct{}{}
	}
	if vid, sip, ok := swcfg.MgmtVLANFromSVI(in.DeviceHost, in.LocalSVIs); ok {
		if _, del := wantVLAN[vid]; del {
			res.MgmtRisks = append(res.MgmtRisks, VLANBlastMgmtRisk{
				VLANID: vid,
				Host:   strings.TrimSpace(in.DeviceHost),
				SVIIP:  sip,
			})
		}
	}
	for _, g := range in.Gateways {
		if _, ok := wantVLAN[g.VLANID]; !ok {
			continue
		}
		if g.DeviceID == in.DeviceID {
			continue // локальный SVI уже в mgmt или просто L3 на Y
		}
		res.Gateways = append(res.Gateways, g)
	}
	sort.Slice(res.Gateways, func(i, j int) bool {
		if res.Gateways[i].VLANID != res.Gateways[j].VLANID {
			return res.Gateways[i].VLANID < res.Gateways[j].VLANID
		}
		return res.Gateways[i].DeviceID < res.Gateways[j].DeviceID
	})

	criticalHits := 0
	for _, h := range res.Hits {
		if !h.Redundant {
			criticalHits++
		}
	}
	switch {
	case len(res.MgmtRisks) > 0:
		res.Severity = "critical"
	case len(res.Gateways) > 0 && criticalHits > 0:
		res.Severity = "warning"
	case len(res.Gateways) > 0:
		res.Severity = "warning"
	case criticalHits > 0:
		res.Severity = "warning"
	case len(res.Hits) > 0:
		res.Severity = "info"
	case len(res.FDBClients) > 0:
		res.Severity = "info"
	default:
		res.Severity = "info"
	}

	res.Warnings = VLANBlastWarnings(res.Hits)
	for _, m := range res.MgmtRisks {
		res.Warnings = append([]string{fmt.Sprintf(
			"КРИТИЧНО: VLAN %d — management SVI этого свитча (host %s = %s). Удаление может оборвать доступ к устройству.",
			m.VLANID, m.Host, m.SVIIP,
		)}, res.Warnings...)
	}
	for _, g := range res.Gateways {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"Шлюз/L3: VLAN %d имеет SVI %s на «%s» (id=%d) — сегмент может потерять маршрутизацию через этот свитч.",
			g.VLANID, g.SVIIP, g.DeviceName, g.DeviceID,
		))
	}
	if len(res.FDBClients) > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"На этом свитче в FDB видно %d MAC в удаляемых VLAN (access / down-порты).",
			len(res.FDBClients),
		))
	}
	res.BlocksDelete = len(res.MgmtRisks) > 0
	// ограничим skips в ответе
	const maxSkips = 40
	if len(res.Skips) > maxSkips {
		res.Skips = res.Skips[:maxSkips]
	}
	res.Summary = VLANBlastSummary(res.Hits, res.TowardCoreSkipped, len(res.AffectedDeviceIDs), res.RedundantHitCount, len(res.FDBClients), len(res.MgmtRisks), len(res.Gateways))
	return res
}

func collectVLANBlastFDBClients(in VLANBlastInput, downIf map[int]struct{}) []VLANBlastFDBClient {
	if len(in.LocalFDB) == 0 || len(in.VLANIDs) == 0 {
		return nil
	}
	wantVLAN := map[int]struct{}{}
	for _, id := range in.VLANIDs {
		wantVLAN[id] = struct{}{}
	}
	out := make([]VLANBlastFDBClient, 0)
	seen := map[string]struct{}{}
	for _, e := range in.LocalFDB {
		if _, ok := wantVLAN[e.VLANID]; !ok {
			continue
		}
		role := ""
		if in.LocalPortRoles != nil {
			role = strings.ToLower(strings.TrimSpace(in.LocalPortRoles[e.IfIndex]))
		}
		_, isDown := downIf[e.IfIndex]
		if role != "access" && !isDown {
			continue
		}
		key := fmt.Sprintf("%s:%d:%d", e.MAC, e.IfIndex, e.VLANID)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		name := ""
		if in.LocalIfNames != nil {
			name = in.LocalIfNames[e.IfIndex]
		}
		out = append(out, VLANBlastFDBClient{
			MAC:      e.MAC,
			VLANID:   e.VLANID,
			IfIndex:  e.IfIndex,
			IfName:   name,
			PortRole: role,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].VLANID != out[j].VLANID {
			return out[i].VLANID < out[j].VLANID
		}
		if out[i].IfIndex != out[j].IfIndex {
			return out[i].IfIndex < out[j].IfIndex
		}
		return out[i].MAC < out[j].MAC
	})
	const maxClients = 50
	if len(out) > maxClients {
		out = out[:maxClients]
	}
	return out
}

func vlanBlastEmptySummary(towardCore int) string {
	if towardCore > 0 {
		return "Риск вниз не найден: удаляемые VLAN есть только у соседей к ядру (вверх), не у нижестоящих."
	}
	return "Риск вниз не найден: у LLDP/CDP-соседей ниже по топологии этих VLAN нет (или VLAN не идёт по trunk к ним)."
}

// formatNamedList — «A, B, C» или «A, B, C… (+N)».
func formatNamedList(names []string, maxShow int) string {
	if maxShow < 1 {
		maxShow = 12
	}
	if len(names) == 0 {
		return ""
	}
	if len(names) <= maxShow {
		return strings.Join(names, ", ")
	}
	shown := names[:maxShow]
	return fmt.Sprintf("%s… (+%d)", strings.Join(shown, ", "), len(names)-maxShow)
}

func uniqueHitDeviceNames(hits []VLANBlastHit) []string {
	seen := map[int64]struct{}{}
	names := make([]string, 0)
	for _, h := range hits {
		id := h.NeighborDeviceID
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		nm := strings.TrimSpace(h.NeighborName)
		if nm == "" {
			nm = fmt.Sprintf("#%d", id)
		}
		names = append(names, nm)
	}
	sort.Strings(names)
	return names
}

func formatVLANIDList(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ", ")
}

// VLANBlastSummary — человекочитаемое резюме с именами свитчей (с любого узла, не только root).
func VLANBlastSummary(hits []VLANBlastHit, towardCoreSkipped, affectedCount, redundantCount, fdbCount, mgmtCount, gwCount int) string {
	if len(hits) == 0 && mgmtCount == 0 && gwCount == 0 {
		s := vlanBlastEmptySummary(towardCoreSkipped)
		if fdbCount > 0 {
			s += fmt.Sprintf(" На этом свитче в FDB: %d MAC в удаляемых VLAN.", fdbCount)
		}
		return s
	}

	vlans := map[int]struct{}{}
	critical := 0
	for _, h := range hits {
		vlans[h.VLANID] = struct{}{}
		if !h.Redundant {
			critical++
		}
	}
	ids := make([]int, 0, len(vlans))
	for id := range vlans {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	vlanCSV := formatVLANIDList(ids)
	switchNames := uniqueHitDeviceNames(hits)
	nameList := formatNamedList(switchNames, 16)

	if mgmtCount > 0 {
		msg := fmt.Sprintf("Критично: удаляется management VLAN этого свитча (SVI = host).")
		if nameList != "" {
			msg += fmt.Sprintf(" Также от этих VLAN могут отключиться свичи: %s.", nameList)
		} else if affectedCount > 0 {
			msg += fmt.Sprintf(" Затронуто нижестоящих: %d.", affectedCount)
		}
		if gwCount > 0 {
			msg += fmt.Sprintf(" Шлюзов/L3 с SVI: %d.", gwCount)
		}
		return msg
	}

	if vlanCSV == "" && gwCount > 0 {
		return fmt.Sprintf("Удаляемые VLAN имеют SVI на L3/роутере (шлюзов: %d) — риск маршрутизации сегмента.", gwCount)
	}

	var msg string
	if nameList != "" {
		if redundantCount > 0 && critical == 0 {
			msg = fmt.Sprintf(
				"Удалив VLAN %s с этого свитча, вы затронете свичи %s — но у всех найден обход к ядру (риск снижен).",
				vlanCSV, nameList,
			)
		} else {
			msg = fmt.Sprintf(
				"Удалив VLAN %s с этого свитча, вы отключите от этих VLAN свичи: %s.",
				vlanCSV, nameList,
			)
			if redundantCount > 0 {
				msg += fmt.Sprintf(" У части (%d) есть обход к ядру.", redundantCount)
			}
		}
	} else if vlanCSV != "" {
		msg = fmt.Sprintf("Удаление VLAN %s может затронуть нижестоящие устройства (%d).", vlanCSV, affectedCount)
	} else {
		msg = "Удаление VLAN может затронуть нижестоящие устройства."
	}
	if gwCount > 0 {
		msg += fmt.Sprintf(" Шлюзов/L3 с SVI: %d.", gwCount)
	}
	if fdbCount > 0 {
		msg += fmt.Sprintf(" FDB на этом свитче: %d MAC.", fdbCount)
	}
	return msg
}

// VLANBlastWarnings — строки для confirm UI.
func VLANBlastWarnings(hits []VLANBlastHit) []string {
	if len(hits) == 0 {
		return nil
	}
	type key struct {
		vid  int
		nid  int64
		name string
	}
	grouped := map[key][]string{}
	order := make([]key, 0)
	for _, h := range hits {
		k := key{vid: h.VLANID, nid: h.NeighborDeviceID, name: h.NeighborName}
		if _, ok := grouped[k]; !ok {
			order = append(order, k)
		}
		port := h.LocalIfName
		if port == "" {
			port = fmt.Sprintf("ifIndex %d", h.LocalIfIndex)
		}
		dirLabel := h.LinkDirection
		if dirLabel != "" {
			port = port + " [" + dirLabel + "]"
		}
		what := "в vlan database"
		if h.InDatabase && h.OnPorts {
			what = "в vlan database и на портах"
		} else if h.OnPorts {
			what = "на портах"
		}
		via := "сосед"
		if h.Reason == "descendant" {
			via = fmt.Sprintf("нижестоящий hop=%d", h.Hop)
		}
		extra := ""
		if h.Redundant {
			extra = ", есть обход к ядру"
		}
		line := fmt.Sprintf("%s → %s (%s, %s%s)", port, h.NeighborName, via, what, extra)
		grouped[k] = append(grouped[k], line)
	}
	out := make([]string, 0, len(order))
	for _, k := range order {
		out = append(out, fmt.Sprintf("VLAN %d у «%s»: %s", k.vid, k.name, strings.Join(grouped[k], "; ")))
	}
	return out
}
