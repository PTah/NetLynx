package poller

import (
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
)

func TestMacMultiAccessDebounceDefault(t *testing.T) {
	e := &Engine{cfg: config.Config{}}
	if got := e.macMultiAccessDebounce(); got != 6*time.Hour {
		t.Fatalf("empty cfg: got %s want 6h", got)
	}
	e.cfg.MACFlapDebounce = 15 * time.Minute
	if got := e.macMultiAccessDebounce(); got != 6*time.Hour {
		t.Fatalf("short flap debounce must not shrink multi-access: got %s", got)
	}
	e.cfg.MACMultiAccessDebounce = 24 * time.Hour
	if got := e.macMultiAccessDebounce(); got != 24*time.Hour {
		t.Fatalf("explicit multi-access debounce: got %s", got)
	}
}
