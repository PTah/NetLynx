package swcfg

import "testing"

func TestUbiquitiDHCPTrustCLI(t *testing.T) {
	if got := UbiquitiDHCPTrustCLI(true); got != "ip dhcp snooping trust" {
		t.Fatalf("on: %q", got)
	}
	if got := UbiquitiDHCPTrustCLI(false); got != "no ip dhcp snooping trust" {
		t.Fatalf("off: %q", got)
	}
}

func TestCountDHCPSnoopingTrustLines(t *testing.T) {
	cfg := `
ip dhcp snooping
interface 0/1
ip dhcp snooping trust
exit
interface 0/2
description cam
exit
interface 0/17
ip dhcp snooping trust
exit
`
	if n := CountDHCPSnoopingTrustLines(cfg); n != 2 {
		t.Fatalf("got %d want 2", n)
	}
	if n := CountDHCPSnoopingTrustLines("no ip dhcp snooping\n"); n != 0 {
		t.Fatalf("got %d", n)
	}
}
