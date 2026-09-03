package swcfg

import "strings"

// IsMikrotikRouterDevice — RouterOS роутер (категория router + вендор MikroTik).
// Для таких узлов: без периодического SSH sync портов, порты только для чтения.
func IsMikrotikRouterDevice(category, sshVendor, sysDescr, name string) bool {
	c := strings.ToLower(strings.TrimSpace(category))
	if c != "router" {
		return false
	}
	return DetectVendor(sshVendor, sysDescr, name) == VendorMikrotik
}

// IsMikrotikRouterForConfigBackup — роутер в SSH-бэкап конфигов только при явно выбранном вендоре MikroTik.
func IsMikrotikRouterForConfigBackup(category, sshVendor string) bool {
	if strings.ToLower(strings.TrimSpace(category)) != "router" {
		return false
	}
	return strings.ToLower(strings.TrimSpace(sshVendor)) == string(VendorMikrotik)
}
