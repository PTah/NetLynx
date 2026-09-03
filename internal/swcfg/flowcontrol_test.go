package swcfg

import (
	"strings"
	"testing"
)

func TestUbiquitiFlowControlCLI(t *testing.T) {
	if got := UbiquitiFlowControlCLI(true); got != "flowcontrol" {
		t.Fatalf("on: %q", got)
	}
	if got := UbiquitiFlowControlCLI(false); got != "no flowcontrol" {
		t.Fatalf("off: %q", got)
	}
}

func TestPortConfigBodyFlowControl(t *testing.T) {
	on := true
	steps, err := portConfigBody(VendorUbiquiti, "0/4", PortChange{Interface: "0/4", FlowControl: &on, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(steps, "\n")
	if !strings.Contains(joined, "flowcontrol") || strings.Contains(joined, "no flowcontrol") {
		t.Fatalf("on: %v", steps)
	}
}
