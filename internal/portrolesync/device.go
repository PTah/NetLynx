package portrolesync

import (
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/swcfg"
)

func SwitchLikeCategory(cat string) bool {
	c := strings.ToLower(strings.TrimSpace(cat))
	return c == "" || c == "switch" || c == "router"
}

// ShouldSyncPortRolesFromConfig — sync show run по SSH (poller / карточка). RouterOS-роутеры исключены.
func ShouldSyncPortRolesFromConfig(dev *models.Device) bool {
	if dev == nil || !SwitchLikeCategory(dev.DeviceCategory) {
		return false
	}
	sys := ""
	if dev.SysDescr != nil {
		sys = *dev.SysDescr
	}
	return !swcfg.IsMikrotikRouterDevice(dev.DeviceCategory, dev.SSHVendor, sys, dev.Name)
}
