package store

import (
	"strings"
	"testing"
)

func TestNormalizeDescrOverride(t *testing.T) {
	if NormalizeDescrOverride("  ") != nil {
		t.Fatal("blank should be nil")
	}
	got := NormalizeDescrOverride("  камера двор  ")
	if got == nil || *got != "камера двор" {
		t.Fatalf("got %#v", got)
	}
	long := strings.Repeat("я", 250)
	got = NormalizeDescrOverride(long)
	if got == nil || len([]rune(*got)) != 200 {
		t.Fatalf("len=%d", len([]rune(*got)))
	}
}

func TestInterfaceSnapshotDisplayDescr(t *testing.T) {
	snmp := "Gi0/1"
	ov := "камера"
	s := InterfaceSnapshot{IfDescr: &snmp, DescrOverride: &ov}
	if s.DisplayDescr() != "камера" {
		t.Fatalf("got %q", s.DisplayDescr())
	}
	s.DescrOverride = nil
	if s.DisplayDescr() != "Gi0/1" {
		t.Fatalf("snmp: %q", s.DisplayDescr())
	}
}
