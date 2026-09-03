package snmp

import (
	"fmt"
	"strings"

	"github.com/gosnmp/gosnmp"
)

const (
	oidLldpLocChassisIdSubtype = "1.0.8802.1.1.2.1.3.1.0"
	oidLldpLocChassisId        = "1.0.8802.1.1.2.1.3.2.0"
	oidDot1dBaseBridgeAddress  = "1.3.6.1.2.1.17.1.1.0"
)

// ReadLocalChassisMAC возвращает chassis MAC устройства (aa:bb:…), если SNMP отдаёт.
// Порядок: LLDP local chassis (то, что рекламируется соседям) → dot1dBaseBridgeAddress.
func ReadLocalChassisMAC(g *gosnmp.GoSNMP) (string, error) {
	if g == nil {
		return "", fmt.Errorf("snmp: nil client")
	}
	if mac := readLLDPLocChassisMAC(g); mac != "" {
		return mac, nil
	}
	if mac := readBridgeAddressMAC(g); mac != "" {
		return mac, nil
	}
	return "", nil
}

func readLLDPLocChassisMAC(g *gosnmp.GoSNMP) string {
	pdus, err := g.Get([]string{oidLldpLocChassisIdSubtype, oidLldpLocChassisId})
	if err != nil || pdus == nil {
		return ""
	}
	subtype := 0
	var raw []byte
	for _, p := range pdus.Variables {
		switch normalizeOID(p.Name) {
		case oidLldpLocChassisIdSubtype:
			subtype = pduInt(p)
		case oidLldpLocChassisId:
			raw = pduRawBytes(p)
		}
	}
	if len(raw) == 0 {
		return ""
	}
	n := &NeighborInfo{}
	applyLLDPChassis(n, subtype, raw)
	return normalizeChassisMAC(n.RemoteChassisID)
}

func readBridgeAddressMAC(g *gosnmp.GoSNMP) string {
	pdus, err := g.Get([]string{oidDot1dBaseBridgeAddress})
	if err != nil || pdus == nil || len(pdus.Variables) == 0 {
		return ""
	}
	raw := pduRawBytes(pdus.Variables[0])
	if mac := macFromBytes(raw); mac != "" {
		return mac
	}
	// Иногда отдаёт как DisplayString / hex text
	s := strings.TrimSpace(SanitizeSNMPValue(raw))
	if s == "" {
		return ""
	}
	return normalizeChassisMAC(FormatMAC(s))
}

func normalizeChassisMAC(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// FormatMAC / applyLLDPChassis уже дают aa:bb:…; отсекаем не-MAC (IP и т.п.)
	parts := strings.Split(strings.ToLower(strings.ReplaceAll(raw, "-", ":")), ":")
	if len(parts) != 6 {
		// компактный aabbccddeeff
		hex := strings.Map(func(r rune) rune {
			switch {
			case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
				return r
			case r >= 'A' && r <= 'F':
				return r + ('a' - 'A')
			default:
				return -1
			}
		}, raw)
		if len(hex) != 12 {
			return ""
		}
		return fmt.Sprintf("%s:%s:%s:%s:%s:%s", hex[0:2], hex[2:4], hex[4:6], hex[6:8], hex[8:10], hex[10:12])
	}
	for _, p := range parts {
		if len(p) != 2 {
			return ""
		}
		for _, c := range p {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
				return ""
			}
		}
	}
	return strings.Join(parts, ":")
}
