package traprecv

import (
	"fmt"
	"strconv"
	"strings"
)

// TrapDecode — человекочитаемая расшифровка SNMP trap.
type TrapDecode struct {
	Label   string `json:"trap_label"`
	Summary string `json:"trap_summary"`
}

// DecodeTrap возвращает короткую подпись и описание по trap OID и varbinds.
func DecodeTrap(trapOID string, ifIndex *int, payload map[string]interface{}) TrapDecode {
	oid := normalizeTrapOID(trapOID)
	if oid == "" {
		return TrapDecode{Label: "SNMP trap", Summary: "Неизвестный trap (нет trap OID)"}
	}

	vb := parseVarbinds(payload)
	idx := ifIndex
	if idx == nil {
		if i, ok := vb.ifIndex(); ok {
			idx = &i
		}
	}

	label, base := lookupTrapLabel(oid)
	summary := base
	if extra := formatTrapExtra(oid, idx, vb); extra != "" {
		if summary != "" {
			summary += ". " + extra
		} else {
			summary = extra
		}
	}
	if summary == "" {
		summary = oid
	}
	return TrapDecode{Label: label, Summary: summary}
}

func normalizeTrapOID(oid string) string {
	return strings.TrimPrefix(strings.TrimSpace(oid), ".")
}

type varbindIndex map[string]string

func parseVarbinds(payload map[string]interface{}) varbindIndex {
	out := make(varbindIndex)
	if payload == nil {
		return out
	}
	raw, ok := payload["varbinds"].([]interface{})
	if !ok {
		return out
	}
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		oid := normalizeTrapOID(fmt.Sprint(m["oid"]))
		if oid == "" {
			continue
		}
		out[oid] = strings.TrimSpace(fmt.Sprint(m["value"]))
	}
	return out
}

func (v varbindIndex) ifIndex() (int, bool) {
	for oid, val := range v {
		norm := normalizeTrapOID(oid)
		if norm == "1.3.6.1.2.1.2.2.1.1" || strings.HasPrefix(norm, "1.3.6.1.2.1.2.2.1.1.") {
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err == nil && n > 0 {
				return n, true
			}
		}
	}
	return 0, false
}

func (v varbindIndex) ifOperStatus(ifIndex *int) string {
	if ifIndex == nil {
		return ""
	}
	key := fmt.Sprintf("1.3.6.1.2.1.2.2.1.8.%d", *ifIndex)
	val, ok := v[key]
	if !ok {
		for oid, val := range v {
			if strings.HasPrefix(oid, "1.3.6.1.2.1.2.2.1.8.") {
				return operStatusText(val)
			}
		}
		return ""
	}
	return operStatusText(val)
}

func operStatusText(raw string) string {
	switch strings.TrimSpace(raw) {
	case "1":
		return "up"
	case "2":
		return "down"
	case "3":
		return "testing"
	case "4":
		return "unknown"
	case "5":
		return "dormant"
	case "6":
		return "notPresent"
	case "7":
		return "lowerLayerDown"
	default:
		if raw != "" {
			return raw
		}
		return ""
	}
}

func lookupTrapLabel(oid string) (label, summary string) {
	// Стандартные SNMPv2 traps (SNMPv2-MIB / IF-MIB).
	switch oid {
	case "1.3.6.1.6.3.1.1.5.1":
		return "coldStart", "Холодный старт агента SNMP"
	case "1.3.6.1.6.3.1.1.5.2":
		return "warmStart", "Тёплый старт агента SNMP"
	case "1.3.6.1.6.3.1.1.5.3":
		return "linkDown", "Порт отключён (linkDown)"
	case "1.3.6.1.6.3.1.1.5.4":
		return "linkUp", "Порт подключён (linkUp)"
	case "1.3.6.1.6.3.1.1.5.5":
		return "authenticationFailure", "Ошибка SNMP authentication"
	case "1.3.6.1.6.3.1.1.5.6":
		return "egpNeighborLoss", "Потеря EGP-соседа"
	case "1.3.6.1.6.3.1.1.5.7":
		return "enterpriseSpecific", "Enterprise-specific trap (v1)"
	}

	// Ubiquiti EdgeSwitch — EdgeSwitch-SWITCHING-MIB (4413.1.1.1.0.*).
	if strings.HasPrefix(oid, "1.3.6.1.4.1.4413.1.1.1.0.") {
		suffix := strings.TrimPrefix(oid, "1.3.6.1.4.1.4413.1.1.1.0.")
		if name, desc, ok := edgeSwitchTrap(suffix); ok {
			return name, desc
		}
		return "EdgeSwitch trap", "Ubiquiti EdgeSwitch: " + oid
	}

	// Частые enterprise OID без полной MIB.
	if strings.HasPrefix(oid, "1.3.6.1.4.1.") {
		return "enterprise trap", "Vendor trap " + oid
	}

	return "SNMP trap", oid
}

