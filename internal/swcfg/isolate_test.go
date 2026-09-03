package swcfg

import (
	"strings"
	"testing"
)

func TestUbiquitiIsolateCLI(t *testing.T) {
	if got := UbiquitiIsolateCLI(true); got != "switchport protected 1" {
		t.Fatalf("on: %q", got)
	}
	if got := UbiquitiIsolateCLI(false); got != "no switchport protected" {
		t.Fatalf("off: %q", got)
	}
}

func TestPortConfigBodyIsolate(t *testing.T) {
	on := true
	steps, err := portConfigBody(VendorUbiquiti, "0/5", PortChange{Interface: "0/5", Isolate: &on, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(steps, "\n")
	if !strings.Contains(joined, "switchport protected 1") {
		t.Fatalf("missing isolate on: %v", steps)
	}
	off := false
	steps, err = portConfigBody(VendorUbiquiti, "0/5", PortChange{Interface: "0/5", Isolate: &off})
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(steps, "\n")
	if !strings.Contains(joined, "no switchport protected") {
		t.Fatalf("missing isolate off: %v", steps)
	}
}
