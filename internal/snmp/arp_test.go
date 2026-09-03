package snmp

import "testing"

func TestParseIpNetToMediaIndex(t *testing.T) {
	base := oidIpNetToMediaPhysAddress
	ifIdx, ip, err := parseIpNetToMediaIndex(base+".12.192.168.1.50", base)
	if err != nil {
		t.Fatal(err)
	}
	if ifIdx != 12 || ip != "192.168.1.50" {
		t.Fatalf("got if=%d ip=%s", ifIdx, ip)
	}
	if _, _, err := parseIpNetToMediaIndex(base+".1.10.0.0", base); err == nil {
		t.Fatal("expected short index error")
	}
	if _, _, err := parseIpNetToMediaIndex(base+".3.10.0.0.256", base); err == nil {
		t.Fatal("expected invalid octet")
	}
}

func TestNormalizeARPMac(t *testing.T) {
	if got := normalizeARPMac("10-FF-E0-65-58-22"); got != "10:ff:e0:65:58:22" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeARPMac("10ffe0655822"); got != "10:ff:e0:65:58:22" {
		t.Fatalf("got %q", got)
	}
}
