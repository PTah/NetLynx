package snmp

import (
	"net"
	"testing"
)

func TestDecodeCDPAddress(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"raw ipv4", []byte{10, 1, 2, 3}, "10.1.2.3"},
		{"type1 ipv4", []byte{1, 192, 168, 1, 10}, "192.168.1.10"},
		{"empty", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeCDPAddress(tt.in)
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
			if tt.want != "" && net.ParseIP(got) == nil {
				t.Fatalf("not a valid IP: %q", got)
			}
		})
	}
}
