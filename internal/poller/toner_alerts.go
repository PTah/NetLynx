package poller

import (
	"context"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

const (
	tonerLowPct      = float32(5)
	tonerCriticalPct = float32(2)
)

type tonerCross struct {
	Key, Label, Description string
	Pct                     float32
	EventType               string
	Severity                string
	Threshold               float32
}

// tonerThresholdCrossings — пересечения 5% / 2% при падении относительно предыдущего опроса.
func tonerThresholdCrossings(prev []models.PrinterToner, cur []models.PrinterToner) []tonerCross {
	if len(cur) == 0 {
		return nil
	}
	prevByKey := tonerPctByKey(prev)
	var out []tonerCross
	for _, t := range cur {
		if t.Pct == nil {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(t.Key))
		if key == "" {
			key = "other"
		}
		now := *t.Pct
		prevPct, hadPrev := prevByKey[key]
		if !hadPrev {
			// Первый снимок — без алерта (иначе спам при добавлении узла).
			continue
		}
		label := strings.TrimSpace(t.Label)
		if label == "" {
			label = key
		}
		base := tonerCross{Key: key, Label: label, Description: strings.TrimSpace(t.Description), Pct: now}
		if prevPct > tonerLowPct && now <= tonerLowPct {
			c := base
			c.EventType = "TONER_LOW"
			c.Severity = "warning"
			c.Threshold = tonerLowPct
			out = append(out, c)
		}
		if prevPct > tonerCriticalPct && now <= tonerCriticalPct {
			c := base
			c.EventType = "TONER_CRITICAL"
			c.Severity = "error"
			c.Threshold = tonerCriticalPct
			out = append(out, c)
		}
	}
	return out
}

// emitTonerThresholdAlerts — при пересечении порогов 5% и 2% (падение с предыдущего опроса).
func (e *Engine) emitTonerThresholdAlerts(ctx context.Context, d store.PollDevice, prev []models.PrinterToner, cur *snmp.PrinterReading) {
	if e == nil || cur == nil {
		return
	}
	for _, c := range tonerThresholdCrossings(prev, cur.Toners) {
		pl := map[string]interface{}{
			"toner_key":     c.Key,
			"toner_label":   c.Label,
			"pct":           c.Pct,
			"threshold_pct": c.Threshold,
		}
		if c.Description != "" {
			pl["description"] = c.Description
		}
		e.emit(ctx, nil, d, nil, c.EventType, c.Severity, pl)
	}
}

func tonerPctByKey(toners []models.PrinterToner) map[string]float32 {
	out := make(map[string]float32, len(toners))
	for _, t := range toners {
		if t.Pct == nil {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(t.Key))
		if key == "" {
			key = "other"
		}
		out[key] = *t.Pct
	}
	return out
}
