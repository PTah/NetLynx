package snmp

import (
	"strings"

	"github.com/gosnmp/gosnmp"
)

const (
	oidCPUUbiquiti1 = "1.3.6.1.4.1.41112.1.6.1.1.0"
	oidCPUUbiquiti2 = "1.3.6.1.4.1.4413.1.1.1.1.5.0"
	oidCPUMikrotik  = "1.3.6.1.4.1.14988.1.1.3.10.0"
	oidCPUSNR       = "1.3.6.1.4.1.40418.7.100.30.1.0"
	oidCPUCisco5Min = "1.3.6.1.4.1.9.9.109.1.1.1.1.5"
	oidCPUJuniper   = "1.3.6.1.4.1.2636.3.1.13.1.8.0"
	oidCPUELTEX     = "1.3.6.1.4.1.35265.1.1.1.1.1.0"
	oidCPUIdleUCD   = "1.3.6.1.4.1.2021.11.9.0"
	oidCPUHrBase    = "1.3.6.1.2.1.25.3.3.1.2"
)

type cpuProfile struct {
	Name     string
	MatchAny []string
	OIDs     []string
}

var cpuProfiles = []cpuProfile{
	{Name: "mikrotik", MatchAny: []string{"mikrotik", "routeros"}, OIDs: []string{oidCPUMikrotik, oidCPUIdleUCD}},
	{Name: "ubiquiti", MatchAny: []string{"ubiquiti", "edgeswitch", "unifi"}, OIDs: []string{oidCPUUbiquiti1, oidCPUUbiquiti2, oidCPUIdleUCD}},
	{Name: "snr", MatchAny: []string{"snr"}, OIDs: []string{oidCPUSNR, oidCPUIdleUCD}},
	{Name: "huawei", MatchAny: []string{"huawei", "vrp", "quidway"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "aruba", MatchAny: []string{"aruba", "aos-cx", "hpe networking"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "hp", MatchAny: []string{"procurve", "hewlett-packard", "hewlett packard"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "zyxel", MatchAny: []string{"zyxel"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "cisco", MatchAny: []string{"cisco", "ios-xe", "nx-os", "nexus", "catalyst"}, OIDs: []string{oidCPUCisco5Min, oidCPUIdleUCD, oidCPUHrBase}},
	{Name: "tplink", MatchAny: []string{"tp-link", "tplink", "jetstream"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "dlink", MatchAny: []string{"d-link", "dlink", "dgs-", "des-"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "dahua", MatchAny: []string{"dahua", "dh-pfs", "pfs42"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "hikvision", MatchAny: []string{"hikvision", "ds-3e", "hiwatch"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "trassir", MatchAny: []string{"trassir"}, OIDs: []string{oidCPUHrBase, oidCPUIdleUCD}},
	{Name: "juniper", MatchAny: []string{"juniper", "junos"}, OIDs: []string{oidCPUJuniper, oidCPUIdleUCD, oidCPUHrBase}},
	{Name: "eltex", MatchAny: []string{"eltex", "mes"}, OIDs: []string{oidCPUELTEX, oidCPUIdleUCD, oidCPUHrBase}},
}

// ReadCPU пытается прочитать CPU с учетом профиля вендора.
// Возвращает имя профиля и значение CPU в процентах (0..100), если удалось.
func ReadCPU(g *gosnmp.GoSNMP, sysDescr string) (string, *float32, error) {
	desc := strings.ToLower(sysDescr)
	prof := cpuProfile{Name: "generic", OIDs: []string{oidCPUIdleUCD}}
	for _, p := range cpuProfiles {
		for _, needle := range p.MatchAny {
			if strings.Contains(desc, needle) {
				prof = p
				break
			}
		}
		if prof.Name == p.Name {
			break
		}
	}

	for _, oid := range prof.OIDs {
		if oid == oidCPUHrBase {
			if v := readHrProcessorLoad(g); v != nil {
				return prof.Name, v, nil
			}
			continue
		}
		if v := readCPUPercentByOID(g, oid); v != nil {
			return prof.Name, v, nil
		}
	}
	// как fallback, если профиль не дал результат — пробуем усреднение hrProcessorLoad
	if v := readHrProcessorLoad(g); v != nil {
		return prof.Name, v, nil
	}
	return prof.Name, nil, nil
}

func readCPUPercentByOID(g *gosnmp.GoSNMP, oid string) *float32 {
	pdus, err := g.Get([]string{oid})
	if err != nil || len(pdus.Variables) == 0 {
		return nil
	}
	v := float32(pduInt64(pdus.Variables[0]))
	// Для ucd ssCpuIdle возвращается idle, конвертируем в usage.
	if normalizeOID(oid) == oidCPUIdleUCD {
		v = 100 - v
	}
	if v < 0 || v > 100 {
		return nil
	}
	return &v
}

func readHrProcessorLoad(g *gosnmp.GoSNMP) *float32 {
	var total float32
	var count int
	err := g.BulkWalk(oidCPUHrBase, func(pdu gosnmp.SnmpPDU) error {
		v := float32(pduInt64(pdu))
		if v >= 0 && v <= 100 {
			total += v
			count++
		}
		return nil
	})
	if err != nil || count == 0 {
		return nil
	}
	avg := total / float32(count)
	return &avg
}
