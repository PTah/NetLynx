package snmp

import "testing"

func TestResolveLLDPLocalPortNumToIfIndex(t *testing.T) {
	rows := map[int]IfRow{
		9:  {IfIndex: 9, IfName: "0/1", Oper: 1},
		16: {IfIndex: 16, IfName: "0/16", Oper: 1},
		66: {IfIndex: 66, IfName: "lag0", Oper: 1},
	}
	tests := []struct {
		name     string
		locPort  int
		portToIf map[int]int
		want     int
	}{
		{"from loc table", 5, map[int]int{5: 9}, 9},
		{"direct ifIndex", 16, nil, 16},
		{"guess from ifName 0/1", 1, nil, 9},
		{"unknown", 99, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveLLDPLocalPortNumToIfIndex(tt.locPort, tt.portToIf, rows)
			if got != tt.want {
				t.Fatalf("locPort=%d got ifIndex=%d want %d", tt.locPort, got, tt.want)
			}
		})
	}
}
