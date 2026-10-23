package store

import (
	"testing"
	"time"
)

func TestComputePoEResetSlotsThreeFrom0300(t *testing.T) {
	slots := ComputePoEResetSlots(3, 0, 3)
	if len(slots) != 3 {
		t.Fatalf("len=%d", len(slots))
	}
	want := []string{"03:00", "11:00", "19:00"}
	for i, s := range slots {
		got := FormatPoEResetSlot(s)
		if got != want[i] {
			t.Fatalf("[%d] got %s want %s", i, got, want[i])
		}
	}
}

func TestComputePoEResetSlotsOnce(t *testing.T) {
	slots := ComputePoEResetSlots(14, 30, 1)
	if len(slots) != 1 || FormatPoEResetSlot(slots[0]) != "14:30" {
		t.Fatalf("%v", slots)
	}
}

func TestComputePoEResetSlotsTwiceFrom0000(t *testing.T) {
	slots := ComputePoEResetSlots(0, 0, 2)
	want := []string{"00:00", "12:00"}
	for i, s := range slots {
		if FormatPoEResetSlot(s) != want[i] {
			t.Fatalf("[%d] %s", i, FormatPoEResetSlot(s))
		}
	}
}

func TestPoEResetScheduleMatches(t *testing.T) {
	now := time.Date(2026, 10, 22, 11, 0, 15, 0, time.Local)
	if !PoEResetScheduleMatches(now, 3, 0, 3) {
		t.Fatal("expected match at 11:00")
	}
	now2 := time.Date(2026, 10, 22, 11, 1, 0, 0, time.Local)
	if PoEResetScheduleMatches(now2, 3, 0, 3) {
		t.Fatal("should not match 11:01")
	}
}

func TestNormalizePortPoEResetSchedule(t *testing.T) {
	r, err := NormalizePortPoEResetSchedule(PortPoEResetSchedule{
		Mode: PoEResetSchedAtTime, StartHour: 3, StartMinute: 0, TimesPerDay: 5, ResetSeconds: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.TimesPerDay != 1 || r.ResetSeconds != 10 {
		t.Fatalf("%+v", r)
	}
	r2, err := NormalizePortPoEResetSchedule(PortPoEResetSchedule{
		Mode: PoEResetSchedTimesPerDay, StartHour: 3, StartMinute: 0, TimesPerDay: 3, ResetSeconds: 15,
	})
	if err != nil || r2.TimesPerDay != 3 || r2.ResetSeconds != 15 {
		t.Fatalf("%+v %v", r2, err)
	}
}

func TestAttachPoEResetNextSlots(t *testing.T) {
	r := PortPoEResetSchedule{StartHour: 3, StartMinute: 0, TimesPerDay: 3}
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.Local)
	AttachPoEResetNextSlots(&r, now)
	if len(r.NextSlots) < 1 || r.NextSlots[0] != "19:00" {
		t.Fatalf("%v", r.NextSlots)
	}
}
