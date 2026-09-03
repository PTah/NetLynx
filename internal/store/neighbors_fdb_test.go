package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestUpsertFDBTopologyNeighborRejectsBadInput(t *testing.T) {
	s := &Store{}
	err := s.UpsertFDBTopologyNeighbor(context.Background(), 0, 1, "aa:bb:cc:dd:ee:ff", nil, nil, time.Time{})
	if err == nil {
		t.Fatal("expected error for device_id=0")
	}
	err = s.UpsertFDBTopologyNeighbor(context.Background(), 1, 1, "aa:bb", nil, nil, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "MAC") {
		t.Fatalf("expected MAC error, got %v", err)
	}
}

func TestHasPortFDBEntryInvalidMAC(t *testing.T) {
	s := &Store{}
	ok, err := s.HasPortFDBEntry(context.Background(), 1, 1, "zz")
	if err != nil || ok {
		t.Fatalf("invalid MAC should be false,nil got ok=%v err=%v", ok, err)
	}
}

func TestNeighborProtocolFDB(t *testing.T) {
	if NeighborProtocolFDB != "fdb" {
		t.Fatalf("got %q", NeighborProtocolFDB)
	}
}

func TestFdbTopoPortEligible(t *testing.T) {
	if !fdbTopoPortEligible("access", 1, 0, 0, 0) {
		t.Fatal("access with 1 MAC should be eligible")
	}
	if fdbTopoPortEligible("access", 11, 0, 0, 0) {
		t.Fatal("access with flood of MACs should be rejected")
	}
	if fdbTopoPortEligible("trunk", 100, 1, 2, 20) {
		t.Fatal("core trunk flood must not be eligible")
	}
	if !fdbTopoPortEligible("trunk", 3, 1, 1, 0) {
		t.Fatal("edge trunk with 1 AP and few MACs should be eligible")
	}
	if fdbTopoPortEligible("trunk", 50, 2, 2, 0) {
		t.Fatal("trunk with 2+ AP must not link")
	}
}

func TestMaxSiblingTrunkAPCount(t *testing.T) {
	ap := map[int]int{1: 1, 2: 15, 3: 1, 4: 1}
	role := map[int]string{1: "trunk", 2: "trunk", 3: "trunk", 4: "access"}
	if got := maxSiblingTrunkAPCount(ap, role, 1); got != 1 {
		t.Fatalf("port 1 want 1 single-AP trunk sibling (port 3), got %d", got)
	}
	if got := maxSiblingTrunkAPCount(ap, role, 2); got != 2 {
		t.Fatalf("flood port 2 should count single-AP siblings only, got %d", got)
	}
}

func TestFdbTopoLinkEligible(t *testing.T) {
	if !fdbTopoLinkEligible("access", DeviceCategoryMFU) {
		t.Fatal("access should link any inventory type")
	}
	if !fdbTopoLinkEligible("trunk", DeviceCategoryAP) {
		t.Fatal("trunk should link AP")
	}
	if fdbTopoLinkEligible("trunk", DeviceCategoryMFU) {
		t.Fatal("trunk must not link MFU/PC from VLAN flood")
	}
}

func TestInventoryStatsByPortAPOnly(t *testing.T) {
	chassis := map[string]ChassisEndpoint{
		"aabbccddeeff": {ID: 1, Name: "AP", Category: DeviceCategoryAP},
		"aabbccddee01": {ID: 1, Name: "AP", Category: DeviceCategoryAP},
		"112233445566": {ID: 2, Name: "PC", Category: DeviceCategoryComputer},
	}
	entries := map[string]FDBLearnedEntry{
		"aa:bb:cc:dd:ee:ff": {IfIndex: 24},
		"aa:bb:cc:dd:ee:01": {IfIndex: 24},
		"11:22:33:44:55:66": {IfIndex: 24},
		"de:ad:be:ef:00:01": {IfIndex: 24},
	}
	got := inventoryStatsByPort(entries, chassis)
	if got.apDeviceCount[24] != 1 {
		t.Fatalf("want 1 AP device on port, got %d", got.apDeviceCount[24])
	}
	if got.inventoryMACCount[24] != 3 {
		t.Fatalf("want 3 inventory MAC on port, got %d", got.inventoryMACCount[24])
	}
}

func TestFdbAPLinkScore(t *testing.T) {
	access := fdbAPLinkScore("access", 2, 0, 0, 0)
	edgeTrunk := fdbAPLinkScore("trunk", 3, 1, 1, 0)
	coreGhost := fdbAPLinkScore("trunk", 64, 1, 1, 20)
	coreFew := fdbAPLinkScore("trunk", 3, 1, 1, 15)
	if access < 1000000 {
		t.Fatalf("access want tier-1 score, got %d", access)
	}
	if edgeTrunk < 1000000 {
		t.Fatalf("edge trunk with few MACs want tier-1 score, got %d", edgeTrunk)
	}
	if coreGhost >= 0 {
		t.Fatalf("core flood ghost should score -1, got %d", coreGhost)
	}
	if coreFew >= 0 {
		t.Fatalf("core ghost with few MACs should score -1, got %d", coreFew)
	}
	if fdbTopoPortEligible("trunk", 50, 2, 2, 0) {
		t.Fatal("trunk with 2+ AP must not get FDB topology links")
	}
	if !fdbTopoPortEligible("trunk", 3, 1, 1, 0) {
		t.Fatal("edge trunk with 1 AP and few MACs should be eligible")
	}
	if fdbTopoPortEligible("trunk", 64, 1, 1, 20) {
		t.Fatal("core trunk flood must not be eligible")
	}
}

func TestResolveInterfacePortRoleCLIPriority(t *testing.T) {
	trunk := "trunk"
	access := "access"
	if got := ResolveInterfacePortRole("access", &trunk); got != "trunk" {
		t.Fatalf("cli_port_mode trunk wins, got %q", got)
	}
	if got := ResolveInterfacePortRole("trunk", &access); got != "access" {
		t.Fatalf("cli_port_mode access wins, got %q", got)
	}
	if got := ResolveInterfacePortRole("trunk", nil); got != "trunk" {
		t.Fatalf("port_role trunk, got %q", got)
	}
}
