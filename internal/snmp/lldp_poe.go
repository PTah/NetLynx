package snmp

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// LLDP-MIB + LLDP-EXT-DOT3-MIB (IEEE 802.3): удалённый TLV «Power via MDI» — класс порта соседа.
// Значение pd(2) означает, что сосед объявляет себя PD (потребитель PoE) на этом локальном порту LLDP.
// Это не эквивалент pethPsePortDetectionStatus=deliveringPower, но на коммутаторах без PSE-MIB (напр. EdgeSwitch 1.9.x)
// иногда единственный сигнал по SNMP.
//
// По IEEE / LLDP-EXT-DOT3-MIB объект lldpXdot3RemPowerPortClass лежит в lldpXdot3RemPowerTable: …4623.1.3.2.1.1 (а не в …1.3.1.6).
// Ранее в коде ошибочно использовался суффикс …1.3.1.6 — на агентах с корректным деревом там No Such Object.
const (
	baseLldpLocPortIdSubtype = "1.0.8802.1.1.2.1.3.7.1.2"
	baseLldpLocPortID       = "1.0.8802.1.1.2.1.3.7.1.3"
	// lldpXdot3RemPowerPortClass (LldpPowerPortClass: pClassPSE=1, pClassPD=2)
	baseLldpXdot3RemPowerPortClass = "1.0.8802.1.1.2.1.5.4623.1.3.2.1.1"
	// Нестандартный/устаревший путь — опрашиваем вторым и объединяем, если когда-то встретится.
	baseLldpXdot3RemPowerPortClassLegacy = "1.0.8802.1.1.2.1.5.4623.1.3.1.6"
)

const (
	lldpPortIDSubtypeInterfaceAlias = 1
	lldpPortIDSubtypeInterfaceName  = 5
	lldpPortIDSubtypeLocal          = 7
)

const lldpXdot3PowerPortClassPD = 2

func lldpSuffixAfterBase(oid, base string) string {
	return oidSuffixAfterBase(normalizeOID(oid), strings.TrimPrefix(base, "."))
}

