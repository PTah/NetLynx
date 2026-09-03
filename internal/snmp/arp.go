package snmp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// ARPEntry строка ipNetToMediaTable (IP-MIB).
type ARPEntry struct {
	IP      string
	MAC     string
	IfIndex int
}

const (
	// ipNetToMediaPhysAddress — значение MAC; индекс OID: ifIndex.A.B.C.D
	oidIpNetToMediaPhysAddress = "1.3.6.1.2.1.4.22.1.2"
)

// WalkARP читает ipNetToMediaTable (IP + MAC + ifIndex).
// Стандартный индекс: ifIndex + 4 октета IPv4; MAC — в значении physAddress.
func WalkARP(g *gosnmp.GoSNMP) ([]ARPEntry, error) {
	type key struct {
		ip  string
		mac string
	}
	byKey := make(map[key]int)
	if err := walk(g, oidIpNetToMediaPhysAddress, func(pdu gosnmp.SnmpPDU) error {
		ifIndex, ip, err := parseIpNetToMediaIndex(pdu.Name, oidIpNetToMediaPhysAddress)
		if err != nil {
			return nil
		}
		mac := normalizeARPMac(macFromARPPhysPDU(pdu))
		if ip == "" || mac == "" || ip == "0.0.0.0" {
			return nil
		}
		if strings.HasPrefix(mac, "ff:") || strings.HasPrefix(mac, "01:") {
			return nil
		}
		byKey[key{ip: ip, mac: mac}] = ifIndex
		return nil
	}); err != nil {
		return nil, err
	}
	out := make([]ARPEntry, 0, len(byKey))
	for k, ifIdx := range byKey {
		out = append(out, ARPEntry{IP: k.ip, MAC: k.mac, IfIndex: ifIdx})
	}
	return out, nil
}

// parseIpNetToMediaIndex: суффикс OID = ifIndex.A.B.C.D (RFC 1213 / IP-MIB).
func parseIpNetToMediaIndex(oid, base string) (ifIndex int, ip string, err error) {
	oid = normalizeOID(oid)
	base = normalizeOID(base)
	if !strings.HasPrefix(oid, base+".") {
		return 0, "", fmt.Errorf("oid mismatch")
	}
	parts := strings.Split(strings.TrimPrefix(oid, base+"."), ".")
	if len(parts) < 5 {
		return 0, "", fmt.Errorf("short arp index")
	}
	// Берём хвост из 5 чисел: ifIndex + IPv4 (на случай лишних префиксов).
	tail := parts[len(parts)-5:]
	ifIndex, err = strconv.Atoi(tail[0])
	if err != nil || ifIndex < 0 {
		return 0, "", fmt.Errorf("invalid ifIndex")
	}
	for i := 1; i <= 4; i++ {
		n, e := strconv.Atoi(tail[i])
		if e != nil || n < 0 || n > 255 {
			return 0, "", fmt.Errorf("invalid ip octet")
		}
	}
	ip = fmt.Sprintf("%s.%s.%s.%s", tail[1], tail[2], tail[3], tail[4])
	return ifIndex, ip, nil
}

func macFromARPPhysPDU(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case []byte:
		if len(v) == 6 {
			return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", v[0], v[1], v[2], v[3], v[4], v[5])
		}
	}
	return pduString(pdu)
}

func normalizeARPMac(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", ":")
	if strings.Contains(s, ":") {
		return s
	}
	// Компактный hex чётной длины 6–12 → aa:bb:…
	if len(s) < 6 || len(s) > 12 || len(s)%2 != 0 {
		return s
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return s
		}
	}
	var b strings.Builder
	for i := 0; i < len(s); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(s[i : i+2])
	}
	return b.String()
}

// FormatMAC приводит MAC-подобную строку к aa:bb:…; иначе возвращает исходную (после lower/-→:).
func FormatMAC(s string) string {
	return normalizeARPMac(s)
}
