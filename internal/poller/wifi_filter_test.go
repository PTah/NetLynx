package poller

import (
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func TestQEMULocallyAdminMACIsFullMAC(t *testing.T) {
	mac := "52:54:4c:83:09:e0"
	norm, ok := store.FormatFullMAC(mac)
	if !ok {
		t.Fatal("format")
	}
	if !store.IsLocallyAdministeredMAC(norm) {
		t.Fatal("expected LAA (QEMU)")
	}
}
