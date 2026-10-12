package investigate

import (
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestLooksLikeVirtualMAC(t *testing.T) {
	if !looksLikeVirtualMAC("52:54:4c:83:09:e0") {
		t.Fatal("expected virtualization hint for 52:54:4c…")
	}
	if !isLocallyAdministered("52:54:4c:83:09:e0") {
		t.Fatal("52:… should be LAA")
	}
}

func TestSortFootprintSuspectAccessFirst(t *testing.T) {
	foot := []FootprintEntry{
		{MACFootprintHit: store.MACFootprintHit{DeviceID: 1, DeviceName: "core", IfIndex: 1, PortRole: "trunk", MACCount: 200}},
		{MACFootprintHit: store.MACFootprintHit{DeviceID: 2, DeviceName: "edge", IfIndex: 10, PortRole: "access", MACCount: 1}},
		{MACFootprintHit: store.MACFootprintHit{DeviceID: 3, DeviceName: "hub", IfIndex: 5, PortRole: "access", MACCount: 12}},
	}
	sortFootprint(foot)
	if foot[0].DeviceName != "edge" || foot[0].MACCount != 1 {
		t.Fatalf("want edge/1 first, got %+v", foot[0])
	}
}

func TestCoreLoopBroadcastHypothesis(t *testing.T) {
	foot := make([]FootprintEntry, 0, 12)
	for i := int64(1); i <= 12; i++ {
		foot = append(foot, FootprintEntry{
			MACFootprintHit: store.MACFootprintHit{DeviceID: i, DeviceName: "sw", IfIndex: 1, PortRole: "access", MACCount: 1},
		})
	}
	rep := &Report{
		Identity:  Identity{MAC: "52:54:4c:83:09:e0", VirtualizationHint: true},
		Footprint: foot,
	}
	hs := buildHypotheses(rep)
	ids := map[string]bool{}
	for _, h := range hs {
		ids[h.ID] = true
	}
	if !ids["core_loop_broadcast"] {
		t.Fatalf("want core_loop_broadcast, got %+v", hs)
	}
	if ids["kvm_dual_uplink"] {
		t.Fatal("kvm_dual_uplink should be suppressed when core_loop_broadcast fires")
	}
}

func TestBuildHypothesesKVMDualUplink(t *testing.T) {
	from, to := 10, 23
	now := time.Now()
	rep := &Report{
		Identity: Identity{MAC: "52:54:4c:83:09:e0", VirtualizationHint: true, LocallyAdmin: true},
		Timeline: []TimelineEntry{
			{MACFDBMove: store.MACFDBMove{
				MAC: "52:54:4c:83:09:e0", DeviceID: 1,
				FromIfIndex: &from, ToIfIndex: &to, SeenAt: now, Source: store.MACMoveSourceSyslog,
			}},
			{MACFDBMove: store.MACFDBMove{
				MAC: "52:54:4c:83:09:e0", DeviceID: 1,
				FromIfIndex: &to, ToIfIndex: &from, SeenAt: now, Source: store.MACMoveSourceFDBPoll,
			}},
		},
		Footprint: []FootprintEntry{
			{MACFootprintHit: store.MACFootprintHit{DeviceID: 1, IfIndex: 10, PortRole: "access", MACCount: 1}},
			{MACFootprintHit: store.MACFootprintHit{DeviceID: 1, IfIndex: 23, PortRole: "access", MACCount: 1}},
		},
	}
	hs := buildHypotheses(rep)
	ids := map[string]bool{}
	for _, h := range hs {
		ids[h.ID] = true
	}
	if !ids["kvm_dual_uplink"] {
		t.Fatalf("want kvm_dual_uplink for 52:54 dual-port flap, got %+v", hs)
	}
	if ids["unmanaged_loop"] {
		t.Fatal("unmanaged_loop should be superseded by kvm_dual_uplink for QEMU MAC")
	}
	if ids["virtualization_mac"] {
		t.Fatal("generic virtualization_mac should not duplicate kvm_dual_uplink")
	}
}

func TestHypervisorLabel(t *testing.T) {
	if hypervisorLabel("52:54:4c:83:09:e0") != "QEMU/KVM" {
		t.Fatal("expected QEMU/KVM")
	}
	if hypervisorLabel("00:50:56:12:34:56") != "VMware" {
		t.Fatal("expected VMware")
	}
}
