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

// IsMikrotikRouterForConfigBackup — роутер RouterOS в ZIP/снимки конфига.
// Явный ssh_vendor=mikrotik или автодетект по sysDescr/имени (RouterOS, CCR, RouterBOARD…).
func IsMikrotikRouterForConfigBackup(category, sshVendor, sysDescr, name string) bool {
	return IsMikrotikRouterDevice(category, sshVendor, sysDescr, name)
}
