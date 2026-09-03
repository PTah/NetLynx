package syslogrecv

import "testing"

func TestParseEltexMACFlapping(t *testing.T) {
	raw := `01-Sep-2026 08:07:28 %BRG_MACNTFY-I-MAC_FLAPPING: Host 52:54:4c:83:09:e0 in vlan 1 is flapping between port gi1/0/23 and port gi1/0/10`
	msg, ok := ParseMACFlapping(StripSyslogHeader(raw))
	if !ok {
		t.Fatal("parse failed")
	}
	if msg.MAC != "52:54:4c:83:09:e0" {
		t.Fatalf("mac=%q", msg.MAC)
	}
	if msg.VLAN == nil || *msg.VLAN != 1 {
		t.Fatalf("vlan=%v", msg.VLAN)
	}
	if msg.PortA != "gi1/0/23" || msg.PortB != "gi1/0/10" {
		t.Fatalf("ports %q / %q", msg.PortA, msg.PortB)
	}
}

func TestStripSyslogPRI(t *testing.T) {
	raw := `<134>Sep  1 08:07:28 sw38 %BRG_MACNTFY-I-MAC_FLAPPING: Host 52:54:4c:83:09:e0 in vlan 1 is flapping between port gi1/0/23 and port gi1/0/10`
	body := StripSyslogHeader(raw)
	msg, ok := ParseMACFlapping(body)
	if !ok {
		t.Fatalf("body=%q", body)
	}
	if msg.MAC != "52:54:4c:83:09:e0" {
		t.Fatalf("mac=%q", msg.MAC)
	}
}
