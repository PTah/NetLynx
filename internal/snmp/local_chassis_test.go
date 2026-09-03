package snmp

import "testing"

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
