package store

import "testing"

func TestValidateWiFiClientIPPrefix(t *testing.T) {
	if err := ValidateWiFiClientIPPrefix("192.168.120.0/24"); err != nil {
		t.Fatalf("valid prefix: %v", err)
	}
	if err := ValidateWiFiClientIPPrefix(""); err != nil {
		t.Fatalf("empty uses default: %v", err)
	}
	if err := ValidateWiFiClientIPPrefix("not-a-cidr"); err == nil {
		t.Fatal("expected error for bad cidr")
	}
}

func TestAnyIPInCIDR(t *testing.T) {
	if !anyIPInCIDR([]string{"192.168.120.226"}, "192.168.120.0/24") {
		t.Fatal("wifi IP expected in prefix")
	}
	if anyIPInCIDR([]string{"192.168.160.35"}, "192.168.120.0/24") {
		t.Fatal("LAN IP not in wifi prefix")
	}
	if anyIPInCIDR([]string{"192.168.120.226", "10.0.0.1"}, "192.168.120.0/24") {
		// any match counts
	} else {
		t.Fatal("mixed list should match wifi")
	}
}

func TestIsLocallyAdministeredMAC_2a(t *testing.T) {
	if !IsLocallyAdministeredMAC("2a:89:2d:5a:80:ba") {
		t.Fatal("2a:89 expected LAA (WiFi random MAC)")
	}
	if !IsLocallyAdministeredMAC("52:54:4c:83:09:e0") {
		t.Fatal("52:54 expected LAA")
	}
	if IsLocallyAdministeredMAC("e4:9f:7d:f0:3a:22") {
		t.Fatal("e4:9f universal OUI")
	}
}

func TestEventPayloadMAC(t *testing.T) {
	m := EventPayloadMAC("MAC_FLAPPING", map[string]interface{}{"mac": "aa:bb:cc:dd:ee:ff"})
	if m != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("got %q", m)
	}
	m = EventPayloadMAC("ACCESS_PORT_MAC_SUBSTITUTED", map[string]interface{}{
		"old_mac": "11:22:33:44:55:66",
		"new_mac": "aa:bb:cc:dd:ee:01",
	})
	if m != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("substituted: got %q", m)
	}
	m = EventPayloadMAC("LINK_UP", map[string]interface{}{"if_descr": "Gi1/0/1"})
	if m != "" {
		t.Fatalf("link event: got %q", m)
	}
}
