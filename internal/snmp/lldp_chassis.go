package snmp

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
)

// LLDP chassis ID subtype (lldpRemChassisIdSubtype).
const (
	lldpChassisSubtypeMAC     = 4
	lldpChassisSubtypeNetwork = 5
)

const baseLldpRemChassisIdSubtype = "1.0.8802.1.1.2.1.4.1.1.4"

func pduRawBytes(p gosnmp.SnmpPDU) []byte {
	switch v := p.Value.(type) {
	case []byte:
		return v
	case string:
		return []byte(v)
	default:
		return nil
	}
}

func pduInt(p gosnmp.SnmpPDU) int {
	switch v := p.Value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(v))
		return n
	default:
		return 0
	}
}

// applyLLDPChassis заполняет RemoteChassisID и/или RemoteMgmtAddr по subtype + значению.
// networkAddress (5) — это IP (часто 01 + IPv4), а не MAC; macAddress (4) — настоящий MAC.
func applyLLDPChassis(n *NeighborInfo, subtype int, raw []byte) {
	if n == nil {
		return
	}
	switch subtype {
	case lldpChassisSubtypeMAC:
		if mac := macFromBytes(raw); mac != "" {
			n.RemoteChassisID = mac
			return
		}
	case lldpChassisSubtypeNetwork:
		if ip := decodeNetworkAddressBytes(raw); ip != "" {
			if n.RemoteMgmtAddr == "" {
				n.RemoteMgmtAddr = ip
			}
			return
		}
	}
	// subtype неизвестен / walk subtype не отдал — эвристика по сырым байтам
	if mac := macFromBytes(raw); mac != "" {
		n.RemoteChassisID = mac
		return
	}
	if ip := decodeNetworkAddressBytes(raw); ip != "" {
		if n.RemoteMgmtAddr == "" {
			n.RemoteMgmtAddr = ip
		}
		return
	}
	// текстовый fallback (interfaceName и т.п.)
	s := strings.TrimSpace(SanitizeSNMPValue(raw))
	if s == "" {
		return
	}
	// Компактный «01c0a8aa49» после hex-encode — IP, не MAC.
	if ip := decodeNetworkAddressHex(s); ip != "" {
		if n.RemoteMgmtAddr == "" {
			n.RemoteMgmtAddr = ip
		}
		return
	}
	n.RemoteChassisID = FormatMAC(s)
}

func macFromBytes(b []byte) string {
	if len(b) != 6 {
		return ""
	}
	printable := 0
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			printable++
		}
	}
	if printable >= 4 {
		// похоже на ASCII, не бинарный MAC
		return ""
	}
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5])
}

// decodeNetworkAddressBytes: LLDP/CDP network address — IANA family + адрес.
func decodeNetworkAddressBytes(b []byte) string {
	if len(b) == 4 {
		return net.IP(b).String()
	}
	if len(b) >= 5 && b[0] == 1 {
		return net.IP(b[1:5]).String()
	}
	if len(b) >= 6 && binary.BigEndian.Uint16(b[0:2]) == 1 {
		return net.IP(b[2:6]).String()
	}
	return ""
}

// decodeNetworkAddressHex распознаёт 01c0a8aa49 / 01:c0:a8:aa:49 как IPv4 (family=1).
func decodeNetworkAddressHex(raw string) string {
	hex := strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r >= 'a' && r <= 'f':
			return r
		case r >= 'A' && r <= 'F':
			return r + ('a' - 'A')
		default:
			return -1
		}
	}, strings.TrimSpace(raw))
	if len(hex) != 10 || !strings.HasPrefix(hex, "01") {
		return ""
	}
	var b [4]byte
	for i := 0; i < 4; i++ {
		v, err := strconv.ParseUint(hex[2+i*2:4+i*2], 16, 8)
		if err != nil {
			return ""
		}
		b[i] = byte(v)
	}
	return net.IP(b[:]).String()
}
