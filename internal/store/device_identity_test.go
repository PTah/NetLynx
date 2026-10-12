package store

import (
	"strings"
	"testing"
)

func TestDuplicateIdentityErrorMessage(t *testing.T) {
	e := &DuplicateIdentityError{Kind: "host", Value: "192.168.1.1", ExistingID: 5, ExistingName: "sw1"}
	if got := e.Error(); got != `IP/host 192.168.1.1 уже занят узлом «sw1» (id=5)` {
		t.Fatalf("%q", got)
	}
	e2 := &DuplicateIdentityError{Kind: "mac", Value: "aa:bb:cc:dd:ee:ff", ExistingID: 7, ExistingName: "pc"}
	got := e2.Error()
	for _, part := range []string{"MAC", "aa:bb:cc:dd:ee:ff", "pc", "id=7"} {
		if !strings.Contains(got, part) {
			t.Fatalf("missing %q in %q", part, got)
		}
	}
}
