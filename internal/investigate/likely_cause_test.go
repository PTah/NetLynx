package investigate

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestClassifyLikelyCausePortFlap(t *testing.T) {
	ifIdx := 7
	rep := &Report{
		Footprint: []FootprintEntry{{MACFootprintHit: store.MACFootprintHit{IfIndex: 7}}},
		Correlated: []store.EventBrief{
			{EventType: "PORT_FLAP", IfIndex: &ifIdx},
		},
	}
	lc := classifyLikelyCause(rep)
	if lc == nil || lc.ID != "port_flap_cable" {
		t.Fatalf("%+v", lc)
	}
}

func TestClassifyLikelyCauseSTP(t *testing.T) {
	rep := &Report{
		Correlated: []store.EventBrief{
			{EventType: "STP_TOPOLOGY_CHANGE"},
		},
	}
	lc := classifyLikelyCause(rep)
	if lc == nil || lc.ID != "stp_reconvergence" {
		t.Fatalf("%+v", lc)
	}
}

func TestClassifyLikelyCauseL2Loop(t *testing.T) {
	rep := &Report{
		Hypotheses:    []Hypothesis{{ID: "unmanaged_loop"}},
		LoopsTouching: []TopologyCycle{{Summary: "A→B→C→A", DeviceIDs: []int64{1, 2, 3}}},
		Correlated:    []store.EventBrief{{EventType: "MAC_FLAPPING"}},
	}
	lc := classifyLikelyCause(rep)
	if lc == nil || lc.ID != "l2_loop" {
		t.Fatalf("%+v", lc)
	}
}
