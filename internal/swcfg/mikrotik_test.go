package swcfg

import (
	"strings"
	"testing"
)

func TestDetectVendorMikrotik(t *testing.T) {
	if DetectVendor("", "RouterOS CCR1009", "CCR1009-SkyNet") != VendorMikrotik {
		t.Fatal("ccr")
	}
	if DetectVendor("mikrotik", "", "") != VendorMikrotik {
		t.Fatal("explicit")
	}
	if DetectVendor("", "MikroTik CRS326", "crs-core") != VendorMikrotik {
		t.Fatal("crs")
	}
	if !SupportsPortCLI(VendorMikrotik, "RouterOS", "CCR") {
		t.Fatal("supports cli")
	}
}

func TestMikrotikPortCmds(t *testing.T) {
	up := true
	desc := `uplink "core"`
	cmds, err := MikrotikPortCmds("sfp-sfpplus1", PortChange{AdminUp: &up, Description: &desc})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 2 {
		t.Fatalf("want 2 cmds, got %#v", cmds)
	}
	if !strings.Contains(cmds[0], "/interface set") ||
		!strings.Contains(cmds[0], `name="sfp-sfpplus1"`) ||
		!strings.Contains(cmds[0], "disabled=no") {
		t.Fatalf("admin: %s", cmds[0])
	}
	if !strings.Contains(cmds[1], "comment=") || !strings.Contains(cmds[1], `\"core\"`) {
		t.Fatalf("comment: %s", cmds[1])
	}
	dollar := "cost $wan"
	cmds, err = MikrotikPortCmds("ether1", PortChange{Description: &dollar})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 || !strings.Contains(cmds[0], `\$wan`) || strings.Contains(cmds[0], `comment="$wan"`) {
		t.Fatalf("dollar: %s", cmds[0])
	}
	off := "off"
	cmds, err = MikrotikPortCmds("ether5", PortChange{PoEMode: &off})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 1 || !strings.Contains(cmds[0], "poe-out=off") || !strings.Contains(cmds[0], "ether5") {
		t.Fatalf("poe: %#v", cmds)
	}
	iso := true
	if _, err := MikrotikPortCmds("ether1", PortChange{Isolate: &iso}); err == nil {
		t.Fatal("isolate should fail")
	}
}

func TestDetectVendorStillUbntEltex(t *testing.T) {
	if DetectVendor("auto", "EdgeSwitch 24", "sw") != VendorUbiquiti {
		t.Fatal("ubnt")
	}
	if DetectVendor("", "Eltex MES2324", "") != VendorEltex {
		t.Fatal("eltex")
	}
}
