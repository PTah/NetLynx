package store

import "testing"

func TestEventDisplayDescr(t *testing.T) {
	override := "ручная"
	cli := "ROOM2-VLAN162-KOVAL"
	snmp := "GigabitEthernet1/0/6"
	got := eventDisplayDescr(&override, &cli, &snmp)
	if got != "ручная" {
		t.Fatalf("override: got %q", got)
	}
	got = eventDisplayDescr(nil, &cli, &snmp)
	if got != "ROOM2-VLAN162-KOVAL" {
		t.Fatalf("cli: got %q", got)
	}
	got = eventDisplayDescr(nil, nil, &snmp)
	if got != "GigabitEthernet1/0/6" {
		t.Fatalf("snmp: got %q", got)
	}
}

func TestApplyEventIfaceLabels(t *testing.T) {
	pl := map[string]interface{}{"source": "trap"}
	ApplyEventIfaceLabels(pl, EventIfaceLabels{
		IfName:  "Ethernet1/0/6",
		IfDescr: "ROOM2-VLAN162-KOVAL",
		IfAlias: "ROOM2-VLAN162-KOVAL",
	})
	if pl["if_descr"] != "ROOM2-VLAN162-KOVAL" {
		t.Fatalf("if_descr: %v", pl["if_descr"])
	}
	if pl["source"] != "trap" {
		t.Fatal("must keep trap fields")
	}
}
