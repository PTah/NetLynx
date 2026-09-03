package store

import (
	"testing"
	"time"
)

func TestFilterTopologyGraph(t *testing.T) {
	sys := "core-sw"
	g := &TopologyGraph{
		Nodes: []TopologyNode{
			{ID: 1, Name: "sw-a", Host: "10.0.0.1"},
			{ID: 2, Name: "sw-b", Host: "10.0.0.2", SysName: &sys},
			{ID: 3, Name: "edge", Host: "10.0.0.3"},
		},
		Edges: []TopologyEdge{
			{LocalDeviceID: 1, LocalIfIndex: 1, RemoteDeviceID: int64Ptr(2), Protocol: "lldp"},
			{LocalDeviceID: 2, LocalIfIndex: 5, RemoteDeviceID: int64Ptr(3), Protocol: "cdp", RemoteSysName: strPtr("edge")},
		},
	}
	out := FilterTopologyGraph(g, "core")
	if len(out.Nodes) < 1 {
		t.Fatal("expected matching nodes for sysName")
	}
	found := false
	for _, n := range out.Nodes {
		if n.ID == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected node 2 in filter result: %+v", out.Nodes)
	}

	out2 := FilterTopologyGraph(g, "cdp")
	if len(out2.Edges) != 1 || out2.Edges[0].Protocol != "cdp" {
		t.Fatalf("expected cdp edge, got %+v", out2.Edges)
	}
}

func TestDedupeTopologyEdges(t *testing.T) {
	now := time.Now().UTC()
	earlier := now.Add(-time.Hour)
	rid := int64(2)
	port := "Gi0/1"
	in := []TopologyEdge{
		{
			LocalDeviceID: 1, LocalIfIndex: 10, RemoteDeviceID: &rid, RemotePortID: &port,
			Protocol: "lldp", Protocols: []string{"lldp"}, RemIndex: 1, LastSeenAt: &earlier,
		},
		{
			LocalDeviceID: 1, LocalIfIndex: 10, RemoteDeviceID: &rid, RemotePortID: &port,
			Protocol: "cdp", Protocols: []string{"cdp"}, RemIndex: 1, Stale: true, LastSeenAt: &now,
		},
	}
	out := dedupeTopologyEdges(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(out))
	}
	if len(out[0].Protocols) != 2 {
		t.Fatalf("expected lldp+cdp, got %v", out[0].Protocols)
	}
	if out[0].Stale {
		t.Fatal("fresh LLDP must not be poisoned by stale CDP on the same port")
	}
	if out[0].LastSeenAt == nil || !out[0].LastSeenAt.Equal(now) {
		t.Fatal("expected max last_seen_at")
	}
}

func TestDedupeTopologyEdgesUndirected(t *testing.T) {
	a, b := int64(10), int64(20)
	pA, pB := "0/17", "0/9"
	nameA, nameB := "0/17", "0/9"
	in := []TopologyEdge{
		{
			LocalDeviceID: a, LocalIfIndex: 17, LocalIfName: &nameA,
			RemoteDeviceID: &b, RemotePortID: &pB,
			Protocol: "lldp", Protocols: []string{"lldp"},
		},
		{
			LocalDeviceID: b, LocalIfIndex: 9, LocalIfName: &nameB,
			RemoteDeviceID: &a, RemotePortID: &pA,
			Protocol: "lldp", Protocols: []string{"lldp"},
		},
	}
	out := dedupeTopologyEdges(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 undirected edge, got %d: %+v", len(out), out)
	}
	// Ориентация: меньший device id как local.
	if out[0].LocalDeviceID != a || *out[0].RemoteDeviceID != b {
		t.Fatalf("expected orientation %d→%d, got %d→%v", a, b, out[0].LocalDeviceID, out[0].RemoteDeviceID)
	}
}

func TestDedupeTopologyEdgesUndirectedMismatchedPorts(t *testing.T) {
	sw, mt := int64(41), int64(103)
	swName, mtName := "0/17", "ether1"
	mac := "dc:2c:6e:11:22:33"
	in := []TopologyEdge{
		{
			LocalDeviceID: sw, LocalIfIndex: 17, LocalIfName: &swName,
			RemoteDeviceID: &mt, RemotePortID: &mac,
			Protocol: "lldp", Protocols: []string{"lldp"},
		},
		{
			LocalDeviceID: mt, LocalIfIndex: 1, LocalIfName: &mtName,
			RemoteDeviceID: &sw, RemotePortID: &swName,
			Protocol: "lldp", Protocols: []string{"lldp"},
		},
	}
	out := dedupeTopologyEdges(in)
	if len(out) != 1 {
		t.Fatalf("expected 1 edge for switch↔mikrotik with mismatched port ids, got %d: %+v", len(out), out)
	}
}

