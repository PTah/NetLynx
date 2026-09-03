package poller

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
)

func TestNeighborEmptyStreakConfirm(t *testing.T) {
	e := New(nil, nil, config.Config{}, nil, nil)
	if got := e.bumpNeighborEmptyStreak(42, "lldp"); got != 1 {
		t.Fatalf("first bump: %d", got)
	}
	if got := e.bumpNeighborEmptyStreak(42, "lldp"); got != 2 {
		t.Fatalf("second bump: %d", got)
	}
	e.clearNeighborEmptyStreak(42, "lldp")
	if got := e.bumpNeighborEmptyStreak(42, "lldp"); got != 1 {
		t.Fatalf("after clear: %d", got)
	}
	if neighborEmptyKey(42, "LLDP") != "42:lldp" {
		t.Fatal("key normalize")
	}
}
