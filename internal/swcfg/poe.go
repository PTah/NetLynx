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

// NormalizePoEResetSeconds — длительность сброса PoE (EdgeSwitch: 1–60 с).
func NormalizePoEResetSeconds(sec int) (int, error) {
	if sec < 1 || sec > 60 {
		return 0, fmt.Errorf("poe reset: длительность %d вне 1..60 с", sec)
	}
	return sec, nil
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

// UbiquitiPoEResetCLI — EdgeSwitch Fastpath: poe reset <1-60> (interface config).
func UbiquitiPoEResetCLI(seconds int) (string, error) {
	sec, err := NormalizePoEResetSeconds(seconds)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("poe reset %d", sec), nil
}

// PoEResetStrategy — как выполнять сброс PoE на вендоре.
type PoEResetStrategy int

const (
	// PoEResetNative — одна CLI-команда на порту (EdgeSwitch poe reset N).
	PoEResetNative PoEResetStrategy = iota
	// PoEResetPowerCycle — выкл → пауза → вкл (Cisco/Eltex/SNR и т.п.).
	PoEResetPowerCycle
	// PoEResetMikrotik — /interface ethernet poe power-cycle.
	PoEResetMikrotik
)

// PoEResetPlan — план сброса для вендора.
func PoEResetPlan(v Vendor) PoEResetStrategy {
	switch v {
	case VendorMikrotik:
		return PoEResetMikrotik
	case VendorUbiquiti:
		return PoEResetNative
	case VendorAuto:
		return PoEResetNative
	default:
		return PoEResetPowerCycle
	}
}
