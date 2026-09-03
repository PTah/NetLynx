package store

import (
	"testing"
	"time"
)

func TestAttachmentCandidateBetter(t *testing.T) {
	if !attachmentCandidateBetter(4, 0, 100, 1, 1000000, 200, time.Time{}, time.Time{}) {
		t.Fatal("manual should beat strong FDB")
	}
	if !attachmentCandidateBetter(1, 1000009, 140, 1, 100, 140, time.Time{}, time.Time{}) {
		t.Fatal("edge access FDB should beat core ghost FDB")
	}
	if attachmentCandidateBetter(1, 100, 140, 1, 1000009, 140, time.Time{}, time.Time{}) {
		t.Fatal("weak FDB should not beat strong FDB at same proto priority")
	}
	t1 := time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)
	t0 := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	if !attachmentCandidateBetter(1, 140, 140, 1, 140, 140, t1, t0) {
		t.Fatal("later first_seen_at should win tie on equal score")
	}
	if attachmentCandidateBetter(1, 140, 140, 1, 140, 140, t0, t1) {
		t.Fatal("earlier first_seen_at should not win tie")
	}
}

func TestAttachmentProtoPriority(t *testing.T) {
	if attachmentProtoPriority("manual") <= attachmentProtoPriority("fdb") {
		t.Fatal("manual > fdb")
	}
	if attachmentProtoPriority("lldp") <= attachmentProtoPriority("fdb") {
		t.Fatal("lldp > fdb")
	}
}

func TestDeviceAttachmentMACHexKeys(t *testing.T) {
	mac := "d8:bb:c1:9c:03:be"
	ch := &mac
	keys := deviceAttachmentMACHexKeys(ch, []string{"aabbccddeeff"})
	if len(keys) != 2 {
		t.Fatalf("keys=%v", keys)
	}
	if keys[0] != "d8bbc19c03be" {
		t.Fatalf("chassis: %v", keys)
	}
	keys2 := deviceAttachmentMACHexKeys(nil, []string{"d8bbc19c03be"})
	if len(keys2) != 1 || keys2[0] != "d8bbc19c03be" {
		t.Fatalf("arp only: %v", keys2)
	}
	if len(deviceAttachmentMACHexKeys(nil, nil)) != 0 {
		t.Fatal("empty")
	}
}

func TestAttachmentScorePrefersAccessOverTrunk(t *testing.T) {
	accessFDB := attachmentScore(DeviceCategorySwitch, DeviceCategoryMFU, NeighborProtocolFDB, "access", true)
	trunkFDB := attachmentScore(DeviceCategorySwitch, DeviceCategoryMFU, NeighborProtocolFDB, "trunk", true)
	if accessFDB <= trunkFDB {
		t.Fatalf("access FDB %d should beat trunk FDB %d", accessFDB, trunkFDB)
	}
	accessLLDP := attachmentScore(DeviceCategorySwitch, DeviceCategoryMFU, "lldp", "access", true)
	trunkLLDP := attachmentScore(DeviceCategorySwitch, DeviceCategoryMFU, "lldp", "trunk", true)
	if accessLLDP <= trunkLLDP {
		t.Fatalf("access LLDP %d should beat trunk LLDP %d", accessLLDP, trunkLLDP)
	}
}

func TestAttachmentScoreManualAccessBeatsFDB(t *testing.T) {
	manualTrunk := attachmentScore(DeviceCategorySwitch, DeviceCategoryMFU, "manual", "trunk", true)
	accessFDB := attachmentScore(DeviceCategorySwitch, DeviceCategoryMFU, NeighborProtocolFDB, "access", true)
	if manualTrunk >= accessFDB {
		t.Fatalf("manual on trunk (%d) should lose to access FDB (%d)", manualTrunk, accessFDB)
	}
	manualAccess := attachmentScore(DeviceCategorySwitch, DeviceCategoryMFU, "manual", "access", true)
	if manualAccess <= accessFDB {
		t.Fatalf("manual access %d should beat access FDB %d", manualAccess, accessFDB)
	}
}

func TestAttachmentScoreAPTrunkSingleVsCoreFlood(t *testing.T) {
	edge := attachmentScore(DeviceCategorySwitch, DeviceCategoryAP, NeighborProtocolFDB, "trunk", true)
	core := attachmentScore(DeviceCategorySwitch, DeviceCategoryAP, NeighborProtocolFDB, "trunk", false)
	if edge <= core {
		t.Fatalf("edge AP trunk %d should beat core flood %d", edge, core)
	}
	access := attachmentScore(DeviceCategorySwitch, DeviceCategoryAP, NeighborProtocolFDB, "access", true)
	if access <= edge {
		t.Fatalf("access %d should beat edge trunk AP %d", access, edge)
	}
}

func TestInfraUplinkLocalPortEligible(t *testing.T) {
	if !infraUplinkLocalPortEligible("manual", "access") {
		t.Fatal("manual on any port")
	}
	if !infraUplinkLocalPortEligible("lldp", "trunk") {
		t.Fatal("lldp trunk")
	}
	if infraUplinkLocalPortEligible("lldp", "access") {
		t.Fatal("lldp access is not uplink")
	}
}

func TestFDBAttachmentRejectsComputerOnTrunk(t *testing.T) {
	if fdbTopoLinkEligible("trunk", DeviceCategoryComputer) {
		t.Fatal("computer on trunk must be rejected")
	}
	if !fdbTopoLinkEligible("access", DeviceCategoryComputer) {
		t.Fatal("computer on access should be allowed")
	}
	if fdbTopoLinkEligible("trunk", DeviceCategoryMFU) {
		t.Fatal("MFU on trunk must be rejected")
	}
}

func TestTrunkAPUplinkLikely(t *testing.T) {
	if trunkAPUplinkLikely(100, 1, 2, 20) {
		t.Fatal("core port with VLAN flood must not pass")
	}
	if trunkAPUplinkLikely(3, 1, 1, 15) {
		t.Fatal("core ghost port with few MACs should fail")
	}
	if !trunkAPUplinkLikely(3, 1, 1, 0) {
		t.Fatal("edge AP uplink with few MACs should pass")
	}
	if !trunkAPUplinkLikely(8, 1, 1, 3) {
		t.Fatal("multi-AP edge (3 single-AP trunk siblings) should pass")
	}
	if trunkAPUplinkLikely(3, 1, 1, 4) {
		t.Fatal("core-like switch with 4+ single-AP trunk siblings should fail")
	}
}
