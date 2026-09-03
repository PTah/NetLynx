package poller

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestInferRoleFromSnapshotCLIPortMode(t *testing.T) {
	trunk := "trunk"
	snap := store.InterfaceSnapshot{IfName: strPtr("0/8"), IfDescr: strPtr("0/8"), CLIPortMode: &trunk}
	if got := inferRoleFromSnapshot(snap); got != "trunk" {
		t.Fatalf("expected trunk from cli_port_mode, got %q", got)
	}
	snap2 := store.InterfaceSnapshot{IfName: strPtr("0/8"), PortRole: "trunk"}
	if got := inferRoleFromSnapshot(snap2); got != "trunk" {
		t.Fatalf("expected trunk from port_role, got %q", got)
	}
}

func TestInferPortRoleAccess(t *testing.T) {
	role := inferPortRole("Gi1/0/1", "Port 1", 6, store.InterfaceSnapshot{})
	if role != "access" {
		t.Fatalf("expected access, got %q", role)
	}
}

func TestInferPortRoleTrunkDescrTO(t *testing.T) {
	role := inferPortRole("0/17", "port 17: TO-SERVERROOM-ES 48 #11 -port-52", 6, store.InterfaceSnapshot{})
	if role != "trunk" {
		t.Fatalf("expected trunk for TO- descr, got %q", role)
	}
	role2 := inferPortRole("0/18", "port 18: TO-EdgeSwitch 8-150 #1 - VentKamera #3", 6, store.InterfaceSnapshot{})
	if role2 != "trunk" {
		t.Fatalf("expected trunk for TO-EdgeSwitch, got %q", role2)
	}
}

func TestInferPortRoleIgnoreVlan(t *testing.T) {
	role := inferPortRole("Vlan100", "", 53, store.InterfaceSnapshot{})
	if role != "ignore" {
		t.Fatalf("expected ignore, got %q", role)
	}
	role2 := inferPortRole("Vlan1", "Vlan1", 136, store.InterfaceSnapshot{})
	if role2 != "ignore" {
		t.Fatalf("expected ignore for l3ipvlan, got %q", role2)
	}
}

func TestInferPortRoleAccessDespiteVLANInDescr(t *testing.T) {
	// SNR/Eltex: в ifAlias часто «ROOM-VLAN162-Name» — это физический порт, не VLAN-if.
	role := inferPortRole("Ethernet1/0/1", "ROOM2-VLAN162-Shelkovaya", 6, store.InterfaceSnapshot{})
	if role != "access" {
		t.Fatalf("expected access for Ethernet with VLAN in descr, got %q", role)
	}
	role2 := inferPortRole("Ethernet1/0/30", "ROOM7-VLAN34-Debian", 6, store.InterfaceSnapshot{})
	if role2 != "access" {
		t.Fatalf("expected access, got %q", role2)
	}
}

func TestIsLikelyTrunkByFDB(t *testing.T) {
	cfg := config.Config{FDBAutoTrunkMinMACs: 8, FDBAutoTrunkMinVLANs: 2, FDBAutoTrunkFallbackMinMACs: 12}
	if !isLikelyTrunkByFDB(snmp.FDBIfStats{MACCount: 10, VLANCount: 3}, cfg) {
		t.Fatal("expected trunk by MAC+VLAN")
	}
	if isLikelyTrunkByFDB(snmp.FDBIfStats{MACCount: 2, VLANCount: 1}, cfg) {
		t.Fatal("expected not trunk")
	}
	if !isLikelyTrunkByFDB(snmp.FDBIfStats{MACCount: 12, VLANCount: 0}, cfg) {
		t.Fatal("expected trunk by fallback MAC count")
	}
}

func TestActionMatchesFilter(t *testing.T) {
	types := "UNKNOWN_MAC_ON_ACCESS_PORT,MAC_MOVED"
	if !actionMatchesFilter("UNKNOWN_MAC_ON_ACCESS_PORT", &types) {
		t.Fatal("expected match")
	}
	if actionMatchesFilter("LINK_UP", &types) {
		t.Fatal("expected no match")
	}
}
