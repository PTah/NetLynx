package investigate

import (
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestBuildMoveGraph(t *testing.T) {
	from, to := 10, 23
	now := time.Now()
	g := buildMoveGraph(
		[]TimelineEntry{
			{MACFDBMove: store.MACFDBMove{
				DeviceID: 1, FromIfIndex: &from, ToIfIndex: &to, SeenAt: now, Source: "syslog",
			}, DeviceName: "sw38"},
			{MACFDBMove: store.MACFDBMove{
				DeviceID: 1, FromIfIndex: &to, ToIfIndex: &from, SeenAt: now, Source: "fdb_poll",
			}, DeviceName: "sw38"},
		},
		nil,
		map[string]string{"1:10": "gi1/0/10", "1:23": "gi1/0/23"},
	)
	if len(g.Nodes) != 2 {
		t.Fatalf("nodes=%d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("edges=%d %+v", len(g.Edges), g.Edges)
	}
	if g.Edges[0].Count != 2 {
		t.Fatalf("count=%d", g.Edges[0].Count)
	}
}
