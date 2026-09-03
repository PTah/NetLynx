package snmp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

const (
	oidDot1dTpFdbPort       = "1.3.6.1.2.1.17.4.3.1.2"
	oidDot1dBasePortIfIndex = "1.3.6.1.2.1.17.1.4.1.2"
	oidDot1qTpFdbPort       = "1.3.6.1.2.1.17.7.1.2.2.1.2"
)

type FDBIfStats struct {
	MACCount  int
	VLANCount int
}

// FDBLearnedEntry MAC на порту; VLAN заполняется из Q-BRIDGE при наличии.
type FDBLearnedEntry struct {
	IfIndex int
	VLANID  *int
}

// WalkFDB читает FDB, возвращая map[mac]ifIndex (без VLAN).
func WalkFDB(g *gosnmp.GoSNMP) (map[string]int, error) {
	entries, _, err := WalkFDBWithStats(g)
	if err != nil {
		return nil, err
	}
	return FDBEntriesToIfIndexMap(entries), nil
}

// FDBEntriesToIfIndexMap для poller-событий по MAC.
func FDBEntriesToIfIndexMap(entries map[string]FDBLearnedEntry) map[string]int {
	out := make(map[string]int, len(entries))
	for mac, e := range entries {
		out[mac] = e.IfIndex
	}
	return out
}

// WalkFDBWithStats читает FDB и статистику по портам.
// Классическая dot1dTpFdbPort опциональна (EdgeSwitch и др. отдают только Q-BRIDGE).
func WalkFDBWithStats(g *gosnmp.GoSNMP) (map[string]FDBLearnedEntry, map[int]FDBIfStats, error) {
	baseToIf := map[int]int{}
	if err := walk(g, oidDot1dBasePortIfIndex, func(pdu gosnmp.SnmpPDU) error {
		basePort, err := parseSuffixInt(pdu.Name, oidDot1dBasePortIfIndex)
		if err != nil {
			return nil
		}
		ifIndex := int(pduInt64(pdu))
		if ifIndex > 0 {
			baseToIf[basePort] = ifIndex
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	if len(baseToIf) == 0 {
		return nil, nil, fmt.Errorf("bridge port map empty")
	}

	entries := map[string]FDBLearnedEntry{}
	_ = walk(g, oidDot1dTpFdbPort, func(pdu gosnmp.SnmpPDU) error {
		mac, err := parseMACSuffix(pdu.Name, oidDot1dTpFdbPort)
		if err != nil {
			return nil
		}
		basePort := int(pduInt64(pdu))
		if basePort <= 0 {
			return nil
		}
		ifIndex, ok := baseToIf[basePort]
		if !ok || ifIndex <= 0 {
			return nil
		}
		entries[mac] = FDBLearnedEntry{IfIndex: ifIndex}
		return nil
	})

	vlansByIf := make(map[int]map[int]struct{})
	_ = walk(g, oidDot1qTpFdbPort, func(pdu gosnmp.SnmpPDU) error {
		vlanID, mac, err := parseVLANAndMACSuffix(pdu.Name, oidDot1qTpFdbPort)
		if err != nil {
			return nil
		}
		basePort := int(pduInt64(pdu))
		if basePort <= 0 {
			return nil
		}
		ifIndex, ok := baseToIf[basePort]
		if !ok || ifIndex <= 0 {
			return nil
		}
		v := vlanID
		entries[mac] = FDBLearnedEntry{IfIndex: ifIndex, VLANID: &v}
		m, ok := vlansByIf[ifIndex]
		if !ok {
			m = make(map[int]struct{})
			vlansByIf[ifIndex] = m
		}
		m[vlanID] = struct{}{}
		return nil
	})

	return entries, buildFDBIfStats(entries, vlansByIf), nil
}

func buildFDBIfStats(entries map[string]FDBLearnedEntry, vlansByIf map[int]map[int]struct{}) map[int]FDBIfStats {
	stats := make(map[int]FDBIfStats)
	for _, ent := range entries {
		s := stats[ent.IfIndex]
		s.MACCount++
		stats[ent.IfIndex] = s
	}
	for ifIndex, vlans := range vlansByIf {
		s := stats[ifIndex]
		s.VLANCount = len(vlans)
		stats[ifIndex] = s
	}
	return stats
}

func walk(g *gosnmp.GoSNMP, base string, fn gosnmp.WalkFunc) error {
	if g.Version == gosnmp.Version1 {
		return g.Walk(base, fn)
	}
	return g.BulkWalk(base, fn)
}

func parseSuffixInt(oid, base string) (int, error) {
	oid = normalizeOID(oid)
	base = normalizeOID(base)
	if !strings.HasPrefix(oid, base+".") {
		return 0, fmt.Errorf("oid mismatch")
	}
	s := strings.TrimPrefix(oid, base+".")
	return strconv.Atoi(s)
}

func parseMACSuffix(oid, base string) (string, error) {
	oid = normalizeOID(oid)
	base = normalizeOID(base)
	if !strings.HasPrefix(oid, base+".") {
		return "", fmt.Errorf("oid mismatch")
	}
	suffix := strings.TrimPrefix(oid, base+".")
	parts := strings.Split(suffix, ".")
	if len(parts) != 6 {
		return "", fmt.Errorf("unexpected mac suffix length")
	}
	return formatMACOctets(parts)
}

// parseVLANAndMACSuffix разбирает индекс dot1qTpFdbPort: vlan + [fdbId] + 6 октетов MAC.
func parseVLANAndMACSuffix(oid, base string) (int, string, error) {
	oid = normalizeOID(oid)
	base = normalizeOID(base)
	if !strings.HasPrefix(oid, base+".") {
		return 0, "", fmt.Errorf("oid mismatch")
	}
	suffix := strings.TrimPrefix(oid, base+".")
	parts := strings.Split(suffix, ".")
	var macParts []string
	switch len(parts) {
	case 7:
		// vlan + 6 MAC (EdgeSwitch и др.)
		macParts = parts[1:]
	case 8:
		// vlan + dot1qFdbId + 6 MAC (Q-BRIDGE)
		macParts = parts[2:]
	default:
		return 0, "", fmt.Errorf("unexpected suffix length")
	}
	vlanID, err := strconv.Atoi(parts[0])
	if err != nil || vlanID <= 0 || vlanID > 4094 {
		return 0, "", fmt.Errorf("invalid vlan id")
	}
	mac, err := formatMACOctets(macParts)
	if err != nil {
		return 0, "", err
	}
	return vlanID, mac, nil
}

func formatMACOctets(parts []string) (string, error) {
	if len(parts) != 6 {
		return "", fmt.Errorf("unexpected mac suffix length")
	}
	m := make([]string, 6)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 255 {
			return "", fmt.Errorf("invalid mac octet")
		}
		m[i] = fmt.Sprintf("%02x", n)
	}
	return strings.Join(m, ":"), nil
}
