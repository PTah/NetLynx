package swcfg

import (
	"strings"
	"testing"
)

func TestSTPCLILinesUbiquiti(t *testing.T) {
	en := true
	ep := "auto"
	prio := 128
	cost := 0
	lines, err := STPCLILines(VendorUbiquiti, STPChange{
		Enabled: &en, EdgePort: &ep, PortPriority: &prio, PathCost: &cost,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"spanning-tree port mode", "spanning-tree edgeport auto", "spanning-tree port-priority 128", "no spanning-tree cost"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, lines)
		}
	}
}

func TestSTPCLILinesEltex(t *testing.T) {
	off := false
	lines, err := STPCLILines(VendorEltex, STPChange{Enabled: &off})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0] != "spanning-tree disable" {
		t.Fatalf("%v", lines)
	}
}

func TestPoEModeCLIEltex(t *testing.T) {
	got, err := PoEModeCLI(VendorEltex, "off")
	if err != nil || got != "power inline never" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := PoEModeCLI(VendorEltex, "24v"); err == nil {
		t.Fatal("expected 24v error")
	}
}
