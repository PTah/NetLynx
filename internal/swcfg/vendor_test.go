package swcfg

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestPreferBusyboxFastpath(t *testing.T) {
	c := Creds{Name: "EdgeSwitch 16", SysDescr: "EdgeSwitch 16-Port"}
	if preferBusybox(c, VendorUbiquiti) {
		t.Fatal("fastpath should use CLI, not busybox exec")
	}
	linux := Creds{Name: "ES-5XP", SysDescr: "Linux EdgeSwitch"}
	if !preferBusybox(linux, VendorUbiquiti) {
		t.Fatal("linux edgeswitch may use busybox")
	}
	xp := Creds{Name: "EdgeSwitch 5XP PoE #2 (Админы)", SysDescr: ""}
	if !isEdgeSwitchXP(xp.SysDescr, xp.Name) || !preferBusybox(xp, VendorUbiquiti) {
		t.Fatal("5XP is BusyBox, not Fastpath CLI")
	}
	if preferBusybox(Creds{Name: "EdgeSwitch 48 Lite"}, VendorUbiquiti) {
		t.Fatal("ES-48 is Fastpath")
	}
	fastpathLinux := Creds{
		Name:     "EdgeSwitch 16 #22 (Parkovka)",
		SysDescr: "EdgeSwitch 16-Port, 1.9.3-lite, Linux 3.6.5-03329b4a, 1.1.0.5102011",
	}
	if preferBusybox(fastpathLinux, VendorUbiquiti) {
		t.Fatal("Fastpath sysDescr contains Linux but must use CLI, not exec/cat")
	}
}

func TestLooksLikeBusyboxShell(t *testing.T) {
	s := "BusyBox v1.11.2 built-in shell (ash) SW.v2.1.0# -sh: en: not found"
	if !looksLikeBusyboxShell(s) {
		t.Fatal("banner")
	}
	if looksLikeBusyboxShell("!Current Configuration:\nhostname switch") {
		t.Fatal("fastpath")
	}
}

func TestLooksLikeAirOSCfg(t *testing.T) {
	cfg := "bridge.status=enabled\nusers.1.name=ubnt\nhttpd.https.status=enabled\n"
	if !looksLikeConfig(cfg) {
		t.Fatal("system.cfg")
	}
}

func TestRedactSecrets(t *testing.T) {
	got := compactCLIErr("-sh: SecretPass: not found", "SecretPass")
	if strings.Contains(got, "SecretPass") {
		t.Fatal(got)
	}
}

func TestHostKeySameTypeMismatchEmptyWant(t *testing.T) {
	if !hostKeySameTypeMismatch(nil, nil) {
		t.Fatal("nil key should reject")
	}
	if hostKeySameTypeMismatch(nil, dummyPubKey{}) {
		t.Fatal("new type with no known keys should not count as same-type mismatch")
	}
}

func TestIsLocalPortDescrOnly(t *testing.T) {
	cases := []struct {
		descr, name string
		extra       []string
		want        bool
	}{
		{descr: "", name: "EdgeSwitch 5XP PoE #2", want: true},
		{descr: "EdgeSwitch 16-Port, 1.9.3-lite", name: "ES-16 #22", want: false},
		{descr: "USW-24-PoE, 6.6.61", name: "Office-USW", want: true},
		{descr: "UniFi Switch 8 POE-60W", name: "us-8-lobby", want: true},
		{descr: "Linux UniFi-USW-Lite-8-PoE", name: "usw-lite-8", want: true},
		{descr: "UAP-AC-Pro", name: "ap-hall", want: true},
		{descr: "EdgeSwitch 24 Lite", name: "sw-core", want: false},
		{descr: "SNR-S2989G", name: "sw", want: false},
		// Кастомное имя без модели — ловим по sysName / баннеру в extra.
		{descr: "Linux 3.6.5", name: "свитч кабинет", extra: []string{"US-8-150W"}, want: true},
		{descr: "", name: "sw1", extra: []string{"Welcome to UniFi US-8-150W"}, want: true},
	}
	for _, tc := range cases {
		if got := IsLocalPortDescrOnly(tc.descr, tc.name, tc.extra...); got != tc.want {
			t.Fatalf("%q / %q / %v: got %v want %v", tc.descr, tc.name, tc.extra, got, tc.want)
		}
	}
	if IsUniFiNetworkDevice("EdgeSwitch 16-Port", "sw") {
		t.Fatal("EdgeSwitch must not be UniFi")
	}
	if !IsUniFiNetworkDevice("USW-Lite-8-PoE", "sw1") {
		t.Fatal("USW should be UniFi")
	}
}

func TestPortDescrPushImpliesLocalOnly(t *testing.T) {
	errBusy := fmt.Errorf("не удалось записать описание на свитч (SNMP: NoAccess; SSH: cli: нет признаков configure/interface — BusyBox v1.25.1 Welcome to UniFi US-8-150W! www.ui.com)")
	if !PortDescrPushImpliesLocalOnly(errBusy) {
		t.Fatal("UniFi BusyBox push fail must imply local")
	}
	errEdge := fmt.Errorf("SSH: Invalid input on EdgeSwitch 16")
	if PortDescrPushImpliesLocalOnly(errEdge) {
		t.Fatal("EdgeSwitch Fastpath fail must not imply local")
	}
}

func TestDetectVendor(t *testing.T) {
	if DetectVendor("eltex", "", "x") != VendorEltex {
		t.Fatal("explicit")
	}
	if DetectVendor("auto", "EdgeSwitch 24", "sw") != VendorUbiquiti {
		t.Fatal("ubnt")
	}
	if DetectVendor("", "Eltex MES2324", "") != VendorEltex {
		t.Fatal("eltex descr")
	}
	if DetectVendor("", "SNR-S2989G", "") != VendorSNR {
		t.Fatal("snr")
	}
}

type dummyPubKey struct{}

func (dummyPubKey) Type() string { return "ssh-rsa" }
func (dummyPubKey) Marshal() []byte { return []byte("x") }
func (dummyPubKey) Verify([]byte, *ssh.Signature) error { return nil }
