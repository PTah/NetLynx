package swcfg

import "testing"

func TestParseInterfaceConfigSnippet(t *testing.T) {
	out := `
interface 0/5
description CAM-01
shutdown
switchport protected 1
poe opmode shutdown
spanning-tree edgeport auto
spanning-tree port-priority 128
exit
`
	s := ParseInterfaceConfigSnippet(out)
	if s.AdminUp {
		t.Fatal("expected shutdown → admin down")
	}
	if !s.Isolate {
		t.Fatal("expected isolate")
	}
	if s.PoEMode != "off" {
		t.Fatalf("poe: %q", s.PoEMode)
	}
	if s.DHCPTrusted {
		t.Fatal("expected no dhcp trust")
	}
	if s.EdgePort != "auto" {
		t.Fatalf("edge: %q", s.EdgePort)
	}

	out2 := `
interface 0/1
description uplink
poe opmode passive24v
ip dhcp snooping trust
flowcontrol
spanning-tree portfast
exit
`
	s2 := ParseInterfaceConfigSnippet(out2)
	if !s2.AdminUp || s2.Isolate || s2.PoEMode != "24v" || !s2.DHCPTrusted || !s2.FlowControl {
		t.Fatalf("%+v", s2)
	}
	if s2.EdgePort != "enable" {
		t.Fatalf("edge: %q", s2.EdgePort)
	}

	s3 := ParseInterfaceConfigSnippet("interface gi1/0/2\npower inline never\nswitchport protected-port\nspanning-tree disable\nexit\n")
	if s3.PoEMode != "off" || !s3.Isolate || s3.STPEnabled {
		t.Fatalf("%+v", s3)
	}

	// Eltex: show running-config interfaces gi1/0/10 может вернуть хвост конфига —
	// следующий interface с no shutdown не должен перезаписывать shutdown текущего.
	out4 := `
interface gi1/0/10
 shutdown
 description cameras
exit
interface gi1/0/11
 no shutdown
exit
`
	s4 := ParseInterfaceConfigSnippet(out4)
	if s4.AdminUp {
		t.Fatal("expected shutdown on gi1/0/10, must not bleed from gi1/0/11")
	}

	out5 := `
interface 0/1
no shutdown
exit
interface 0/9
shutdown
description counter
exit
`
	s5 := ParseInterfaceConfigSnippet(out5, "0/9")
	if s5.AdminUp {
		t.Fatal("wantIface 0/9 must be shutdown")
	}
	s5b := ParseInterfaceConfigSnippet(out5, "0/1")
	if !s5b.AdminUp {
		t.Fatal("wantIface 0/1 must be up")
	}
}

func TestParseInterfaceFromRunningConfig(t *testing.T) {
	full := `
interface 0/1
no shutdown
exit
interface 0/9
shutdown
switchport protected 1
poe opmode shutdown
exit
`
	s, ok := ParseInterfaceFromRunningConfig(full, "0/9")
	if !ok {
		t.Fatal("expected 0/9 block")
	}
	if s.AdminUp || !s.Isolate || s.PoEMode != "off" {
		t.Fatalf("%+v", s)
	}
	if _, ok := ParseInterfaceFromRunningConfig(full, "0/99"); ok {
		t.Fatal("missing interface must not match")
	}
	// 0/1 не должен совпасть с 0/10
	full2 := "interface 0/10\nshutdown\nexit\ninterface 0/1\nno shutdown\nexit\n"
	s2, ok := ParseInterfaceFromRunningConfig(full2, "0/1")
	if !ok || !s2.AdminUp {
		t.Fatalf("0/1 vs 0/10: ok=%v admin=%v", ok, s2.AdminUp)
	}
}
