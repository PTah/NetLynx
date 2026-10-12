package api

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

func TestMatchVLANInInventory(t *testing.T) {
	inv := []swcfg.VLANInventoryRow{
		{VLANID: 10, Name: "users", InDatabase: true},
		{VLANID: 14, Name: "ost", InDatabase: true, AccessPorts: []swcfg.VLANPortRef{{IfIndex: 2, IfName: "0/2"}}},
		{VLANID: 99, InDatabase: false, FDBPorts: []swcfg.VLANPortRef{{IfIndex: 1}}},
		{VLANID: 20, InDatabase: false, TaggedPorts: []swcfg.VLANPortRef{{IfIndex: 5, IfName: "0/5"}}},
	}

	ok, inDB, onPorts, name := matchVLANInInventory(inv, 14)
	if !ok || !inDB || !onPorts || name != "ost" {
		t.Fatalf("vlan 14: ok=%v inDB=%v onPorts=%v name=%q", ok, inDB, onPorts, name)
	}

	ok, inDB, onPorts, name = matchVLANInInventory(inv, 10)
	if !ok || !inDB || onPorts || name != "users" {
		t.Fatalf("vlan 10: ok=%v inDB=%v onPorts=%v name=%q", ok, inDB, onPorts, name)
	}

	ok, inDB, onPorts, _ = matchVLANInInventory(inv, 20)
	if !ok || inDB || !onPorts {
		t.Fatalf("vlan 20 tagged-only: ok=%v inDB=%v onPorts=%v", ok, inDB, onPorts)
	}

	ok, _, _, _ = matchVLANInInventory(inv, 99)
	if ok {
		t.Fatal("FDB-only must not match")
	}

	ok, _, _, _ = matchVLANInInventory(inv, 15)
	if ok {
		t.Fatal("missing vlan must not match")
	}
}

func TestParseVLANIDsQuery(t *testing.T) {
	ids, err := parseVLANIDsQuery("14,15,14")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != 14 || ids[1] != 15 {
		t.Fatalf("got %v", ids)
	}

	ids, err = parseVLANIDsQuery("1,14")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != 14 {
		t.Fatalf("vlan 1 skipped: %v", ids)
	}

	if _, err := parseVLANIDsQuery(""); err == nil {
		t.Fatal("empty expected error")
	}
	if _, err := parseVLANIDsQuery("1"); err == nil {
		t.Fatal("only vlan 1 expected error")
	}
}
