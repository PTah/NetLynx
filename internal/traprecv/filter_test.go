package traprecv

import "testing"

func TestParseIncludeLabels(t *testing.T) {
	t.Parallel()
	if ParseIncludeLabels("") != nil {
		t.Fatal("empty should be nil")
	}
	m := ParseIncludeLabels("linkUp, linkDown,linkUp")
	if len(m) != 2 {
		t.Fatalf("len=%d", len(m))
	}
	if !LabelMatchesInclude("linkUp", m) || LabelMatchesInclude("loginSessionStartStopTrap", m) {
		t.Fatal("filter mismatch")
	}
	if !LabelMatchesInclude("anything", nil) {
		t.Fatal("nil filter should allow all")
	}
}
