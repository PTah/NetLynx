package swcfg

import (
	"strings"
	"testing"
)

func TestDetectVendorW1(t *testing.T) {
	cases := []struct {
		descr, name string
		want        Vendor
	}{
		{"Cisco IOS Software, C2960", "sw1", VendorCisco},
		{"Cisco NX-OS", "n9k", VendorCisco},
		{"Aruba JL725A", "core-aruba", VendorAruba},
		{"HP ProCurve Switch", "pc", VendorHP},
		{"Zyxel XGS4600", "zy", VendorZyxel},
		{"Huawei Versatile Routing Platform Software", "s5720", VendorHuawei},
		{"RouterOS CCR1009", "CCR", VendorMikrotik},
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

func TestCiscoLikePortBody(t *testing.T) {
	up := false
	desc := "uplink"
	body, err := portConfigBody(VendorCisco, "GigabitEthernet1/0/1", PortChange{
		AdminUp:     &up,
		Description: &desc,
		Write:       true,
	})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(body, "\n")
	for _, need := range []string{"interface GigabitEthernet1/0/1", "description uplink", "shutdown", "exit", "write memory"} {
		if !strings.Contains(joined, need) {
			t.Fatalf("missing %q in %s", need, joined)
		}
	}
	priv, conf, write := confEnterCmds(VendorCisco)
	if priv != "enable" || conf != "configure terminal" || write != "write memory" {
		t.Fatalf("cisco enter: %q %q %q", priv, conf, write)
	}
}

func TestHuaweiPortBody(t *testing.T) {
	up := true
	body, err := portConfigBody(VendorHuawei, "GigabitEthernet0/0/1", PortChange{AdminUp: &up, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(body, "\n")
	if !strings.Contains(joined, "undo shutdown") || !strings.Contains(joined, "quit") || !strings.Contains(joined, "return") {
		t.Fatalf("huawei body: %s", joined)
	}
	priv, conf, write := confEnterCmds(VendorHuawei)
	if priv != "" || conf != "system-view" || write != "save" {
		t.Fatalf("huawei enter: %q %q %q", priv, conf, write)
	}
	cmd, err := PoEModeCLI(VendorHuawei, "poe+")
	if err != nil || cmd != "poe enable" {
		t.Fatalf("huawei poe: %v %q", err, cmd)
	}
}

func TestArubaZyxelPoE(t *testing.T) {
	cmd, err := PoEModeCLI(VendorAruba, "off")
	if err != nil || cmd != "no power-over-ethernet" {
		t.Fatalf("aruba: %v %q", err, cmd)
	}
	cmd, err = PoEModeCLI(VendorZyxel, "poe+")
	if err != nil || cmd != "poe mode auto" {
		t.Fatalf("zyxel: %v %q", err, cmd)
	}
	iso := true
	if _, err := portConfigBody(VendorAruba, "1/1/1", PortChange{Isolate: &iso}); err == nil {
		t.Fatal("aruba isolate should fail")
	}
}
