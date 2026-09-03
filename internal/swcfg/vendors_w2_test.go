package swcfg

import (
	"strings"
	"testing"
)

func TestDetectVendorW2(t *testing.T) {
	cases := []struct {
		descr, name string
		want        Vendor
	}{
		{"HP ProCurve Switch 2920", "pc-core", VendorHP},
		{"Hewlett-Packard J9727A", "hp1", VendorHP},
		{"TP-LINK JetStream T1600G-28PS", "tpl", VendorTPLink},
		{"TL-SG3428 JetStream", "sg", VendorTPLink},
		{"D-Link DGS-1210-28P", "dgs", VendorDLink},
		{"DES-3200-28 Fast Ethernet Switch", "des", VendorDLink},
		{"Dahua DH-PFS4218-16ET-190", "pfs", VendorDahua},
		{"Hikvision DS-3E1526P-EI", "hik-sw", VendorHikvision},
		{"HiWatch DS-3E0318P-E", "hiw", VendorHiWatch},
		{"Trassir Network Switch TR-NS24P", "tr-sw", VendorTrassir},
	}
	for _, tc := range cases {
		if got := DetectVendor("", tc.descr, tc.name); got != tc.want {
			t.Fatalf("%s / %s: got %s want %s", tc.descr, tc.name, got, tc.want)
		}
		if !SupportsPortCLI(tc.want, tc.descr, tc.name) {
			t.Fatalf("%s should support port CLI", tc.want)
		}
	}
}

func TestVideoCameraNotSwitch(t *testing.T) {
	if LooksLikeVideoLANSwitch("Hikvision DS-2CD2143G0-I", "cam1") {
		t.Fatal("camera must not look like switch")
	}
	if !LooksLikeIPCamera("Hikvision DS-2CD2143G0-I", "cam1") {
		t.Fatal("camera expected")
	}
	if DetectVendor("", "Hikvision DS-2CD2143G0-I", "cam1") != VendorAuto {
		t.Fatal("IPC should not map to hikvision switch vendor")
	}
	if !LooksLikeVideoLANSwitch("Hikvision DS-3E1526P-EI Switch", "sw") {
		t.Fatal("DS-3E is switch")
	}
	if LooksLikeIPCamera("Hikvision DS-3E1526P-EI Switch", "sw") {
		t.Fatal("switch must not look like camera")
	}
}

func TestW2PoEAndBackupCmds(t *testing.T) {
	cmd, err := PoEModeCLI(VendorTPLink, "poe+")
	if err != nil || cmd != "power inline supply" {
		t.Fatalf("tplink poe: %v %q", err, cmd)
	}
	cmd, err = PoEModeCLI(VendorDLink, "off")
	if err != nil || cmd != "no power inline enable" {
		t.Fatalf("dlink poe: %v %q", err, cmd)
	}
	cmd, err = PoEModeCLI(VendorHP, "poe+")
	if err != nil || cmd != "power-over-ethernet" {
		t.Fatalf("hp poe: %v %q", err, cmd)
	}
	cmd, err = PoEModeCLI(VendorDahua, "poe+")
	if err != nil || cmd != "poe enable" {
		t.Fatalf("dahua poe: %v %q", err, cmd)
	}

	up := false
	body, err := portConfigBody(VendorTPLink, "gigabitEthernet 1/0/1", PortChange{AdminUp: &up, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(body, "\n")
	for _, need := range []string{"interface gigabitEthernet 1/0/1", "shutdown", "write memory"} {
		if !strings.Contains(joined, need) {
			t.Fatalf("missing %q in %s", need, joined)
		}
	}

	iso := true
	if _, err := portConfigBody(VendorHP, "1", PortChange{Isolate: &iso}); err == nil {
		t.Fatal("hp isolate should fail for now")
	}
}