// parseLLDPDot3RemPowerSuffix: индекс lldpXdot3RemPowerEntry = { lldpRemTimeMark, lldpRemLocalPortNum, lldpRemIndex }.
func parseLLDPDot3RemPowerSuffix(suf string) (localPortNum int, ok bool) {
	suf = strings.Trim(suf, ".")
	parts := strings.Split(suf, ".")
	if len(parts) < 3 {
		return 0, false
	}
	n, err := strconv.Atoi(parts[len(parts)-2])
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func walkLLDPLocPortNumToIfIndex(g *gosnmp.GoSNMP, ifRows map[int]IfRow) map[int]int {
	sub := make(map[int]int64)
	_ = g.BulkWalk(baseLldpLocPortIdSubtype, func(pdu gosnmp.SnmpPDU) error {
		suf := lldpSuffixAfterBase(pdu.Name, baseLldpLocPortIdSubtype)
		if suf == "" {
			return nil
		}
		portNum, err := strconv.Atoi(strings.Trim(suf, "."))
		if err != nil || portNum <= 0 {
			return nil
		}
		sub[portNum] = pduInt64(pdu)
		return nil
	})
	idStr := make(map[int]string)
	_ = g.BulkWalk(baseLldpLocPortID, func(pdu gosnmp.SnmpPDU) error {
		suf := lldpSuffixAfterBase(pdu.Name, baseLldpLocPortID)
		if suf == "" {
			return nil
		}
		portNum, err := strconv.Atoi(strings.Trim(suf, "."))
		if err != nil || portNum <= 0 {
			return nil
		}
		idStr[portNum] = pduString(pdu)
		return nil
	})

	out := make(map[int]int)
	for portNum, st := range sub {
		id := idStr[portNum]
		if ifIdx := resolveLLDPLocalPortIDToIfIndex(portNum, st, id, ifRows); ifIdx > 0 {
			out[portNum] = ifIdx
		}
	}
	// Часть агентов отдаёт только Port ID без subtype (или subtype walk пуст).
	for portNum, id := range idStr {
		if _, ok := out[portNum]; ok {
			continue
		}
		if ifIdx := matchLLDPPortIDStringToIfIndex(id, ifRows); ifIdx > 0 {
			out[portNum] = ifIdx
			continue
		}
		if ifIdx := parseLLDPLocalPortIDOctets(id, ifRows); ifIdx > 0 {
			out[portNum] = ifIdx
		}
	}
	return out
}

func resolveLLDPLocalPortIDToIfIndex(locPort int, subtype int64, portID string, ifRows map[int]IfRow) int {
	portID = strings.TrimSpace(portID)
	switch subtype {
	case lldpPortIDSubtypeInterfaceName:
		for idx, row := range ifRows {
			if strings.TrimSpace(row.IfName) == portID {
				return idx
			}
		}
	case lldpPortIDSubtypeInterfaceAlias:
		for idx, row := range ifRows {
			if strings.TrimSpace(row.IfDescr) == portID {
				return idx
			}
		}
	case lldpPortIDSubtypeLocal:
		// Ubiquiti EdgeSwitch: subtype local(7), строка вида "0/1" совпадает с ifName.
		if ifIdx := matchLLDPPortIDStringToIfIndex(portID, ifRows); ifIdx > 0 {
			return ifIdx
		}
		if ifIdx := parseLLDPLocalPortIDOctets(portID, ifRows); ifIdx > 0 {
			return ifIdx
		}
		if n, err := strconv.Atoi(portID); err == nil && n > 0 {
			if _, ok := ifRows[n]; ok {
				return n
			}
		}
	default:
		if portID != "" {
			if ifIdx := matchLLDPPortIDStringToIfIndex(portID, ifRows); ifIdx > 0 {
				return ifIdx
			}
		}
	}
	return guessIfIndexFromPsePortIndex(ifRows, locPort)
}

// matchLLDPPortIDStringToIfIndex: явное совпадение Port ID с ifName/ifDescr (напр. EdgeSwitch "0/1").
func matchLLDPPortIDStringToIfIndex(portID string, ifRows map[int]IfRow) int {
	if portID == "" {
		return 0
	}
	for idx, row := range ifRows {
		if strings.TrimSpace(row.IfName) == portID {
			return idx
		}
		if strings.TrimSpace(row.IfDescr) == portID {
			return idx
		}
	}
	return 0
}

// parseLLDPLocalPortIDOctets: при subtype=local часто 4 октета ifIndex (big-endian) или один байт.
func parseLLDPLocalPortIDOctets(portID string, ifRows map[int]IfRow) int {
	b := []byte(portID)
	if len(b) == 4 {
		v := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
		if v > 0 && v <= 0x7fffffff {
			ifIdx := int(v)
			if _, ok := ifRows[ifIdx]; ok {
				return ifIdx
			}
		}
	}
	if len(b) == 1 {
		ifIdx := int(b[0])
		if _, ok := ifRows[ifIdx]; ok {
			return ifIdx
		}
	}
	return 0
}

func bulkWalkLldpRemPowerPDClass(g *gosnmp.GoSNMP, ifRows map[int]IfRow, locMap map[int]int, base string, out map[int]bool) error {
	return g.BulkWalk(base, func(pdu gosnmp.SnmpPDU) error {
		if pduInt64(pdu) != lldpXdot3PowerPortClassPD {
			return nil
		}
		suf := lldpSuffixAfterBase(pdu.Name, base)
		locPort, ok := parseLLDPDot3RemPowerSuffix(suf)
		if !ok {
			return nil
		}
		ifIdx := 0
		if locMap != nil {
			ifIdx = locMap[locPort]
		}
		if ifIdx <= 0 {
			ifIdx = guessIfIndexFromPsePortIndex(ifRows, locPort)
		}
		if ifIdx > 0 {
			out[ifIdx] = true
		}
		return nil
	})
}

// WalkLLDPDot3RemotePDByIfIndex: порты, где LLDP-сосед в TLV 802.3 (RemPower) объявлен как PD.
func WalkLLDPDot3RemotePDByIfIndex(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]bool, error) {
	return walkLLDPDot3RemotePDByIfIndex(g, ifRows)
}

