package poller

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/models"
)

func TestTonerPctByKey(t *testing.T) {
	y := float32(6)
	b := float32(35)
	m := tonerPctByKey([]models.PrinterToner{
		{Key: "Yellow", Pct: &y},
		{Key: "black", Pct: &b},
		{Key: "cyan", Pct: nil},
	})
	if m["yellow"] != 6 || m["black"] != 35 {
		t.Fatalf("got %#v", m)
	}
	if _, ok := m["cyan"]; ok {
		t.Fatal("nil pct must be skipped")
	}
}

func TestTonerThresholdCrossings(t *testing.T) {
	pct := func(v float32) *float32 { return &v }
	prev := []models.PrinterToner{{Key: "yellow", Label: "Жёлтый", Pct: pct(6)}}
	cur := []models.PrinterToner{{Key: "yellow", Label: "Жёлтый", Pct: pct(4)}}
	got := tonerThresholdCrossings(prev, cur)
	if len(got) != 1 || got[0].EventType != "TONER_LOW" {
		t.Fatalf("low: %#v", got)
	}
	curCrit := []models.PrinterToner{{Key: "yellow", Pct: pct(1)}}
	got = tonerThresholdCrossings(prev, curCrit)
	if len(got) != 2 || got[0].EventType != "TONER_LOW" || got[1].EventType != "TONER_CRITICAL" {
		t.Fatalf("jump both: %#v", got)
	}
	got = tonerThresholdCrossings(nil, cur)
	if len(got) != 0 {
		t.Fatalf("first sample must not alert: %#v", got)
	}
	got = tonerThresholdCrossings(
		[]models.PrinterToner{{Key: "yellow", Pct: pct(3)}},
		[]models.PrinterToner{{Key: "yellow", Pct: pct(1)}},
	)
	if len(got) != 1 || got[0].EventType != "TONER_CRITICAL" || got[0].Severity != "error" {
		t.Fatalf("only critical: %#v", got)
	}
}
