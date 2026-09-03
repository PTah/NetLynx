package snmp

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	ubnt12FModelRe      = regexp.MustCompile(`\b12f\b`)
	snrUplinkPortNameRe = regexp.MustCompile(`(?i)(?:ethernet|gi|gigabitethernet)\d+/\d+/(\d+)$`)
)

// UbiquitiLikelySFPPort — порты SFP/SFP+ на EdgeSwitch по модели (на них нет PSE/PoE).
func UbiquitiLikelySFPPort(sysDescr string, portNum int) bool {
	if portNum <= 0 {
		return false
	}
	s := strings.ToLower(sysDescr)
	if !strings.Contains(s, "edgeswitch") && !strings.Contains(s, "ubnt") {
		return false
	}
	if ubnt12FModelRe.MatchString(s) {
		return portNum >= 1 && portNum <= 12
	}
	if strings.Contains(s, "edgeswitch 8") {
		return portNum >= 9 && portNum <= 10
	}
	if strings.Contains(s, "edgeswitch 16") {
		return portNum >= 17 && portNum <= 18
	}
	if strings.Contains(s, "edgeswitch 24") {
		return portNum >= 25 && portNum <= 26
	}
	if strings.Contains(s, "edgeswitch 48") {
		return portNum >= 49 && portNum <= 52
	}
	return false
}

// PortNumFromIfName — последний числовой сегмент ifName (0/26 → 26).
func PortNumFromIfName(ifName string) int {
	name := strings.TrimSpace(ifName)
	if name == "" {
		return 0
	}
	m := trailingPortNumRe.FindStringSubmatch(name)
	if len(m) != 2 {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// ClearPoEOnFiberPorts принудительно снимает PoE с SFP/оптики (молния на uplink — всегда ошибка).
// Пишет false в карту, чтобы UpsertInterfaces сбросил залипший poe_active=true в БД.
func ClearPoEOnFiberPorts(poeByIf map[int]bool, ifRows map[int]IfRow, sysDescr string) {
	if poeByIf == nil || len(ifRows) == 0 {
		return
	}
	for idx, row := range ifRows {
		name := strings.TrimSpace(row.IfName)
		blob := strings.ToLower(name + " " + row.IfDescr)
		portNum := PortNumFromIfName(name)
		fiber := UbiquitiLikelySFPPort(sysDescr, portNum)
		if !fiber {
			if m := snrUplinkPortNameRe.FindStringSubmatch(name); len(m) == 2 {
				if pn, err := strconv.Atoi(m[1]); err == nil && pn >= 49 && pn <= 52 {
					fiber = true
				}
			}
		}
		if !fiber {
			if strings.Contains(blob, "sfp") || strings.Contains(blob, "xfp") || strings.Contains(blob, "qsfp") ||
				strings.Contains(blob, "fiber") || strings.Contains(blob, "optical") {
				fiber = true
			}
		}
		if fiber {
			poeByIf[idx] = false
		}
	}
}
