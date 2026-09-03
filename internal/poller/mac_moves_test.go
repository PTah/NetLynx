package poller

import (
	"testing"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/snmp"
)

func TestComputeMACFDBMoves(t *testing.T) {
	now := time.Now()
	vlan1 := 1
	entries := map[string]snmp.FDBLearnedEntry{
		"aa:bb:cc:dd:ee:01": {IfIndex: 10, VLANID: &vlan1},
		"aa:bb:cc:dd:ee:02": {IfIndex: 20, VLANID: &vlan1},
	}
	prev := map[string]int{
		"aa:bb:cc:dd:ee:01": 5,
		"aa:bb:cc:dd:ee:03": 7,
	}
	cur := map[string]int{
		"aa:bb:cc:dd:ee:01": 10,
		"aa:bb:cc:dd:ee:02": 20,
	}
	moves := computeMACFDBMoves(42, prev, cur, entries, now, "")
	if len(moves) != 3 {
		t.Fatalf("want 3 moves, got %d: %+v", len(moves), moves)
	}
	var sawMove, sawAppear, sawLeave bool
	for _, m := range moves {
		switch m.MAC {
		case "aa:bb:cc:dd:ee:01":
			if m.FromIfIndex == nil || m.ToIfIndex == nil || *m.FromIfIndex != 5 || *m.ToIfIndex != 10 {
				t.Fatalf("move: %+v", m)
			}
			sawMove = true
		case "aa:bb:cc:dd:ee:02":
			if m.FromIfIndex != nil || m.ToIfIndex == nil || *m.ToIfIndex != 20 {
				t.Fatalf("appear: %+v", m)
			}
			sawAppear = true
		case "aa:bb:cc:dd:ee:03":
			if m.ToIfIndex != nil || m.FromIfIndex == nil || *m.FromIfIndex != 7 {
				t.Fatalf("leave: %+v", m)
			}
			sawLeave = true
		}
	}
	if !sawMove || !sawAppear || !sawLeave {
		t.Fatalf("missing kinds move=%v appear=%v leave=%v", sawMove, sawAppear, sawLeave)
	}
}
