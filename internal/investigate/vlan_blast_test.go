package investigate

import (
	"strings"
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

func TestVLANBlastSummaryNames(t *testing.T) {
	hits := []VLANBlastHit{
		{VLANID: 12, NeighborDeviceID: 10, NeighborName: "ES48 #11", Hop: 1},
		{VLANID: 12, NeighborDeviceID: 11, NeighborName: "16 #5", Hop: 2},
		{VLANID: 30, NeighborDeviceID: 10, NeighborName: "ES48 #11", Hop: 1},
	}
	s := VLANBlastSummary(hits, 0, 2, 0, 0, 0, 0)
	if !strings.Contains(s, "Удалив VLAN") || !strings.Contains(s, "12") || !strings.Contains(s, "30") {
		t.Fatalf("vlan list: %s", s)
	}
	if !strings.Contains(s, "ES48 #11") || !strings.Contains(s, "16 #5") {
		t.Fatalf("switch names: %s", s)
	}
	if !strings.Contains(s, "отключите от этих VLAN свичи") {
		t.Fatalf("phrase: %s", s)
	}
}

func TestOrientLink(t *testing.T) {
	// root=0: Y dist 2, parent dist 1 → up; child dist 3 → down; peer same → peer
	if got := OrientLink(2, 1, true, true, ""); got != LinkDirUp {
		t.Fatalf("parent: got %q", got)
	}
	if got := OrientLink(2, 3, true, true, ""); got != LinkDirDown {
		t.Fatalf("child: got %q", got)
	}
	if got := OrientLink(2, 2, true, true, ""); got != LinkDirPeer {
		t.Fatalf("peer: got %q", got)
	}
	if got := OrientLink(2, 0, true, false, ""); got != LinkDirUnknown {
		t.Fatalf("unreachable neighbor: got %q", got)
	}
	if got := OrientLink(2, 3, true, true, "uplink"); got != LinkDirOverrideUp {
		t.Fatalf("override up: got %q", got)
	}
	if got := OrientLink(2, 1, true, true, "downlink"); got != LinkDirOverrideDown {
		t.Fatalf("override down: got %q", got)
	}
}

func TestPortDescrLinkOverride(t *testing.T) {
	up := "port 5:  UPLINK - ES 16 XG#1 port 16"
	if got := PortDescrLinkOverride("0/5", &up, nil, nil); got != "uplink" {
		t.Fatalf("got %q", got)
	}
	down := "DOWNLINK to edge"
	if got := PortDescrLinkOverride("0/12", &down, nil, nil); got != "downlink" {
		t.Fatalf("got %q", got)
	}
	plain := "TO-SERVERROOM"
	if got := PortDescrLinkOverride("0/17", &plain, nil, nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestVLANOnLocalPort(t *testing.T) {
	inv := []swcfg.VLANInventoryRow{
		{VLANID: 14, InDatabase: true, TaggedPorts: []swcfg.VLANPortRef{{IfIndex: 5, IfName: "0/5"}}},
		{VLANID: 20, InDatabase: true, AccessPorts: []swcfg.VLANPortRef{{IfIndex: 8}}},
	}
	if !VLANOnLocalPort(inv, 14, 5) {
		t.Fatal("tagged 5")
	}
	if VLANOnLocalPort(inv, 14, 8) {
		t.Fatal("14 not on 8")
	}
	if !VLANOnLocalPort(inv, 20, 8) {
		t.Fatal("access 8")
	}
	if VLANOnLocalPort(inv, 99, 5) {
		t.Fatal("missing vlan")
	}
}

func TestAnalyzeVLANDeleteBlast_OrientAndOnLink(t *testing.T) {
	// root=1 — core; Y=11 dist 2; uplink neigh=10 dist 1; down neigh=12 dist 3; leaf=13 dist 4
	adj := map[int64][]int64{
		1:  {10},
		10: {1, 11},
		11: {10, 12},
		12: {11, 13},
		13: {12},
	}
	dist := BFSDistances(adj, 1)
	if dist[11] != 2 || dist[10] != 1 || dist[12] != 3 {
		t.Fatalf("dist: %+v", dist)
	}

	localInv := []swcfg.VLANInventoryRow{
		{VLANID: 50, InDatabase: true, TaggedPorts: []swcfg.VLANPortRef{
			{IfIndex: 5, IfName: "0/5"}, // toward core
			{IfIndex: 7, IfName: "0/7"}, // toward down
		}},
	}
	devices := map[int64]DeviceVLANInv{
		10: {Name: "XG#1", Inv: []swcfg.VLANInventoryRow{{VLANID: 50, InDatabase: true}}},
		12: {Name: "Edge#12", Inv: []swcfg.VLANInventoryRow{{VLANID: 50, InDatabase: true}}},
		13: {Name: "Leaf#13", Inv: []swcfg.VLANInventoryRow{{VLANID: 50, InDatabase: true}}},
	}

	in := VLANBlastInput{
		DeviceID: 11,
		VLANIDs:  []int{50},
		RootID:   1,
		Dist:     dist,
		Adj:      adj,
		LocalInv: localInv,
		TrunkPorts: map[int]TrunkPortMeta{
			5: {Name: "0/5"},
			7: {Name: "0/7"},
		},
		Neighbors: []BlastNeighborEdge{
			{IfIndex: 5, Protocol: "lldp", RemoteID: 10},
			{IfIndex: 7, Protocol: "lldp", RemoteID: 12},
		},
		LookupDevice: func(id int64) (DeviceVLANInv, bool) {
			d, ok := devices[id]
			return d, ok
		},
		MaxDepth: 4,
	}

	res := AnalyzeVLANDeleteBlast(in)
	if res.TowardCoreSkipped != 1 {
		t.Fatalf("toward core skipped want 1 got %d hits=%+v", res.TowardCoreSkipped, res.Hits)
	}
	if len(res.Hits) < 1 {
		t.Fatal("expected down hits")
	}
	found12, found13 := false, false
	for _, h := range res.Hits {
		if h.NeighborDeviceID == 10 {
			t.Fatal("must not warn on up neighbor")
		}
		if h.NeighborDeviceID == 12 {
			found12 = true
			if h.LinkDirection != LinkDirDown {
				t.Fatalf("dir 12: %s", h.LinkDirection)
			}
		}
		if h.NeighborDeviceID == 13 {
			found13 = true
			if h.Reason != "descendant" {
				t.Fatalf("13 reason: %s", h.Reason)
			}
		}
	}
	if !found12 {
		t.Fatal("missing direct down neighbor 12")
	}
	if !found13 {
		t.Fatal("missing descendant 13")
	}
	if res.Severity != "warning" {
		t.Fatalf("severity %s", res.Severity)
	}
	if !strings.Contains(res.Summary, "VLAN 50") {
		t.Fatalf("summary: %s", res.Summary)
	}
}

func TestAnalyzeVLANDeleteBlast_NoVLANOnLink(t *testing.T) {
	adj := map[int64][]int64{1: {2}, 2: {1}}
	dist := BFSDistances(adj, 1)
	in := VLANBlastInput{
		DeviceID: 1,
		VLANIDs:  []int{50},
		RootID:   1,
		Dist:     dist,
		Adj:      adj,
		LocalInv: []swcfg.VLANInventoryRow{
			{VLANID: 50, InDatabase: true}, // в db, но не на портах
		},
		TrunkPorts: map[int]TrunkPortMeta{9: {Name: "0/9"}},
		Neighbors:  []BlastNeighborEdge{{IfIndex: 9, Protocol: "lldp", RemoteID: 2}},
		LookupDevice: func(id int64) (DeviceVLANInv, bool) {
			return DeviceVLANInv{Name: "peer", Inv: []swcfg.VLANInventoryRow{{VLANID: 50, InDatabase: true}}}, true
		},
	}
	res := AnalyzeVLANDeleteBlast(in)
	if len(res.Hits) != 0 {
		t.Fatalf("no on-link membership → no hits, got %+v", res.Hits)
	}
}

func TestAnalyzeVLANDeleteBlast_OverrideUplink(t *testing.T) {
	// Topology would say down, but override uplink skips.
	adj := map[int64][]int64{1: {2}, 2: {1, 3}, 3: {2}}
	dist := BFSDistances(adj, 1)
	in := VLANBlastInput{
		DeviceID: 2,
		VLANIDs:  []int{7},
		RootID:   1,
		Dist:     dist,
		Adj:      adj,
		LocalInv: []swcfg.VLANInventoryRow{
			{VLANID: 7, TaggedPorts: []swcfg.VLANPortRef{{IfIndex: 1}}},
		},
		TrunkPorts: map[int]TrunkPortMeta{1: {Name: "0/1", Override: "uplink"}},
		Neighbors:  []BlastNeighborEdge{{IfIndex: 1, Protocol: "lldp", RemoteID: 3}},
		LookupDevice: func(id int64) (DeviceVLANInv, bool) {
			return DeviceVLANInv{Name: "child", Inv: []swcfg.VLANInventoryRow{{VLANID: 7, InDatabase: true}}}, true
		},
	}
	res := AnalyzeVLANDeleteBlast(in)
	if res.TowardCoreSkipped != 1 || len(res.Hits) != 0 {
		t.Fatalf("override uplink: skipped=%d hits=%+v", res.TowardCoreSkipped, res.Hits)
	}
}

func TestHasPathAvoiding(t *testing.T) {
	adj := map[int64][]int64{
		1: {2, 3},
		2: {1, 4},
		3: {1, 4},
		4: {2, 3},
	}
	if !HasPathAvoiding(adj, 4, 1, 2) {
		t.Fatal("expected alternate path 4-3-1")
	}
	line := map[int64][]int64{1: {2}, 2: {1, 3}, 3: {2}}
	if HasPathAvoiding(line, 3, 1, 2) {
		t.Fatal("no path avoiding 2")
	}
}

func TestAnalyzeVLANDeleteBlast_Redundant(t *testing.T) {
	adj := map[int64][]int64{
		1: {2, 3},
		2: {1, 4},
		3: {1, 4},
		4: {2, 3},
	}
	dist := BFSDistances(adj, 1)
	devices := map[int64]DeviceVLANInv{
		4: {Name: "leaf", Inv: []swcfg.VLANInventoryRow{{VLANID: 9, InDatabase: true}}},
	}
	in := VLANBlastInput{
		DeviceID: 2,
		VLANIDs:  []int{9},
		RootID:   1,
		Dist:     dist,
		Adj:      adj,
		LocalInv: []swcfg.VLANInventoryRow{
			{VLANID: 9, TaggedPorts: []swcfg.VLANPortRef{{IfIndex: 10}}},
		},
		TrunkPorts: map[int]TrunkPortMeta{10: {Name: "0/10"}},
		Neighbors:  []BlastNeighborEdge{{IfIndex: 10, Protocol: "lldp", RemoteID: 4}},
		LookupDevice: func(id int64) (DeviceVLANInv, bool) {
			d, ok := devices[id]
			return d, ok
		},
	}
	res := AnalyzeVLANDeleteBlast(in)
	if len(res.Hits) != 1 || !res.Hits[0].Redundant {
		t.Fatalf("want redundant hit: %+v", res.Hits)
	}
	if res.Severity != "info" {
		t.Fatalf("all redundant → info, got %s", res.Severity)
	}
	if !strings.Contains(res.Summary, "обход") {
		t.Fatalf("summary: %s", res.Summary)
	}
}

func TestAnalyzeVLANDeleteBlast_FDBClients(t *testing.T) {
	adj := map[int64][]int64{1: {2}, 2: {1}}
	dist := BFSDistances(adj, 1)
	in := VLANBlastInput{
		DeviceID:   2,
		VLANIDs:    []int{40},
		RootID:     1,
		Dist:       dist,
		Adj:        adj,
		LocalInv:   []swcfg.VLANInventoryRow{{VLANID: 40, InDatabase: true}},
		TrunkPorts: map[int]TrunkPortMeta{},
		Neighbors:  nil,
		LookupDevice: func(id int64) (DeviceVLANInv, bool) {
			return DeviceVLANInv{}, false
		},
		LocalPortRoles: map[int]string{3: "access", 4: "trunk"},
		LocalIfNames:   map[int]string{3: "0/3", 4: "0/4"},
		LocalFDB: []LocalFDBHint{
			{MAC: "aa:bb:cc:dd:ee:01", IfIndex: 3, VLANID: 40},
			{MAC: "aa:bb:cc:dd:ee:02", IfIndex: 4, VLANID: 40},
		},
	}
	res := AnalyzeVLANDeleteBlast(in)
	if len(res.FDBClients) != 1 || res.FDBClients[0].IfIndex != 3 {
		t.Fatalf("want access FDB only: %+v", res.FDBClients)
	}
}

func TestAnalyzeVLANDeleteBlast_GeneralTaggedDownlink(t *testing.T) {
	// EdgeSwitch: mode general + tagging — ResolveInterfacePortRole = access, раньше отбрасывали.
	adj := map[int64][]int64{1: {2}, 2: {1, 3}, 3: {2}}
	dist := BFSDistances(adj, 1)
	modes := swcfg.ParseRunningConfigPortModes(`
interface 0/14
 description DOWNLINK to edge
 switchport mode general
 vlan participation include 1,17
 vlan tagging 17
!
`)
	devices := map[int64]DeviceVLANInv{
		3: {Name: "edge-down", Inv: []swcfg.VLANInventoryRow{
			{VLANID: 17, InDatabase: true, AccessPorts: []swcfg.VLANPortRef{{IfIndex: 1}}},
		}},
	}
	in := VLANBlastInput{
		DeviceID: 2,
		VLANIDs:  []int{17},
		RootID:   1,
		Dist:     dist,
		Adj:      adj,
		LocalInv: []swcfg.VLANInventoryRow{{VLANID: 17, InDatabase: true}},
		TrunkPorts: map[int]TrunkPortMeta{
			14: {Name: "0/14", Override: "downlink"},
		},
		Neighbors: []BlastNeighborEdge{{IfIndex: 14, Protocol: "lldp", RemoteID: 3}},
		LookupDevice: func(id int64) (DeviceVLANInv, bool) {
			d, ok := devices[id]
			return d, ok
		},
		LocalPortModes: modes,
	}
	res := AnalyzeVLANDeleteBlast(in)
	if len(res.Hits) < 1 {
		t.Fatalf("want hit: %+v summary=%s", res.Hits, res.Summary)
	}
	if res.Hits[0].LinkDirection != LinkDirOverrideDown {
		t.Fatalf("dir: %s", res.Hits[0].LinkDirection)
	}
}

func TestAnalyzeVLANDeleteBlast_AllowedAllTrunk(t *testing.T) {
	// trunk allowed all — VLAN не в TaggedPorts inventory, но должен считаться на линке
	adj := map[int64][]int64{1: {2}, 2: {1, 3}, 3: {2}}
	dist := BFSDistances(adj, 1)
	modes := swcfg.ParseRunningConfigPortModes(`
interface 0/8
 switchport mode trunk
 switchport trunk allowed vlan all
!
`)
	devices := map[int64]DeviceVLANInv{
		3: {Name: "down-sw", Inv: []swcfg.VLANInventoryRow{
			{VLANID: 17, InDatabase: true, AccessPorts: []swcfg.VLANPortRef{{IfIndex: 2, IfName: "0/2"}}},
		}},
	}
	in := VLANBlastInput{
		DeviceID: 2,
		VLANIDs:  []int{17},
		RootID:   1,
		Dist:     dist,
		Adj:      adj,
		LocalInv: []swcfg.VLANInventoryRow{{VLANID: 17, InDatabase: true}},
		TrunkPorts: map[int]TrunkPortMeta{8: {Name: "0/8"}},
		Neighbors:  []BlastNeighborEdge{{IfIndex: 8, Protocol: "lldp", RemoteID: 3}},
		LookupDevice: func(id int64) (DeviceVLANInv, bool) {
			d, ok := devices[id]
			return d, ok
		},
		LocalPortModes: modes,
	}
	res := AnalyzeVLANDeleteBlast(in)
	if len(res.Hits) < 1 || res.Hits[0].NeighborDeviceID != 3 {
		t.Fatalf("want hit on down-sw: %+v", res.Hits)
	}
}

func TestAnalyzeVLANDeleteBlast_MgmtAndGateway(t *testing.T) {
	adj := map[int64][]int64{1: {2}, 2: {1}}
	dist := BFSDistances(adj, 1)
	in := VLANBlastInput{
		DeviceID:   2,
		VLANIDs:    []int{99},
		RootID:     1,
		Dist:       dist,
		Adj:        adj,
		LocalInv:   []swcfg.VLANInventoryRow{{VLANID: 99, InDatabase: true}},
		TrunkPorts: map[int]TrunkPortMeta{},
		Neighbors:  nil,
		LookupDevice: func(id int64) (DeviceVLANInv, bool) {
			return DeviceVLANInv{}, false
		},
		DeviceHost: "10.10.10.5",
		LocalSVIs:  []swcfg.SVIAddress{{VLANID: 99, IP: "10.10.10.5"}},
		Gateways: []VLANBlastGateway{
			{VLANID: 99, DeviceID: 1, DeviceName: "core-rtr", Host: "10.10.10.1", SVIIP: "10.10.10.1"},
		},
	}
	res := AnalyzeVLANDeleteBlast(in)
	if res.Severity != "critical" || len(res.MgmtRisks) != 1 {
		t.Fatalf("mgmt: sev=%s risks=%+v", res.Severity, res.MgmtRisks)
	}
	if len(res.Gateways) != 1 {
		t.Fatalf("gateways: %+v", res.Gateways)
	}
	if !strings.Contains(res.Summary, "management") && !strings.Contains(res.Summary, "Критично") {
		t.Fatalf("summary: %s", res.Summary)
	}
	if !res.BlocksDelete {
		t.Fatal("mgmt SVI must set BlocksDelete")
	}
}

func TestBridgeIDMACHexAndPreferSTP(t *testing.T) {
	if got := BridgeIDMACHex("8000aabbccddeeff"); got != "aabbccddeeff" {
		t.Fatalf("got %q", got)
	}
	root := "8000112233445566"
	states := []store.DeviceSTPState{
		{DeviceID: 10, DesignatedRoot: &root},
		{DeviceID: 11, DesignatedRoot: &root},
	}
	chassis := map[string]int64{"112233445566": 5}
	if PreferSTPRootDeviceID(states, chassis) != 5 {
		t.Fatal("prefer STP root device")
	}
	nodes := []store.TopologyNode{{ID: 5, Name: "core", LinkCount: 1}, {ID: 9, Name: "edge", LinkCount: 50}}
	lc := map[int64]int{5: 1, 9: 50}
	id, src := PickTopologyRootIDPrefer(nodes, lc, 5)
	if id != 5 || src != "stp" {
		t.Fatalf("prefer: id=%d src=%s", id, src)
	}
}
