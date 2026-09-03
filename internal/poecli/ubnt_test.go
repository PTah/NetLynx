package poecli

import (
	"strings"
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
)

func TestSelectSSHVendorProfile(t *testing.T) {
	p := selectSSHVendorProfile("EdgeSwitch 16 150W, 1.12.0")
	if p.name != "ubiquiti" || p.privilegeCmd != "en" {
		t.Fatalf("unexpected ubiquiti profile: %+v", p)
	}
	p = selectSSHVendorProfile("Cisco IOS XE Software")
	if p.name != "cisco" || p.privilegeCmd != "enable" {
		t.Fatalf("unexpected cisco profile: %+v", p)
	}
	p = selectSSHVendorProfile("SNR-S2989G-48TX-POE Device, NAG LLC")
	if p.name != "snr" || p.privilegeCmd != "" {
		t.Fatalf("unexpected snr profile: %+v", p)
	}
}

func TestParseUbiquitiShowPoeStatusToIfIndex(t *testing.T) {
	ifRows := map[int]snmp.IfRow{
		9:  {IfIndex: 9, IfName: "0/1"},
		10: {IfIndex: 10, IfName: "0/2"},
		11: {IfIndex: 11, IfName: "0/3"},
	}
	out := `
Port  Admin  Oper  Power  Device
0/1   Auto   Delivering  6.5W   AP
0/2   Auto   Searching   0.0W   --
0/3   Auto   On          2.1W   Phone
`
	got := parseUbiquitiShowPoeStatusToIfIndex(out, ifRows)
	if !got[9] {
		t.Fatalf("expected ifIndex 9 active")
	}
	if got[10] {
		t.Fatalf("expected ifIndex 10 inactive")
	}
	if !got[11] {
		t.Fatalf("expected ifIndex 11 active")
	}
}

func TestParseUbiquitiShowPoeStatusAllFormat(t *testing.T) {
	ifRows := map[int]snmp.IfRow{
		21: {IfIndex: 21, IfName: "0/3"},
		22: {IfIndex: 22, IfName: "0/4"},
		23: {IfIndex: 23, IfName: "0/15"},
	}
	out := `
Intf      Detection      Class   Consumed(W) Voltage(V) Current(mA) Consumed Meter(Whr) Temperature(C)
--------- -------------- ------- ----------- ---------- ----------- ------------------ --------------
0/3       Good           Class3         2.31      53.33       43.45               0.65             35
0/4       Short          Unknown        0.00       0.00        0.00               0.00             35
0/15      Good           Class4         6.93      53.46      129.63               1.95             31
`
	got := parseUbiquitiShowPoeStatusToIfIndex(out, ifRows)
	if !got[21] {
		t.Fatalf("expected ifIndex 21 active (0/3)")
	}
	if got[22] {
		t.Fatalf("expected ifIndex 22 inactive (0/4)")
	}
	if !got[23] {
		t.Fatalf("expected ifIndex 23 active (0/15)")
	}
}

func TestParseSNRShowPowerInlineFormat(t *testing.T) {
	ifRows := map[int]snmp.IfRow{
		37: {IfIndex: 37, IfName: "Ethernet1/0/37"},
		38: {IfIndex: 38, IfName: "Ethernet1/0/38"},
	}
	out := `
Interface       Status  Oper   Power(mW) Max-type Max(mW) Current(mA) Volt(V) Priority Class
--------------- ------- ------ --------- -------- ------- ----------- ------- -------- -----
Ethernet1/0/37   enable     on      2900    class   33000          48      55      low     3
Ethernet1/0/38   enable    off         0    class   33000           0       0      low     0
`
	got := parseUbiquitiShowPoeStatusToIfIndex(out, ifRows)
	if !got[37] {
		t.Fatalf("expected ifIndex 37 active (Ethernet1/0/37)")
	}
	if got[38] {
		t.Fatalf("expected ifIndex 38 inactive (Ethernet1/0/38)")
	}
}

func TestIsPoEActiveFromDetailedColumns_IgnoresVoltageAlone(t *testing.T) {
	if isPoEActiveFromDetailedColumns("searching", 0, 54.0, 0) {
		t.Fatal("voltage alone must not mark PoE active")
	}
	if !isPoEActiveFromDetailedColumns("good", 0, 0, 0) {
		t.Fatal("detection Good must be active")
	}
	if !isPoEActiveFromDetailedColumns("searching", 1.2, 0, 0) {
		t.Fatal("consumedW > 0 must be active")
	}
}

func TestStripPoePager(t *testing.T) {
	in := "0/20      Good           Class0         1.59      53.08       30.02             230.61             45\n--More-- or (q)uit\n0/21      Short          Unknown        0.00       0.00        0.00               0.00             41\n"
	got := stripPoePager(in)
	if strings.Contains(strings.ToLower(got), "more") {
		t.Fatalf("pager left: %q", got)
	}
	if !strings.Contains(got, "0/21") {
		t.Fatalf("expected 0/21 kept: %q", got)
	}
}
