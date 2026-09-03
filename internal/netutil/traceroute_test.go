package netutil

import (
	"strings"
	"testing"
)

func TestParseTracerouteOutput(t *testing.T) {
	raw := `
traceroute to 192.168.160.38 (192.168.160.38), 15 hops max, 60 byte packets
 1  192.168.160.1  0.456 ms
 2  192.168.160.38  1.234 ms
 3  * * *
`
	hops := parseTracerouteOutput(raw)
	if len(hops) != 3 {
		t.Fatalf("expected 3 hops, got %d", len(hops))
	}
	if hops[0].Address != "192.168.160.1" || len(hops[0].RTTMs) != 1 {
		t.Fatalf("hop1: %+v", hops[0])
	}
	if hops[2].Timeout != true {
		t.Fatalf("hop3 timeout: %+v", hops[2])
	}
}

func TestParseTracepathOutput(t *testing.T) {
	raw := `
 1:  gateway (192.168.160.1)                     0.123ms
 2:  sw38 (192.168.160.38)                       1.456ms reached
`
	hops := parseTracepathOutput(raw)
	if len(hops) != 2 {
		t.Fatalf("expected 2 hops, got %d", len(hops))
	}
	if hops[1].Address != "192.168.160.38" {
		t.Fatalf("hop2: %+v", hops[1])
	}
}

func TestValidateProbeTarget_blocksLoopback(t *testing.T) {
	if err := ValidateProbeTarget(nil, "127.0.0.1"); err == nil {
		t.Fatal("expected block loopback")
	}
	if err := ValidateProbeTarget(nil, "192.168.1.10"); err != nil {
		t.Fatalf("LAN ok: %v", err)
	}
}

func TestNormalizeProbeHost(t *testing.T) {
	if got := normalizeProbeHost(" 192.168.1.1:161 "); got != "192.168.1.1" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeProbeHost("[2001:db8::1]"); got != "2001:db8::1" {
		t.Fatalf("got %q", got)
	}
}

func TestParseTracertOutput(t *testing.T) {
	raw := strings.ReplaceAll(`
  1     1 ms     1 ms     1 ms  192.168.1.1
  2     *        *        *     Request timed out.
`, "\n", "\r\n")
	hops := parseTracertOutput(raw)
	if len(hops) < 1 || hops[0].Address != "192.168.1.1" {
		t.Fatalf("%+v", hops)
	}
}