// walkLLDPDot3RemotePDByIfIndex: порты, где LLDP-сосед в TLV 802.3 (RemPower) объявлен как PD.
func walkLLDPDot3RemotePDByIfIndex(g *gosnmp.GoSNMP, ifRows map[int]IfRow) (map[int]bool, error) {
	locMap := walkLLDPLocPortNumToIfIndex(g, ifRows)

	out := make(map[int]bool)
	bases := []string{baseLldpXdot3RemPowerPortClass, baseLldpXdot3RemPowerPortClassLegacy}
	var errs []error
	for _, base := range bases {
		if err := bulkWalkLldpRemPowerPDClass(g, ifRows, locMap, base, out); err != nil {
			errs = append(errs, err)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	if len(errs) == len(bases) {
		return nil, errs[0]
	}
	return out, nil
}

// eth0 у AP/камеры — не инфрa; eth1/0/1 ловит ветка со слэшем ниже.
var infraNeighborPortRe = regexp.MustCompile(`^(gi|te|ge|fa|fo|xe|hu|lag|po|bond|port-channel|stack|trunk)\d`)

// FilterLldpPdInfraUplinks снимает ложный PoE с uplink'ов: LLDP-PD на линии к другому коммутатору.
// Для таких портов пишет false, чтобы сбросить ранее залипшее poe_active=true.
func FilterLldpPdInfraUplinks(pdByIf map[int]bool, neighbors []NeighborInfo) {
	if len(pdByIf) == 0 {
		return
	}
	byIf := neighborPrimaryByIfIndex(neighbors)
	for ifIdx, active := range pdByIf {
		if !active {
			continue
		}
		if looksLikeInfraNeighbor(byIf[ifIdx]) {
			pdByIf[ifIdx] = false
		}
	}
}

func neighborPrimaryByIfIndex(neighbors []NeighborInfo) map[int]NeighborInfo {
	out := make(map[int]NeighborInfo)
	for _, n := range neighbors {
		if n.IfIndex <= 0 {
			continue
		}
		prev, ok := out[n.IfIndex]
		if !ok || strings.TrimSpace(n.RemoteSysName) != "" && strings.TrimSpace(prev.RemoteSysName) == "" {
			out[n.IfIndex] = n
		}
	}
	return out
}

func looksLikeInfraNeighbor(n NeighborInfo) bool {
	port := strings.ToLower(strings.TrimSpace(n.RemotePortID))
	sys := strings.ToLower(strings.TrimSpace(n.RemoteSysName))
	if port == "" && sys == "" {
		return false
	}
	if infraNeighborPortRe.MatchString(port) {
		return true
	}
	if strings.Contains(port, "/") {
		for _, p := range []string{"gi", "te", "ge", "fa", "fo", "xe", "eth", "ethernet", "hu", "lag", "po"} {
			if strings.HasPrefix(port, p) {
				return true
			}
		}
	}
	for _, kw := range []string{
		"switch", "edgeswitch", "eltex", "snr", "catalyst", "nexus", "comware",
		"router", "mikrotik", "aruba", "procurve", "extreme", "brocade", "d-link", "dlink",
		"huawei", "ruijie", "zyxel", "netgear",
	} {
		if strings.Contains(sys, kw) {
			return true
		}
	}
	return false
}

// IsInfraLLDPNeighbor — сосед похож на коммутатор/инфраструктуру (для PoE uplink и port_role trunk).
func IsInfraLLDPNeighbor(n NeighborInfo) bool {
	return looksLikeInfraNeighbor(n)
}
