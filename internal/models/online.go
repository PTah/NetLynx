package models

import "strings"

// IsOnline совпадает с веб-логикой «Узлы»: серая строка = оффлайн.
// Свитч/роутер: SNMP ok (или ручной override). Один ping не считается.
func (d Device) IsOnline() bool {
	if d.OnlineOverride != nil {
		return *d.OnlineOverride
	}
	if d.LastSNMPOK != nil && *d.LastSNMPOK {
		return true
	}
	cat := strings.ToLower(strings.TrimSpace(d.DeviceCategory))
	if cat == "" || cat == "switch" || cat == "router" || cat == "коммутатор" || cat == "коммутаторы" {
		return false
	}
	return d.LastPingOK != nil && *d.LastPingOK
}
