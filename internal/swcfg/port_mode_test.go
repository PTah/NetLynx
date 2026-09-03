package swcfg

import "testing"

func TestParseRunningConfigPortModes_EdgeSwitch(t *testing.T) {
	raw := `
interface 0/1
switchport mode trunk
exit
interface 0/17
switchport mode trunk
lldp transmit-tlv port-desc
exit
interface 0/18
switchport mode trunk
exit
interface 0/2
switchport mode access
switchport access vlan 190
exit
interface 0/15
description 'Port 15: camera .211.184 vhod i vyhod DDX'
switchport mode access
exit
`
	m := ParseRunningConfigPortModes(raw)
	if len(m) != 5 {
		t.Fatalf("expected 5 interfaces, got %d", len(m))
	}
	if m["0/17"].Mode != "trunk" {
		t.Fatalf("0/17 mode: %q", m["0/17"].Mode)
	}
	if m["0/18"].Mode != "trunk" {
		t.Fatalf("0/18 mode: %q", m["0/18"].Mode)
	}
	if m["0/2"].Mode != "access" || m["0/2"].AccessVLAN == nil || *m["0/2"].AccessVLAN != 190 {
		t.Fatalf("0/2: %+v", m["0/2"])
	}
	p15 := m["0/15"]
	if !p15.HasDescr || p15.Description != "Port 15: camera .211.184 vhod i vyhod DDX" {
		t.Fatalf("0/15 description: has=%v %q", p15.HasDescr, p15.Description)
	}
}

func TestUnquoteCLIDescription(t *testing.T) {
	if unquoteCLIDescription(`'hello'`) != "hello" {
		t.Fatal("single")
	}
	if unquoteCLIDescription(`"hello"`) != "hello" {
		t.Fatal("double")
	}
	if unquoteCLIDescription(`plain`) != "plain" {
		t.Fatal("plain")
	}
}

func TestLooksLikeHardwareIfDescr(t *testing.T) {
	if !LooksLikeHardwareIfDescr("Slot: 0 Port: 15 Gigabit - Level") {
		t.Fatal("slot")
	}
	if LooksLikeHardwareIfDescr("Port 15: camera") {
		t.Fatal("human")
	}
}

func TestMatchConfigIfaceToIfIndex(t *testing.T) {
	names := map[int]string{
		17: "0/17",
		18: "0/18",
		1:  "0/1",
	}
	idx, ok := MatchConfigIfaceToIfIndex("0/17", names)
	if !ok || idx != 17 {
		t.Fatalf("got %d ok=%v", idx, ok)
	}
	idx, ok = MatchConfigIfaceToIfIndex("gi0/18", names)
	if !ok || idx != 18 {
		t.Fatalf("gi0/18: got %d ok=%v", idx, ok)
	}
}

func TestPortRoleFromCLIMode(t *testing.T) {
	if PortRoleFromCLIMode("trunk") != "trunk" {
		t.Fatal("trunk")
	}
	if PortRoleFromCLIMode("access") != "access" {
		t.Fatal("access")
	}
}

func TestNormalizeIfaceKey(t *testing.T) {
	if NormalizeIfaceKey("GigabitEthernet0/17") != "0/17" {
		t.Fatalf("got %q", NormalizeIfaceKey("GigabitEthernet0/17"))
	}
	if NormalizeIfaceKey("0/17") != "0/17" {
		t.Fatalf("got %q", NormalizeIfaceKey("0/17"))
	}
}
