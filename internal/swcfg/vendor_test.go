package swcfg

import (
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
