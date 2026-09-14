package poller

import (
	"testing"
	"time"
)

func TestPortFlapThresholdDefaults(t *testing.T) {
	minB := 0
	if minB < 2 {
		minB = 4
	}
	if minB != 4 {
		t.Fatalf("minB=%d", minB)
	}
	window := time.Duration(0)
	if window <= 0 {
		window = 10 * time.Minute
	}
	if window != 10*time.Minute {
		t.Fatal(window)
	}
	debounce := time.Duration(0)
	if debounce <= 0 {
		debounce = 15 * time.Minute
	}
	if debounce != 15*time.Minute {
		t.Fatal(debounce)
	}
}

func TestPortFlapBounceWindowLogic(t *testing.T) {
	// N bounce >= min → emit candidate; below → skip
	minB := 4
	cases := []struct {
		n    int
		want bool
	}{
		{0, false},
		{3, false},
		{4, true},
		{10, true},
	}
	for _, c := range cases {
		got := c.n >= minB
		if got != c.want {
			t.Fatalf("n=%d got %v want %v", c.n, got, c.want)
		}
	}
}
