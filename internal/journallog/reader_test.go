package journallog

import "testing"

func TestValidateTime(t *testing.T) {
	ok, err := validateTime("2026-08-20")
	if err != nil || ok != "2026-08-20" {
		t.Fatalf("date: %q %v", ok, err)
	}
	ok, err = validateTime("2026-08-20T14:30")
	if err != nil || ok != "2026-08-20 14:30" {
		t.Fatalf("datetime: %q %v", ok, err)
	}
	if _, err := validateTime("tomorrow"); err == nil {
		t.Fatal("expected error")
	}
}

func TestMatchCategories(t *testing.T) {
	if !matchCategories("level=WARN msg=\"poe ssh fallback\"", []string{"poe"}) {
		t.Fatal("poe")
	}
	if matchCategories("poll device ok", []string{"poe", "backup"}) {
		t.Fatal("should not match")
	}
	if !matchCategories("anything", nil) {
		t.Fatal("empty needles = all")
	}
}

func TestNormalizeUnit(t *testing.T) {
	if normalizeUnit("") != DefaultUnit {
		t.Fatal("default")
	}
	if normalizeUnit("evil;rm -rf") != DefaultUnit {
		t.Fatal("reject injection")
	}
}
