package investigate

import (
	"testing"
)

func chainAdj(n int) map[int64][]int64 {
	adj := map[int64][]int64{}
	for i := 1; i <= n; i++ {
		id := int64(i)
		if i > 1 {
			adj[id] = append(adj[id], int64(i-1))
		}
		if i < n {
			adj[id] = append(adj[id], int64(i+1))
		}
	}
	return adj
}

func TestShortestPathLongChain(t *testing.T) {
	adj := chainAdj(8)
	path := ShortestPath(adj, 1, 8, 0)
	if len(path) != 8 {
		t.Fatalf("want 8 hops nodes, got %v", path)
	}
	for i, id := range path {
		if id != int64(i+1) {
			t.Fatalf("path[%d]=%d", i, id)
		}
	}
	viaAvoid := ShortestPath(adj, 1, 8, 4)
	if len(viaAvoid) != 0 {
		t.Fatalf("chain has no alternate avoiding 4, got %v", viaAvoid)
	}
}

func TestWalkDownDepth32VsCap(t *testing.T) {
	adj := chainAdj(40)
	dist := BFSDistances(adj, 1)
	always := func(int64) bool { return true }

	deep := WalkDownWithVLAN(adj, 2, 1, 32, dist, always)
	if len(deep) < 30 {
		t.Fatalf("depth 32 should reach far down chain, got %d hits", len(deep))
	}
	capped := WalkDownWithVLAN(adj, 2, 1, 4, dist, always)
	if len(capped) > 4 {
		t.Fatalf("depth 4 should cap, got %d", len(capped))
	}
	for _, h := range capped {
		if h.Depth > 4 {
			t.Fatalf("hit depth %d > 4", h.Depth)
		}
	}
}

func TestEffectiveVLANBlastMaxDepth(t *testing.T) {
	if EffectiveVLANBlastMaxDepth(-1) != DefaultVLANBlastMaxDepth {
		t.Fatal("negative → default")
	}
	if EffectiveVLANBlastMaxDepth(0) < 1000 {
		t.Fatal("0 should be unlimited (large cap)")
	}
	if EffectiveVLANBlastMaxDepth(32) != 32 {
		t.Fatal("32 passthrough")
	}
}

func TestShutDownstreamWalkCounts(t *testing.T) {
	// root=1 — Upper; Y=2; neighbor behind port=3 → chain 3-4-5-6
	adj := map[int64][]int64{
		1: {2},
		2: {1, 3},
		3: {2, 4},
		4: {3, 5},
		5: {4, 6},
		6: {5},
	}
	dist := BFSDistances(adj, 1)
	always := func(int64) bool { return true }
	desc := WalkDownWithVLAN(adj, 3, 2, 32, dist, always)
	if len(desc) != 3 { // 4,5,6
		t.Fatalf("want 3 downstream, got %d %+v", len(desc), desc)
	}
	path := ShortestPath(adj, 3, 6, 2)
	if len(path) != 4 {
		t.Fatalf("neighbor→leaf path: %v", path)
	}
}
