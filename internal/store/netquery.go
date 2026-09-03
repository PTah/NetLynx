package store

import (
	"net"
	"regexp"
	"strings"
)

// SearchQueryKind — тип строки поиска на странице «Узлы».
type SearchQueryKind int

const (
	SearchQueryLabel SearchQueryKind = iota
	SearchQueryMAC
	SearchQueryIP
)

var hexOnly = regexp.MustCompile(`^[0-9a-fA-F:]+$`)

// ClassifySearchQuery определяет IP, MAC или подпись порта.
func ClassifySearchQuery(raw string) (kind SearchQueryKind, normalized string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return SearchQueryLabel, ""
	}
	if ip := net.ParseIP(s); ip != nil && ip.To4() != nil {
		return SearchQueryIP, ip.To4().String()
	}
	if mac, ok := NormalizeMACQuery(s); ok {
		return SearchQueryMAC, mac
	}
	return SearchQueryLabel, s
}

// NormalizeMACQuery приводит ввод к aa:bb:cc:dd:ee:ff или "" если не похоже на MAC.
func NormalizeMACQuery(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if strings.Contains(s, ":") || strings.Contains(s, "-") || hexOnly.MatchString(s) {
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
		}, s)
		if len(hex) < 6 || len(hex) > 12 || len(hex)%2 != 0 {
			return "", false
		}
		if len(hex) == 12 {
			var b strings.Builder
			for i := 0; i < 12; i += 2 {
				if i > 0 {
					b.WriteByte(':')
				}
				b.WriteString(hex[i : i+2])
			}
			return b.String(), true
		}
		// частичный MAC — суффикс для LIKE
		var b strings.Builder
		for i := 0; i < len(hex); i += 2 {
			if i > 0 {
				b.WriteByte(':')
			}
			if i+2 <= len(hex) {
				b.WriteString(hex[i : i+2])
			} else {
				b.WriteString(hex[i:])
			}
		}
		return b.String(), true
	}
	return "", false
}

// FormatFullMAC возвращает aa:bb:cc:dd:ee:ff для полного 48-bit MAC, иначе false.
func FormatFullMAC(raw string) (string, bool) {
	mac, ok := NormalizeMACQuery(raw)
	if !ok {
		return "", false
	}
	n := 0
	for _, c := range mac {
		if c != ':' {
			n++
		}
	}
	if n != 12 {
		return "", false
	}
	return mac, true
}

// macHexDigits — только hex-цифры из строки (для стабильного identity).
func macHexDigits(raw string) string {
	return strings.Map(func(r rune) rune {
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
	}, raw)
}
