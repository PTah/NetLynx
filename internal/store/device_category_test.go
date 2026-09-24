package store

import "testing"

func TestNormalizeDeviceCategoryVirtual(t *testing.T) {
	if got := NormalizeDeviceCategory("virtual"); got != DeviceCategoryOther {
		t.Fatalf("virtual → other, got %q", got)
	}
	if got := NormalizeDeviceCategory("Virtual"); got != DeviceCategoryOther {
		t.Fatalf("Virtual → other, got %q", got)
	}
}
