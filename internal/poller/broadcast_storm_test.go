package poller

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
)

func TestFDBGrowing(t *testing.T) {
	cfg := config.Config{
		BroadcastStormFDBMinGrowth:    5,
		BroadcastStormFDBMinGrowthPct: 2,
	}
	if ok, d := fdbGrowing(cfg, 100, 101); ok || d != 1 {
		t.Fatalf("1 MAC on 100 below abs and pct: got ok=%v delta=%d", ok, d)
	}
	if ok, d := fdbGrowing(cfg, 100, 106); !ok || d != 6 {
		t.Fatalf("6 MACs: got ok=%v delta=%d", ok, d)
	}
	if ok, d := fdbGrowing(cfg, 200, 205); !ok || d != 5 {
		t.Fatalf("5 MACs absolute: got ok=%v delta=%d", ok, d)
	}
	if ok, _ := fdbGrowing(cfg, 100, 100); ok {
		t.Fatal("no growth")
	}
	if ok, _ := fdbGrowing(cfg, 0, 10); ok {
		t.Fatal("prev=0")
	}
}

func TestBroadcastStormSuspected(t *testing.T) {
	cfg := config.Config{
		BroadcastStormMinPorts:        3,
		BroadcastStormUtilPct:         80,
		BroadcastStormFDBMinGrowth:    5,
		BroadcastStormFDBMinGrowthPct: 2,
	}
	ports := []stormUtilPort{{IfIndex: 1}, {IfIndex: 2}, {IfIndex: 3}}
	ok, delta := broadcastStormSuspected(cfg, ports[:2], 100, 110)
	if ok || delta != 0 {
		t.Fatal("need 3 ports")
	}
	ok, delta = broadcastStormSuspected(cfg, ports, 100, 101)
	if ok {
		t.Fatalf("need FDB growth, got delta=%d", delta)
	}
	ok, delta = broadcastStormSuspected(cfg, ports, 100, 110)
	if !ok || delta != 10 {
		t.Fatalf("expected suspected, delta=10 got ok=%v delta=%d", ok, delta)
	}
}
