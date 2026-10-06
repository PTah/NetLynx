package models

import "strings"

// NormalizeReachabilityMode: auto|ping|online|offline.
// Пустое / неизвестное → auto. Учитывает legacy online_override, если mode пустой.
func NormalizeReachabilityMode(mode string, onlineOverride *bool) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "auto", "ping", "online", "offline":
		return mode
	}
	if onlineOverride != nil {
		if *onlineOverride {
			return "online"
		}
		return "offline"
	}
	return "auto"
}

// ReachabilityModeEffective — режим с учётом legacy online_override.
func (d Device) ReachabilityModeEffective() string {
	return NormalizeReachabilityMode(d.ReachabilityMode, d.OnlineOverride)
}

// IsOnline совпадает с веб-логикой «Узлы»: серая строка = оффлайн.
// auto: свитч/роутер — SNMP; прочие — ping OR SNMP.
// ping: только ICMP (без SNMP и без учёта категории).
// online/offline: ручная отметка.
func (d Device) IsOnline() bool {
	switch d.ReachabilityModeEffective() {
	case "online":
		return true
	case "offline":
		return false
	case "ping":
		return d.LastPingOK != nil && *d.LastPingOK
	default: // auto
		if d.LastSNMPOK != nil && *d.LastSNMPOK {
			return true
		}
		cat := strings.ToLower(strings.TrimSpace(d.DeviceCategory))
		if cat == "" || cat == "switch" || cat == "router" || cat == "коммутатор" || cat == "коммутаторы" {
			return false
		}
		return d.LastPingOK != nil && *d.LastPingOK
	}
}
