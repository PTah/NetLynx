package topologycache

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestBlastEdgeProtocolOK(t *testing.T) {
	ok := []store.TopologyEdge{
		{Protocol: "lldp"},
		{Protocol: "cdp"},
		{Protocol: "manual"},
		{Protocol: "fdb", Protocols: []string{"lldp"}},
		{Protocol: "LLDP"},
	}
	for _, e := range ok {
		if !blastEdgeProtocolOK(e) {
			t.Fatalf("expected ok: %+v", e)
		}
	}
	bad := []store.TopologyEdge{
		{Protocol: "fdb"},
		{Protocol: ""},
		{Protocol: "other"},
	}
	for _, e := range bad {
		if blastEdgeProtocolOK(e) {
			t.Fatalf("expected reject: %+v", e)
		}
	}
}

func TestIfIndexFromName(t *testing.T) {
	if ifIndexFromName("0/14") != 14 {
		t.Fatal("0/14")
	}
	if ifIndexFromName("14") != 14 {
		t.Fatal("14")
	}
	if ifIndexFromName("") != 0 {
		t.Fatal("empty")
	}
}
