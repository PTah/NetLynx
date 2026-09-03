package fdbsnapshot

import (
	"testing"
	"time"
)

func TestShouldSaveDaily(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	last := now.Add(-25 * time.Hour)
	if !ShouldSaveDaily(&last, now, 20*time.Hour) {
		t.Fatal("expected save after 25h")
	}
	recent := now.Add(-10 * time.Hour)
	if ShouldSaveDaily(&recent, now, 20*time.Hour) {
		t.Fatal("too soon")
	}
	if !ShouldSaveDaily(nil, now, 20*time.Hour) {
		t.Fatal("first snapshot")
	}
}
