package store

import "testing"

func TestEscapeILIKE(t *testing.T) {
	if escapeILIKE(`100%`) != `100\%` {
		t.Fatal("escape percent")
	}
}

func TestAttachMACVendors(t *testing.T) {
	mac := "00:1f:55:aa:bb:cc"
	hits := []PortSearchHit{{MAC: &mac}}
	attachMACVendors(hits)
	if hits[0].MacVendor != "Honeywell Security (China) Co., Ltd." {
		t.Fatalf("got %q", hits[0].MacVendor)
	}
}
