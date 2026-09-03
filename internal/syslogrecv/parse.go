package syslogrecv

import (
	"regexp"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

// MACFlapMsg — разобранное сообщение о MAC flapping.
type MACFlapMsg struct {
	MAC    string
	VLAN   *int
	PortA  string
	PortB  string
	Raw    string
}

// Eltex / SNR / похожие:
// %BRG_MACNTFY-I-MAC_FLAPPING: Host 52:54:4c:83:09:e0 in vlan 1 is flapping between port gi1/0/23 and port gi1/0/10
var reEltexFlap = regexp.MustCompile(`(?i)MAC_FLAPPING:\s*Host\s+([0-9a-fA-F:.\-]+)\s+in\s+vlan\s+(\d+)\s+is\s+flapping\s+between\s+port\s+(\S+)\s+and\s+port\s+(\S+)`)

// Cisco-подобные вариации:
// %SW_MATM-4-MACFLAP_NOTIF: Host ... in vlan ... is flapping between port ... and port ...
var reCiscoFlap = regexp.MustCompile(`(?i)MACFLAP(?:_NOTIF)?:.*?Host\s+([0-9a-fA-F:.\-]+)\s+in\s+vlan\s+(\d+)\s+is\s+flapping\s+between\s+port\s+(\S+)\s+and\s+port\s+(\S+)`)

// ParseMACFlapping извлекает flapping из текста syslog (после PRI/header).
func ParseMACFlapping(msg string) (*MACFlapMsg, bool) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil, false
	}
	for _, re := range []*regexp.Regexp{reEltexFlap, reCiscoFlap} {
		m := re.FindStringSubmatch(msg)
		if m == nil {
			continue
		}
		mac, ok := store.FormatFullMAC(m[1])
		if !ok {
			continue
		}
		vlan, _ := strconv.Atoi(m[2])
		out := &MACFlapMsg{
			MAC:   mac,
			PortA: strings.TrimRight(m[3], ",.;"),
			PortB: strings.TrimRight(m[4], ",.;"),
			Raw:   msg,
		}
		if vlan > 0 {
			out.VLAN = &vlan
		}
		return out, true
	}
	return nil, false
}

// StripSyslogHeader убирает <PRI> и типичный RFC3164/5424 префикс, оставляя тело.
func StripSyslogHeader(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "<") {
		if i := strings.IndexByte(s, '>'); i >= 0 && i < 6 {
			s = s[i+1:]
		}
	}
	// RFC5424: VERSION SP TIMESTAMP SP HOSTNAME SP APP-NAME SP PROCID SP MSGID SP STRUCTURED-DATA SP MSG
	if len(s) > 2 && s[0] >= '1' && s[0] <= '3' && s[1] == ' ' {
		// грубо: ищем " - " или первое "%" / "Host "
		if idx := strings.Index(s, " - "); idx >= 0 {
			return strings.TrimSpace(s[idx+3:])
		}
	}
	// RFC3164: TIMESTAMP HOST TAG: MSG — ищем %... или MAC_FLAPPING
	if i := strings.Index(s, "%"); i >= 0 {
		return strings.TrimSpace(s[i:])
	}
	if i := strings.Index(strings.ToUpper(s), "MAC_FLAPPING"); i >= 0 {
		// отступить к началу токена
		start := i
		for start > 0 && s[start-1] != ' ' && s[start-1] != ']' {
			start--
		}
		return strings.TrimSpace(s[start:])
	}
	if i := strings.Index(strings.ToUpper(s), "MACFLAP"); i >= 0 {
		start := i
		for start > 0 && s[start-1] != ' ' {
			start--
		}
		return strings.TrimSpace(s[start:])
	}
	return s
}
