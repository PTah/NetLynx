package store

import (
	"encoding/hex"
	"strings"
)

// humanizeSNMPText — уже сохранённый if_descr: если это hex от печатного ASCII
// (старый баг Windows DisplayString с NUL → hex), показать читаемый текст.
func humanizeSNMPText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if decoded, ok := decodeHexPrintableASCII(s); ok {
		return decoded
	}
	return s
}

func decodeHexPrintableASCII(s string) (string, bool) {
	if len(s) < 16 || len(s)%2 != 0 {
		return "", false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return "", false
		}
	}
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) == 0 {
		return "", false
	}
	for len(raw) > 0 && raw[len(raw)-1] == 0 {
		raw = raw[:len(raw)-1]
	}
	if len(raw) < 8 {
		return "", false
	}
	for _, c := range raw {
		if c < 0x20 || c >= 0x7f {
			return "", false
		}
	}
	return string(raw), true
}
