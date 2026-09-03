package snmp

import (
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// NeighborInfo — сосед на локальном порту (LLDP/CDP; несколько rem на ifIndex).
type NeighborInfo struct {
	IfIndex         int
	RemIndex        int
	RemoteSysName   string
	RemotePortID    string
	RemoteChassisID string
	RemoteMgmtAddr  string
}

const (
	baseLldpRemSysName   = "1.0.8802.1.1.2.1.4.1.1.9"
	baseLldpRemPortId    = "1.0.8802.1.1.2.1.4.1.1.7"
	baseLldpRemChassisId = "1.0.8802.1.1.2.1.4.1.1.5"
)

// WalkLLDPNeighbors возвращает всех LLDP-соседей (все remIndex на каждом локальном порту).
// Если lldpLocPortTable недоступна, используется fallback: ifIndex=locPort или сопоставление по ifName (0/N).
func WalkLLDPNeighbors(g *gosnmp.GoSNMP, ifRows map[int]IfRow) ([]NeighborInfo, error) {
	portToIf := walkLLDPLocPortNumToIfIndex(g, ifRows)

	type remKey struct {
		localPort int
		remIndex  int
	}
	byKey := make(map[remKey]*NeighborInfo)

	parseRem := func(oid, base string) (localPort, remIndex int, ok bool) {
		suf := lldpSuffixAfterBase(oid, base)
		parts := strings.Split(strings.Trim(suf, "."), ".")
		if len(parts) < 3 {
			return 0, 0, false
		}
		lp, err1 := strconv.Atoi(parts[len(parts)-2])
		ri, err2 := strconv.Atoi(parts[len(parts)-1])
		if err1 != nil || err2 != nil || lp <= 0 {
			return 0, 0, false
		}
		if ri <= 0 {
			ri = 1
		}
		return lp, ri, true
	}

	ensure := func(lp, ri int) *NeighborInfo {
		k := remKey{localPort: lp, remIndex: ri}
		if byKey[k] == nil {
			byKey[k] = &NeighborInfo{RemIndex: ri}
		}
		return byKey[k]
	}

	if err := g.BulkWalk(baseLldpRemSysName, func(pdu gosnmp.SnmpPDU) error {
		lp, ri, ok := parseRem(pdu.Name, baseLldpRemSysName)
		if !ok {
			return nil
		}
		ensure(lp, ri).RemoteSysName = pduString(pdu)
		return nil
	}); err != nil {
		// Как у CDP: ошибка primary walk → не писать пустой снимок (иначе ложный stale).
		return nil, err
	}
	if err := g.BulkWalk(baseLldpRemPortId, func(pdu gosnmp.SnmpPDU) error {
		lp, ri, ok := parseRem(pdu.Name, baseLldpRemPortId)
		if !ok {
			return nil
		}
		ensure(lp, ri).RemotePortID = pduString(pdu)
		return nil
	}); err != nil {
		return nil, err
	}

	subtypes := make(map[remKey]int)
	_ = g.BulkWalk(baseLldpRemChassisIdSubtype, func(pdu gosnmp.SnmpPDU) error {
		lp, ri, ok := parseRem(pdu.Name, baseLldpRemChassisIdSubtype)
		if !ok {
			return nil
		}
		subtypes[remKey{localPort: lp, remIndex: ri}] = pduInt(pdu)
		return nil
	})

	if err := g.BulkWalk(baseLldpRemChassisId, func(pdu gosnmp.SnmpPDU) error {
		lp, ri, ok := parseRem(pdu.Name, baseLldpRemChassisId)
		if !ok {
			return nil
		}
		k := remKey{localPort: lp, remIndex: ri}
		applyLLDPChassis(ensure(lp, ri), subtypes[k], pduRawBytes(pdu))
		return nil
	}); err != nil {
		return nil, err
	}

	out := make([]NeighborInfo, 0, len(byKey))
	for k, n := range byKey {
		ifIdx := resolveLLDPLocalPortNumToIfIndex(k.localPort, portToIf, ifRows)
		if ifIdx <= 0 || n == nil {
			continue
		}
		if strings.TrimSpace(n.RemoteSysName+n.RemotePortID+n.RemoteChassisID+n.RemoteMgmtAddr) == "" {
			continue
		}
		n.IfIndex = ifIdx
		n.RemIndex = k.remIndex
		out = append(out, *n)
	}
	return out, nil
}

// resolveLLDPLocalPortNumToIfIndex: lldpLocPortNum → ifIndex (таблица loc, прямой ifIndex, ifName 0/N).
func resolveLLDPLocalPortNumToIfIndex(locPort int, portToIf map[int]int, ifRows map[int]IfRow) int {
	if locPort <= 0 {
		return 0
	}
	if ifIdx := portToIf[locPort]; ifIdx > 0 {
		return ifIdx
	}
	if _, ok := ifRows[locPort]; ok {
		return locPort
	}
	return guessIfIndexFromPsePortIndex(ifRows, locPort)
}
