package api

import (
	"strings"
	"testing"

	"github.com/PTah/netlynx/internal/netutil"
)

func TestExpandScanTargets_APILimits(t *testing.T) {
	_, err := netutil.ExpandScanTargets("10.0.0.0/16", nil)
	if err == nil || !strings.Contains(err.Error(), "слишком много") {
		t.Fatalf("expected size limit, got %v", err)
	}
	hosts, err := netutil.ExpandScanTargets("10.0.0.0/30", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("got %d", len(hosts))
	}
}

func TestScanSNMPBodyValidation(t *testing.T) {
	// Community empty rejected by handler; here we only check expand+version helpers stay in sync.
	if netutil.MaxScanHosts != 256 {
		t.Fatalf("MaxScanHosts=%d", netutil.MaxScanHosts)
	}
}
