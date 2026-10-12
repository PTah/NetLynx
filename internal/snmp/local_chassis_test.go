package snmp

import (
	"fmt"
	"testing"
)

func TestNormalizeChassisMAC(t *testing.T) {
	if got := normalizeChassisMAC("AA-BB-CC-DD-EE-FF"); got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeChassisMAC("aabbccddeeff"); got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("compact: %q", got)
	}
	if got := normalizeChassisMAC("192.168.1.1"); got != "" {
		t.Fatalf("IP must not be MAC: %q", got)
	}
	if got := normalizeChassisMAC(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
}

func TestParseIPv4Host(t *testing.T) {
	if got := parseIPv4Host("192.168.128.5"); got == nil || got.String() != "192.168.128.5" {
		t.Fatalf("got %v", got)
	}
	if parseIPv4Host("papasha") != nil {
		t.Fatal("hostname must be nil")
	}
	if parseIPv4Host("") != nil {
		t.Fatal("empty")
	}
	if parseIPv4Host("2001:db8::1") != nil {
		t.Fatal("IPv6 not supported for this fallback")
	}
}

func TestPhysAddressOIDForHost(t *testing.T) {
	ip := parseIPv4Host("192.168.128.5")
	if ip == nil {
		t.Fatal("parse")
	}
	want := "1.3.6.1.2.1.4.20.1.2.192.168.128.5"
	got := fmt.Sprintf("%s.%d.%d.%d.%d", oidIpAdEntIfIndex, ip[0], ip[1], ip[2], ip[3])
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMacFromBytesRejectsASCII(t *testing.T) {
	// "0/24" as 4 bytes is not 6 — macFromBytes returns ""
	if macFromBytes([]byte("0/24")) != "" {
		t.Fatal("4-byte ASCII should not be MAC")
	}
	raw := []byte{0xf0, 0x9f, 0xc2, 0x64, 0xfe, 0x5c}
	if got := macFromBytes(raw); got != "f0:9f:c2:64:fe:5c" {
		t.Fatalf("got %q", got)
	}
}