func TestDedupeTopologyEdgesKeepsTwoCables(t *testing.T) {
	a, b := int64(1), int64(2)
	pA1, pA2, pB1, pB2 := "0/5", "0/7", "ether1", "ether2"
	in := []TopologyEdge{
		{LocalDeviceID: a, LocalIfIndex: 5, LocalIfName: &pA1, RemoteDeviceID: &b, RemotePortID: &pB1, Protocol: "lldp"},
		{LocalDeviceID: a, LocalIfIndex: 7, LocalIfName: &pA2, RemoteDeviceID: &b, RemotePortID: &pB2, Protocol: "lldp"},
		{LocalDeviceID: b, LocalIfIndex: 1, LocalIfName: &pB1, RemoteDeviceID: &a, RemotePortID: &pA1, Protocol: "lldp"},
		{LocalDeviceID: b, LocalIfIndex: 2, LocalIfName: &pB2, RemoteDeviceID: &a, RemotePortID: &pA2, Protocol: "lldp"},
	}
	out := dedupeTopologyEdges(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 physical cables, got %d: %+v", len(out), out)
	}
}

func TestApplyTopologyFilterProtocolAndStale(t *testing.T) {
	rid := int64(2)
	g := &TopologyGraph{
		Nodes: []TopologyNode{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}},
		Edges: []TopologyEdge{
			{LocalDeviceID: 1, LocalIfIndex: 1, RemoteDeviceID: &rid, Protocol: "lldp", Protocols: []string{"lldp"}, Stale: false},
			{LocalDeviceID: 1, LocalIfIndex: 2, RemoteDeviceID: &rid, Protocol: "cdp", Protocols: []string{"cdp"}, Stale: true},
		},
	}
	staleOff := false
	out := ApplyTopologyFilter(g, TopologyFilter{Protocol: "lldp", IncludeStale: &staleOff})
	if len(out.Edges) != 1 || out.Edges[0].Protocol != "lldp" {
		t.Fatalf("unexpected edges: %+v", out.Edges)
	}
}

func TestApplyTopologyFilterDepth(t *testing.T) {
	id2, id3 := int64(2), int64(3)
	g := &TopologyGraph{
		Nodes: []TopologyNode{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}, {ID: 99, Name: "z"}},
		Edges: []TopologyEdge{
			{LocalDeviceID: 1, LocalIfIndex: 1, RemoteDeviceID: &id2, Protocol: "lldp"},
			{LocalDeviceID: 2, LocalIfIndex: 1, RemoteDeviceID: &id3, Protocol: "lldp"},
		},
	}
	start := int64(1)
	depth := 1
	out := ApplyTopologyFilter(g, TopologyFilter{DeviceID: &start, Depth: &depth})
	ids := map[int64]bool{}
	for _, n := range out.Nodes {
		ids[n.ID] = true
	}
	if !ids[1] || !ids[2] || ids[3] || ids[99] {
		t.Fatalf("depth=1 from 1 should keep 1,2 only; got %v", ids)
	}
}

func TestFilterSpuriousRouterRouterEdges(t *testing.T) {
	patio, dodo, sw := int64(1), int64(2), int64(3)
	ether2 := "ether2"
	swPort := "0/10"
	edges := []TopologyEdge{
		{LocalDeviceID: patio, LocalIfIndex: 2, RemoteDeviceID: &sw, RemotePortID: &swPort, Protocol: "lldp"},
		{LocalDeviceID: patio, LocalIfIndex: 2, RemoteDeviceID: &dodo, RemotePortID: &ether2, Protocol: "lldp"},
		{LocalDeviceID: dodo, LocalIfIndex: 2, RemoteDeviceID: &sw, RemotePortID: &swPort, Protocol: "lldp"},
		{LocalDeviceID: dodo, LocalIfIndex: 2, RemoteDeviceID: &patio, RemotePortID: &ether2, Protocol: "lldp"},
	}
	kinds := map[int64]string{patio: "router", dodo: "router", sw: "switch"}
	out := filterSpuriousRouterRouterEdges(edges, kinds)
	if len(out) != 2 {
		t.Fatalf("expected 2 edges (router→switch only), got %d: %+v", len(out), out)
	}
	for _, e := range out {
		if e.RemoteDeviceID != nil && *e.RemoteDeviceID != sw {
			t.Fatalf("unexpected router-router edge kept: %+v", e)
		}
	}
}

func TestFilterSpuriousRouterRouterEdgesKeepsDirect(t *testing.T) {
	a, b := int64(1), int64(2)
	pA, pB := "ether1", "ether2"
	edges := []TopologyEdge{
		{LocalDeviceID: a, LocalIfIndex: 1, RemoteDeviceID: &b, RemotePortID: &pB, Protocol: "lldp"},
		{LocalDeviceID: b, LocalIfIndex: 2, RemoteDeviceID: &a, RemotePortID: &pA, Protocol: "lldp"},
	}
	kinds := map[int64]string{a: "router", b: "router"}
	out := filterSpuriousRouterRouterEdges(edges, kinds)
	if len(out) != 2 {
		t.Fatalf("direct router-router should stay, got %d edges", len(out))
	}
}

func strPtr(v string) *string { return &v }
