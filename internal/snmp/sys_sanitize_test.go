package snmp

import "testing"

func TestSanitizeSNMPBytes(t *testing.T) {
	mac := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}
	if got := sanitizeSNMPBytes(mac); got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("mac: %q", got)
	}
	if got := sanitizeSNMPBytes([]byte("SW-1")); got != "SW-1" {
		t.Fatalf("ascii: %q", got)
	}
	// EdgeSwitch ifName "0/10"/"0/24" — 4 ASCII-байта, НЕ IPv4 48.47.x.x
	if got := sanitizeSNMPBytes([]byte("0/10")); got != "0/10" {
		t.Fatalf("ifName 0/10: %q", got)
	}
	if got := sanitizeSNMPBytes([]byte("0/24")); got != "0/24" {
		t.Fatalf("ifName 0/24: %q", got)
	}
	// Сырой IPv4 (непечатные октеты) по-прежнему как dotted-quad
	if got := sanitizeSNMPBytes([]byte{192, 168, 1, 10}); got != "192.168.1.10" {
		t.Fatalf("raw ipv4: %q", got)
	}
	if got := sanitizeSNMPBytes([]byte{1, 10, 0, 0, 1}); got != "10.0.0.1" {
		t.Fatalf("lldp family+ipv4: %q", got)
	}
	bad := []byte{0xe0, 0xd9, 0xe3}
	got := sanitizeSNMPBytes(bad)
	if got == string(bad) || got == "" {
		t.Fatalf("expected hex for invalid utf8, got %q", got)
	}

	// Windows DisplayString с хвостовым NUL (Belyj-PC Wi-Fi Direct / WFP).
	win := append([]byte("Microsoft Wi-Fi Direct Virtual Adapter-WFP Native MAC Layer LightWeight Filter-0000"), 0)
	want := "Microsoft Wi-Fi Direct Virtual Adapter-WFP Native MAC Layer LightWeight Filter-0000"
	if got := sanitizeSNMPBytes(win); got != want {
		t.Fatalf("windows NUL-terminated DisplayString: %q", got)
	}
}

func TestHumanizeSNMPText(t *testing.T) {
	hexBlob := "4d6963726f736f66742057692d466920446972656374205669727475616c20416461707465722d574650204e6174697665204d4143204c61796572204c696768745765696768742046696c7465722d3030303000"
	want := "Microsoft Wi-Fi Direct Virtual Adapter-WFP Native MAC Layer LightWeight Filter-0000"
	if got := HumanizeSNMPText(hexBlob); got != want {
		t.Fatalf("humanize hex DisplayString: %q", got)
	}
	if got := HumanizeSNMPText("port 15: LIFT"); got != "port 15: LIFT" {
		t.Fatalf("plain text unchanged: %q", got)
	}
	// Короткий hex (не DisplayString) не трогаем.
	if got := HumanizeSNMPText("aabbccdd"); got != "aabbccdd" {
		t.Fatalf("short hex left as-is: %q", got)
	}
}
