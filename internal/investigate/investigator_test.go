package investigate

import (
	"context"
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestDuplicateMACInvestigator(t *testing.T) {
	rep := &Report{
		Identity: Identity{MAC: "aa:bb:cc:dd:ee:ff"},
		Footprint: []FootprintEntry{
			{MACFootprintHit: store.MACFootprintHit{DeviceID: 1, IfIndex: 1, PortRole: "access"}},
			{MACFootprintHit: store.MACFootprintHit{DeviceID: 2, IfIndex: 3, PortRole: "access"}},
		},
	}
	hs := DuplicateMACInvestigator{}.Investigate(context.Background(), nil, rep)
	if len(hs) != 1 || hs[0].ID != "duplicate_mac" {
		t.Fatalf("got %+v", hs)
	}
}

func TestPortStormInvestigatorNeedsEvents(t *testing.T) {
	rep := &Report{
		Identity: Identity{MAC: "aa:bb:cc:dd:ee:01"},
		Footprint: []FootprintEntry{
			{MACFootprintHit: store.MACFootprintHit{DeviceID: 1, IfIndex: 1}},
		},
	}
	hs := PortStormInvestigator{}.Investigate(context.Background(), nil, rep)
	if len(hs) != 0 {
		t.Fatalf("nil store should yield nothing, got %+v", hs)
	}
}

func TestMergeHypothesesDropsInsufficient(t *testing.T) {
	base := []Hypothesis{
		{ID: "insufficient_data", Title: "нет данных"},
		{ID: "kvm_dual_uplink", Title: "kvm"},
	}
	extra := []Hypothesis{{ID: "rogue_mac", Title: "rogue"}}
	out := mergeHypotheses(base, extra)
	ids := map[string]bool{}
	for _, h := range out {
		ids[h.ID] = true
	}
	if ids["insufficient_data"] {
		t.Fatal("insufficient_data should be dropped when real hyps exist")
	}
	if !ids["rogue_mac"] || !ids["kvm_dual_uplink"] {
		t.Fatalf("missing hyps: %+v", out)
	}
}

func TestPreferInvestigatorIDs(t *testing.T) {
	hs := []Hypothesis{
		{ID: "dual_homed_or_clone"},
		{ID: "duplicate_mac"},
		{ID: "kvm_dual_uplink"},
	}
	out := preferInvestigatorIDs(hs)
	for _, h := range out {
		if h.ID == "dual_homed_or_clone" {
			t.Fatal("dual_homed should be superseded")
		}
	}
	_ = time.Now
}
