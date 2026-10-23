package snmp

import "testing"

func TestResolveFDBIfIndex_SNRUsesIfIndex(t *testing.T) {
	// SNR-S5010: bridge 1..10 → ifIndex 5001..5010 (ge1..ge10).
	baseToIf := map[int]int{}
	for bp := 1; bp <= 10; bp++ {
		baseToIf[bp] = 5000 + bp
	}
	known := fdbKnownIfIndexes(baseToIf)

	// Классика: значение = bridge-port.
	if got := resolveFDBIfIndex(1, baseToIf, known); got != 5001 {
		t.Fatalf("bridge 1 → ge1: got %d", got)
	}
	if got := resolveFDBIfIndex(8, baseToIf, known); got != 5008 {
		t.Fatalf("bridge 8 → ge8: got %d", got)
	}
	if got := resolveFDBIfIndex(9, baseToIf, known); got != 5009 {
		t.Fatalf("bridge 9 → ge9: got %d", got)
	}

	// SNR: значение уже ifIndex.
	if got := resolveFDBIfIndex(5001, baseToIf, known); got != 5001 {
		t.Fatalf("ifIndex 5001 (ge1): got %d", got)
	}
	if got := resolveFDBIfIndex(5008, baseToIf, known); got != 5008 {
		t.Fatalf("ifIndex 5008 (ge8): got %d", got)
	}
	if got := resolveFDBIfIndex(5009, baseToIf, known); got != 5009 {
		t.Fatalf("ifIndex 5009 (ge9): got %d", got)
	}
	if got := resolveFDBIfIndex(9999, baseToIf, known); got != 0 {
		t.Fatalf("unknown port must be 0, got %d", got)
	}
}

func TestParseVLANAndMACSuffix(t *testing.T) {
	base := oidDot1qTpFdbPort
	tests := []struct {
		name    string
		oid     string
		wantV   int
		wantMAC string
		wantErr bool
	}{
		{
			name:    "edgeswitch vlan+mac",
			oid:     base + ".1.0.17.50.93.74.225",
			wantV:   1,
			wantMAC: "00:11:32:5d:4a:e1",
		},
		{
			name:    "qbridge vlan+fdb+mac",
			oid:     base + ".10.0.0.26.32.46.58.70",
			wantV:   10,
			wantMAC: "00:1a:20:2e:3a:46",
		},
		{
			name:    "too short",
			oid:     base + ".1.0.17.50.93.74",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, mac, err := parseVLANAndMACSuffix(tt.oid, base)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if v != tt.wantV || mac != tt.wantMAC {
				t.Fatalf("got vlan=%d mac=%q want vlan=%d mac=%q", v, mac, tt.wantV, tt.wantMAC)
			}
		})
	}
}
