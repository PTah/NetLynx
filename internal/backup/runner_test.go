package backup

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

func TestWantSwitchConfigOnlySwitch(t *testing.T) {
	ok := models.Device{DeviceCategory: "switch", Host: "10.0.0.1"}
	if !wantSwitchConfig(ok) {
		t.Fatal("switch")
	}
	mk := models.Device{
		DeviceCategory: "router",
		Host:           "192.168.1.1",
		Name:           "RB2011-GATE",
		SSHVendor:      "mikrotik",
	}
	mk.SysDescr = strPtr("RouterOS 7.12 on RB2011")
	if !wantSwitchConfig(mk) {
		t.Fatal("mikrotik router should be backed up")
	}
	patio := models.Device{
		DeviceCategory: "router",
		Host:           "192.168.1.2",
		Name:           "Mikrot-Il Patio",
		SSHVendor:      "auto",
	}
	patio.SysDescr = strPtr("RouterOS 7.12")
	if wantSwitchConfig(patio) {
		t.Fatal("router without explicit mikrotik vendor should skip backup")
	}
	for _, cat := range []string{"router", "ap", "server", "camera"} {
		d := models.Device{DeviceCategory: cat, Host: "10.0.0.1", Name: cat}
		if cat == "router" {
			continue // non-mikrotik tested below
		}
		if wantSwitchConfig(d) {
			t.Fatalf("should skip %s", cat)
		}
	}
	cisco := models.Device{DeviceCategory: "router", Host: "10.0.0.2", Name: "ISR"}
	cisco.SysDescr = strPtr("Cisco IOS Software")
	if wantSwitchConfig(cisco) {
		t.Fatal("non-mikrotik router should skip backup")
	}
	emptyHost := models.Device{DeviceCategory: "switch", Host: ""}
	if wantSwitchConfig(emptyHost) {
		t.Fatal("no host")
	}
}

func strPtr(s string) *string { return &s }

func TestWantSwitchConfigSkipsOffline(t *testing.T) {
	ok := true
	bad := false
	online := models.Device{DeviceCategory: "switch", Host: "10.0.0.1", LastSNMPOK: &ok}
	if !online.IsOnline() {
		t.Fatal("online")
	}
	down := models.Device{DeviceCategory: "switch", Host: "10.0.0.2", LastSNMPOK: &bad}
	if down.IsOnline() {
		t.Fatal("down should be skipped")
	}
}
