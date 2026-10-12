package investigate

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestBFSDevicePath(t *testing.T) {
	names := map[int64]string{1: "core", 2: "dist", 3: "access"}
	adj := map[int64][]l2AdjEdge{
		1: {{to: 2, localIf: 10, localName: "Gi0/1"}},
		2: {{to: 1, localIf: 1, localName: "up"}, {to: 3, localIf: 20, localName: "Gi0/2"}},
		3: {{to: 2, localIf: 1, localName: "uplink"}},
	}
	hops, ok := bfsDevicePath(adj, names, 1, 3)
	if !ok {
		t.Fatal("expected path")
	}
	if len(hops) != 3 {
		t.Fatalf("hops=%d %+v", len(hops), hops)
	}
	if hops[0].DeviceID != 1 || hops[2].DeviceID != 3 {
		t.Fatalf("order %+v", hops)
	}
}

func TestPickTopologyRootPrefersLinkCount(t *testing.T) {
	nodes := []store.TopologyNode{
		{ID: 1, Name: "a", LinkCount: 1, Kind: "switch"},
		{ID: 2, Name: "core", LinkCount: 8, Kind: "router"},
	}
	lc := map[int64]int{1: 1, 2: 8}
	if pickTopologyRoot(nodes, lc) != 2 {
		t.Fatal("want device 2")
	}
}
