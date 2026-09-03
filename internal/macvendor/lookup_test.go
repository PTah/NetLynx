package macvendor

import (
	"strings"
	"testing"
)

func TestLookupKnownVendors(t *testing.T) {
	got := Lookup("00:1F:55:aa:bb:cc")
	if got != "Honeywell Security (China) Co., Ltd." {
		t.Fatalf("Honeywell: got %q", got)
	}
	dahua := Lookup("74-C9-29-01-02-03")
	if !strings.Contains(strings.ToLower(dahua), "dahua") {
		t.Fatalf("Dahua: got %q", dahua)
	}
	hik := Lookup("0c75d2abcdef")
	if !strings.Contains(strings.ToLower(hik), "hikvision") {
		t.Fatalf("Hikvision: got %q", hik)
	}
}

func TestLookupUnknownAndShort(t *testing.T) {
	if Lookup("") != "" || Lookup("aa:bb") != "" {
		t.Fatal("short MAC should be empty")
	}
	if Lookup("ff:ff:ff:ff:ff:ff") != "" {
		t.Fatal("broadcast should not map to a vendor")
	}
}

func TestLookupCaseAndSeparators(t *testing.T) {
	a := Lookup("001F55AABBCC")
	b := Lookup("00-1f-55-aa-bb-cc")
	c := Lookup("00:1F:55:AA:BB:CC")
	if a == "" || a != b || b != c {
		t.Fatalf("normalize: %q %q %q", a, b, c)
	}
}

func TestMaskOUI24(t *testing.T) {
	if maskOUI24("001f55") != "001f55" {
		t.Fatalf("unchanged: %q", maskOUI24("001f55"))
	}
	if maskOUI24("021f55") != "001f55" {
		t.Fatalf("U/L bit: %q", maskOUI24("021f55"))
	}
	if maskOUI24("011f55") != "001f55" {
		t.Fatalf("I/G bit: %q", maskOUI24("011f55"))
	}
}
