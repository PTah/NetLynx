package store

import "git.kalinamall.ru/PapaTramp/netlynx/internal/config"

type UtilThresholds struct {
	HighPct float64
	OkPct   float64
}

// ResolveUtilThresholds: порт → узел → глобальный env.
func ResolveUtilThresholds(cfg config.Config, dev *PollDevice, port *InterfaceSnapshot) UtilThresholds {
	high := cfg.PortUtilHighPct
	ok := cfg.PortUtilOKPct
	if dev != nil {
		if dev.UtilHighPct != nil {
			high = float64(*dev.UtilHighPct)
		}
		if dev.UtilOkPct != nil {
			ok = float64(*dev.UtilOkPct)
		}
	}
	if port != nil {
		if port.UtilHighPct != nil {
			high = float64(*port.UtilHighPct)
		}
		if port.UtilOkPct != nil {
			ok = float64(*port.UtilOkPct)
		}
	}
	return UtilThresholds{HighPct: high, OkPct: ok}
}
