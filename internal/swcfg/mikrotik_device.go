package swcfg

import "strings"

func normalizeMikrotikCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

func isMikrotikVendor(sshVendor, sysDescr, name string) bool {
	return DetectVendor(sshVendor, sysDescr, name) == VendorMikrotik
}

// IsMikrotikRouterDevice — RouterOS роутер (категория router + вендор MikroTik).
// Для таких узлов: без периодического SSH sync портов, порты только для чтения.
func IsMikrotikRouterDevice(category, sshVendor, sysDescr, name string) bool {
	if normalizeMikrotikCategory(category) != "router" {
		return false
	}
	return isMikrotikVendor(sshVendor, sysDescr, name)
}

// IsMikrotikRouterForConfigBackup — RouterOS в ZIP/снимки конфига.
// Категории router и ap (wAP, cAP…); вендор — явно mikrotik или автодетект
// (RouterOS, CCR, RouterBOARD, RBwAP… в sysDescr/имени).
func IsMikrotikRouterForConfigBackup(category, sshVendor, sysDescr, name string) bool {
	switch normalizeMikrotikCategory(category) {
	case "router", "ap":
		return isMikrotikVendor(sshVendor, sysDescr, name)
	default:
		return false
	}
}
