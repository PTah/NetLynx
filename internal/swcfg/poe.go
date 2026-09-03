package swcfg

import (
	"fmt"
	"strings"
)

// NormalizePoEMode: off | 24v | poe+ (пустая строка → ошибка).
func NormalizePoEMode(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "")
	switch s {
	case "off", "shutdown", "disabled", "0":
		return "off", nil
	case "24v", "24", "passive24v", "passive":
		return "24v", nil
	case "poe+", "poe", "auto", "af", "at", "802.3af", "802.3at":
		return "poe+", nil
	default:
		return "", fmt.Errorf("неизвестный poe_mode %q (ожидается off, 24v, poe+)", raw)
	}
}

// UbiquitiPoEOpmodeCLI — команда EdgeSwitch Fastpath (не XP).
func UbiquitiPoEOpmodeCLI(mode string) (string, error) {
	m, err := NormalizePoEMode(mode)
	if err != nil {
		return "", err
	}
	switch m {
	case "off":
		return "poe opmode shutdown", nil
	case "24v":
		return "poe opmode passive24v", nil
	case "poe+":
		return "poe opmode auto", nil
	default:
		return "", fmt.Errorf("poe_mode %q", m)
	}
}
