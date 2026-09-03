package snmp

import "testing"

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
