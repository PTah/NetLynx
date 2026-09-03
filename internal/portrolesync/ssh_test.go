package portrolesync

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

func TestShouldSyncPortRolesFromConfig(t *testing.T) {
	sys := "RouterOS 7.12"
	rb := &models.Device{DeviceCategory: "router", Name: "RB2011-GATE", SysDescr: &sys}
	if ShouldSyncPortRolesFromConfig(rb) {
		t.Fatal("mikrotik router should skip cli sync")
	}
	sw := &models.Device{DeviceCategory: "switch", Name: "EdgeSwitch 24"}
	if !ShouldSyncPortRolesFromConfig(sw) {
		t.Fatal("switch should sync")
	}
	cisco := &models.Device{DeviceCategory: "router", Name: "ISR", SysDescr: strPtr("Cisco IOS")}
	if !ShouldSyncPortRolesFromConfig(cisco) {
		t.Fatal("non-mikrotik router should sync")
	}
}

func strPtr(s string) *string { return &s }