func edgeSwitchTrap(suffix string) (name, desc string, ok bool) {
	table := map[string][2]string{
		"1":  {"multipleUsersTrap", "Несколько admin-сессий CLI"},
		"2":  {"broadcastStormStartTrap", "Broadcast storm (начало, obsolete)"},
		"3":  {"broadcastStormEndTrap", "Broadcast storm (конец, obsolete)"},
		"4":  {"linkFailureTrap", "Сбой линка (obsolete)"},
		"5":  {"vlanRequestFailureTrap", "Ошибка VLAN request (obsolete)"},
		"6":  {"vlanDeleteLastTrap", "Попытка удалить последний VLAN"},
		"7":  {"vlanDefaultCfgFailureTrap", "Сбой сброса VLAN к defaults"},
		"8":  {"vlanRestoreFailureTrap", "Сбой restore VLAN (obsolete)"},
		"9":  {"fanFailureTrap", "Отказ вентилятора (obsolete)"},
		"10": {"stpInstanceNewRootTrap", "STP: новый root (multi-instance)"},
		"11": {"stpInstanceTopologyChangeTrap", "STP: смена топологии (multi-instance)"},
		"12": {"powerSupplyStatusChangeTrap", "Изменение статуса БП (obsolete)"},
		"13": {"failedUserLoginTrap", "Неудачный вход (CLI/Web)"},
		"14": {"userLockoutTrap", "Блокировка пользователя после неудачных входов"},
		"15": {"daiIntfErrorDisabledTrap", "DAI: порт error-disabled (rate limit)"},
		"16": {"stpInstanceLoopInconsistentStartTrap", "STP: loop inconsistent (начало)"},
		"17": {"stpInstanceLoopInconsistentEndTrap", "STP: loop inconsistent (конец)"},
		"18": {"dhcpSnoopingIntfErrorDisabledTrap", "DHCP snooping: порт error-disabled"},
		"19": {"noStartupConfigTrap", "Нет startup-config при включённом SSH"},
		"20": {"agentSwitchIpAddressConflictTrap", "Конфликт IP (ARP)"},
		"21": {"agentSwitchCpuRisingThresholdTrap", "CPU выше порога"},
		"22": {"agentSwitchCpuFallingThresholdTrap", "CPU ниже порога"},
		"23": {"agentSwitchCpuFreeMemBelowThresholdTrap", "Свободная память CPU ниже порога"},
		"24": {"agentSwitchCpuFreeMemAboveThresholdTrap", "Свободная память CPU выше порога"},
		"25": {"topologyChangeInitiatedTrap", "Topology change на порту"},
		"26": {"loopDetectedTrap", "STP: обнаружена петля"},
		"27": {"agentSwitchMbufRisingThresholdTrap", "Mbuf выше порога"},
		"28": {"agentSwitchMbufFallingThresholdTrap", "Mbuf ниже порога"},
		"29": {"loginSessionStartStopTrap", "CLI-сессия началась или завершилась"},
		"31": {"agentSwitchStormControlTrap", "Storm control: достигнут rate limit"},
	}
	if row, ok := table[suffix]; ok {
		return row[0], row[1], true
	}
	return "", "", false
}

func formatTrapExtra(oid string, ifIndex *int, vb varbindIndex) string {
	parts := make([]string, 0, 3)
	if ifIndex != nil && *ifIndex > 0 {
		parts = append(parts, fmt.Sprintf("ifIndex %d", *ifIndex))
	}
	if oper := vb.ifOperStatus(ifIndex); oper != "" {
		parts = append(parts, "oper="+oper)
	}
	switch oid {
	case "1.3.6.1.6.3.1.1.5.3", "1.3.6.1.6.3.1.1.5.4":
		// link traps — ifIndex/oper уже добавлены.
	case "1.3.6.1.4.1.4413.1.1.1.0.29":
		parts = append(parts, "обычно SSH/Telnet/console")
	}
	return strings.Join(parts, ", ")
}
