package store

import "testing"

func TestPortRolesForFDBTopologyUsesCLI(t *testing.T) {
	trunk := "trunk"
	access := "access"
	ifs := map[int]InterfaceSnapshot{
		1: {PortRole: "access", CLIPortMode: &trunk},
		2: {PortRole: "trunk", CLIPortMode: &access},
		3: {PortRole: "access"},
	}
	got := PortRolesForFDBTopology(ifs)
	if got[1] != "trunk" || got[2] != "access" || got[3] != "access" {
		t.Fatalf("unexpected roles: %#v", got)
	}
}
