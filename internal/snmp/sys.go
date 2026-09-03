package snmp

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gosnmp/gosnmp"
)

const (
	oidSysDescr  = "1.3.6.1.2.1.1.1.0"
	oidSysName   = "1.3.6.1.2.1.1.5.0"
	oidSysUpTime = "1.3.6.1.2.1.1.3.0"
)

// SysGet читает sysName/sysDescr при уже установленном соединении g.Conn.
func SysGet(g *gosnmp.GoSNMP) (sysName, sysDescr string, err error) {
	pdus, err := g.Get([]string{oidSysDescr, oidSysName})
	if err != nil {
		return "", "", err
	}
	for _, v := range pdus.Variables {
		switch normalizeOID(v.Name) {
		case oidSysDescr:
			sysDescr = pduString(v)
		case oidSysName:
			sysName = pduString(v)
		}
	}
	if sysName == "" && sysDescr == "" {
		return "", "", fmt.Errorf("SNMP Get: пустой ответ")
	}
	return sysName, sysDescr, nil
}

// SysUpTimeCentiseconds читает sysUpTime (SNMP TimeTicks, сотые доли секунды).
func SysUpTimeCentiseconds(g *gosnmp.GoSNMP) (uint64, error) {
	pdus, err := g.Get([]string{oidSysUpTime})
	if err != nil {
		return 0, err
	}
	for _, v := range pdus.Variables {
		if normalizeOID(v.Name) != oidSysUpTime {
			continue
		}
		switch x := v.Value.(type) {
		case uint:
			return uint64(x), nil
		case uint32:
			return uint64(x), nil
		case uint64:
			return x, nil
		case int:
			if x >= 0 {
				return uint64(x), nil
			}
		case int64:
			if x >= 0 {
				return uint64(x), nil
			}
		}
		return 0, fmt.Errorf("SNMP Get sysUpTime: неожиданный тип %T", v.Value)
	}
	return 0, fmt.Errorf("SNMP Get: sysUpTime отсутствует в ответе")
}

// SanitizeSNMPValue приводит SNMP/trap-значение к безопасной UTF-8 строке для JSONB/PostgreSQL.
func SanitizeSNMPValue(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return sanitizeSNMPBytes(x)
	case string:
		return sanitizeSNMPBytes([]byte(x))
	default:
		return sanitizeSNMPBytes([]byte(fmt.Sprint(x)))
	}
}

func pduString(p gosnmp.SnmpPDU) string {
	return SanitizeSNMPValue(p.Value)
}

// sanitizeSNMPBytes делает значение безопасным для PostgreSQL UTF-8.
// Бинарный chassis ID (часто 6 байт MAC) → aa:bb:…; прочий мусор → hex.
//
// Важно: 4 печатаемых ASCII-байта (например ifName EdgeSwitch "0/24") — это текст,
// а не IPv4. Раньше любой len==4 превращали в "48.47.50.52", из‑за чего порты
// 0/10+ отображались как «IP» и путали топологию.
func sanitizeSNMPBytes(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// Печатный UTF-8/ASCII без NUL — обычные ifName/sysName/Port ID ("0/24", "gi1/0/3").
	if utf8.Valid(b) && !containsNUL(b) && isMostlyPrintableASCII(b) {
		return string(b)
	}
	// LLDP networkAddress без subtype: family IPv4 (1) + 4 октета (не MAC).
	if len(b) == 5 && b[0] == 1 {
		return fmt.Sprintf("%d.%d.%d.%d", b[1], b[2], b[3], b[4])
	}
	// Сырой IPv4 (непечатные октеты) — только если это не ASCII-текст.
	if len(b) == 4 && !isMostlyPrintableASCII(b) {
		return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
	}
	if len(b) == 6 {
		printable := 0
		for _, c := range b {
			if c >= 0x20 && c < 0x7f {
				printable++
			}
		}
		if printable < 4 {
			return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", b[0], b[1], b[2], b[3], b[4], b[5])
		}
	}
	if utf8.Valid(b) && !containsNUL(b) {
		return string(b)
	}
	return hex.EncodeToString(b)
}

func isMostlyPrintableASCII(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c < 0x20 || c >= 0x7f {
			return false
		}
	}
	return true
}

func containsNUL(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return true
		}
	}
	return false
}

func normalizeOID(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), ".")
}
