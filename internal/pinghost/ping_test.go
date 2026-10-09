package pinghost

import "testing"

func TestSanitizeHost(t *testing.T) {
	ok, err := sanitizeHost("10.0.0.1")
	if err != nil || ok != "10.0.0.1" {
		t.Fatalf("ip: %q %v", ok, err)
	}
	if _, err := sanitizeHost("evil;rm -rf /"); err == nil {
		t.Fatal("expected reject")
	}
	h, err := sanitizeHost("sw-core.lan")
	if err != nil || h != "sw-core.lan" {
		t.Fatalf("name: %q %v", h, err)
	}
}
