package investigate

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestFindUndirectedCyclesTriangle(t *testing.T) {
	rid := func(id int64) *int64 { return &id }
	edges := []store.TopologyEdge{
		{LocalDeviceID: 1, LocalIfIndex: 10, RemoteDeviceID: rid(2), Protocol: "lldp"},
		{LocalDeviceID: 2, LocalIfIndex: 20, RemoteDeviceID: rid(3), Protocol: "lldp"},
		{LocalDeviceID: 3, LocalIfIndex: 30, RemoteDeviceID: rid(1), Protocol: "lldp"},
	}
	links, adj := buildUndirectedLLDPGraph(edges)
	if len(links) != 3 {
		t.Fatalf("links=%d want 3", len(links))
	}
	names := map[int64]string{1: "A", 2: "B", 3: "C"}
	cycles := findUndirectedCycles(adj, links, names)
	if len(cycles) != 1 {
		t.Fatalf("cycles=%d want 1: %+v", len(cycles), cycles)
	}
	if cycles[0].Length != 3 {
		t.Fatalf("length=%d want 3", cycles[0].Length)
	}
}

func TestFindUndirectedCyclesNoLoop(t *testing.T) {
	rid := func(id int64) *int64 { return &id }
	edges := []store.TopologyEdge{
		{LocalDeviceID: 1, LocalIfIndex: 1, RemoteDeviceID: rid(2), Protocol: "lldp"},
		{LocalDeviceID: 2, LocalIfIndex: 2, RemoteDeviceID: rid(3), Protocol: "lldp"},
	}
	links, adj := buildUndirectedLLDPGraph(edges)
	cycles := findUndirectedCycles(adj, links, map[int64]string{1: "a", 2: "b", 3: "c"})
	if len(cycles) != 0 {
		t.Fatalf("expected no cycles, got %+v", cycles)
	}
}

func TestParallelLinksCycle(t *testing.T) {
	rid := func(id int64) *int64 { return &id }
	edges := []store.TopologyEdge{
		{LocalDeviceID: 1, LocalIfIndex: 10, RemoteDeviceID: rid(2), Protocol: "lldp"},
		{LocalDeviceID: 1, LocalIfIndex: 11, RemoteDeviceID: rid(2), Protocol: "lldp"},
	}
	cycles := parallelLinkCyclesFromEdges(edges, map[int64]string{1: "SW1", 2: "SW2"})
	if len(cycles) < 1 {
		t.Fatal("want parallel-link cycle from two local ports")
	}
	// reverse of same two cables should not double-count as second cycle type alone
	edges2 := append(edges,
		store.TopologyEdge{LocalDeviceID: 2, LocalIfIndex: 20, RemoteDeviceID: rid(1), Protocol: "lldp"},
		store.TopologyEdge{LocalDeviceID: 2, LocalIfIndex: 21, RemoteDeviceID: rid(1), Protocol: "lldp"},
	)
	cycles2 := parallelLinkCyclesFromEdges(edges2, map[int64]string{1: "SW1", 2: "SW2"})
	if len(cycles2) != 1 {
		t.Fatalf("want 1 parallel cycle, got %d", len(cycles2))
	}
}

func TestBidirectionalSingleLinkNotParallel(t *testing.T) {
	rid := func(id int64) *int64 { return &id }
	edges := []store.TopologyEdge{
		{LocalDeviceID: 1, LocalIfIndex: 10, RemoteDeviceID: rid(2), Protocol: "lldp"},
		{LocalDeviceID: 2, LocalIfIndex: 20, RemoteDeviceID: rid(1), Protocol: "lldp"},
	}
	cycles := parallelLinkCyclesFromEdges(edges, map[int64]string{1: "SW1", 2: "SW2"})
	if len(cycles) != 0 {
		t.Fatalf("single cable both directions must not be parallel: %+v", cycles)
	}
}

func TestCycleKeyNormalized(t *testing.T) {
	a := cycleKey([]int64{3, 1, 2})
	b := cycleKey([]int64{1, 2, 3})
	c := cycleKey([]int64{1, 3, 2})
	if a != b {
		t.Fatalf("rotation should match: %s vs %s", a, b)
	}
	if a != c {
		t.Fatalf("reverse should match: %s vs %s", a, c)
	}
}
