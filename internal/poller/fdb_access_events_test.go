package poller

import (
	"testing"
)

func roleFromMap(m map[int]string) func(int) string {
	return func(ifIdx int) string {
		if r, ok := m[ifIdx]; ok {
			return r
		}
		return "access"
	}
}

func eventTypes(events []fdbAccessEvent) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.eventType
	}
	return out
}

func hasEvent(events []fdbAccessEvent, typ, mac string, ifIndex int) bool {
	for _, e := range events {
		if e.eventType == typ && e.mac == mac && e.ifIndex == ifIndex {
			return true
		}
	}
	return false
}

func TestComputeFDBAccessEvents_AccessToAccess(t *testing.T) {
	prev := map[string]int{"aa:bb:cc:dd:ee:01": 5}
	cur := map[string]int{"aa:bb:cc:dd:ee:01": 8}
	roles := roleFromMap(map[int]string{5: "access", 8: "access"})
	ev := computeFDBAccessEvents(prev, cur, roles, nil, nil)
	if !hasEvent(ev, "MAC_MOVED", "aa:bb:cc:dd:ee:01", 8) {
		t.Fatalf("want MAC_MOVED, got %+v", ev)
	}
}

func TestComputeFDBAccessEvents_TrunkToAccess(t *testing.T) {
	prev := map[string]int{"aa:bb:cc:dd:ee:01": 48}
	cur := map[string]int{"aa:bb:cc:dd:ee:01": 5}
	roles := roleFromMap(map[int]string{48: "trunk", 5: "access"})
	ev := computeFDBAccessEvents(prev, cur, roles, nil, nil)
	if !hasEvent(ev, "UNKNOWN_MAC_ON_ACCESS_PORT", "aa:bb:cc:dd:ee:01", 5) {
		t.Fatalf("want UNKNOWN on trunk→access, got %+v", ev)
	}
}

func TestComputeFDBAccessEvents_SkipUnknownInventoryMAC(t *testing.T) {
	prev := map[string]int{}
	cur := map[string]int{"4c:cc:6a:d7:a3:cd": 30}
	roles := roleFromMap(map[int]string{30: "access"})
	isInv := func(mac string) bool { return mac == "4c:cc:6a:d7:a3:cd" }
	ev := computeFDBAccessEvents(prev, cur, roles, isInv, nil)
	if len(ev) != 0 {
		t.Fatalf("inventory MAC should not emit UNKNOWN, got %+v", ev)
	}
}

func TestComputeFDBAccessEvents_SkipUnknownBoundPortMAC(t *testing.T) {
	prev := map[string]int{}
	cur := map[string]int{"4c:cc:6a:d7:a3:cd": 30}
	roles := roleFromMap(map[int]string{30: "access"})
	isKnown := func(mac string, ifIndex int) bool {
		return ifIndex == 30 && mac == "4c:cc:6a:d7:a3:cd"
	}
	ev := computeFDBAccessEvents(prev, cur, roles, nil, isKnown)
	if len(ev) != 0 {
		t.Fatalf("bound port MAC should not emit UNKNOWN, got %+v", ev)
	}
}

func TestComputeFDBAccessEvents_SkipUnknownAfterTrunkToAccessIfKnown(t *testing.T) {
	prev := map[string]int{"4c:cc:6a:d7:a3:cd": 48}
	cur := map[string]int{"4c:cc:6a:d7:a3:cd": 30}
	roles := roleFromMap(map[int]string{48: "trunk", 30: "access"})
	isInv := func(mac string) bool { return mac == "4c:cc:6a:d7:a3:cd" }
	ev := computeFDBAccessEvents(prev, cur, roles, isInv, nil)
	if len(ev) != 0 {
		t.Fatalf("inventory MAC trunk→access should be silent, got %+v", ev)
	}
}

func TestComputeFDBAccessEvents_SkipTrunkOnly(t *testing.T) {
	prev := map[string]int{"aa:bb:cc:dd:ee:01": 48}
	cur := map[string]int{"aa:bb:cc:dd:ee:01": 49}
	roles := roleFromMap(map[int]string{48: "trunk", 49: "trunk"})
	ev := computeFDBAccessEvents(prev, cur, roles, nil, nil)
	if len(ev) != 0 {
		t.Fatalf("trunk-only should be silent, got %+v", ev)
	}
}

func TestComputeFDBAccessEvents_RemovedFromAccess(t *testing.T) {
	prev := map[string]int{"aa:bb:cc:dd:ee:01": 5}
	cur := map[string]int{}
	roles := roleFromMap(map[int]string{5: "access"})
	ev := computeFDBAccessEvents(prev, cur, roles, nil, nil)
	if !hasEvent(ev, "MAC_REMOVED", "aa:bb:cc:dd:ee:01", 5) {
		t.Fatalf("want MAC_REMOVED, got %+v", ev)
	}
}

func TestComputeFDBAccessEvents_SkipRemovedInventoryMAC(t *testing.T) {
	prev := map[string]int{"00:15:5d:01:02:03": 5}
	cur := map[string]int{}
	roles := roleFromMap(map[int]string{5: "access"})
	isInv := func(mac string) bool { return mac == "00:15:5d:01:02:03" }
	ev := computeFDBAccessEvents(prev, cur, roles, isInv, nil)
	if len(ev) != 0 {
		t.Fatalf("inventory MAC should not emit REMOVED, got %+v", ev)
	}
}

func TestComputeFDBAccessEvents_AccessToTrunk(t *testing.T) {
	prev := map[string]int{"aa:bb:cc:dd:ee:01": 5}
	cur := map[string]int{"aa:bb:cc:dd:ee:01": 48}
	roles := roleFromMap(map[int]string{5: "access", 48: "trunk"})
	ev := computeFDBAccessEvents(prev, cur, roles, nil, nil)
	if !hasEvent(ev, "MAC_REMOVED", "aa:bb:cc:dd:ee:01", 5) {
		t.Fatalf("access→trunk should REMOVED from access, got %+v", ev)
	}
	if hasEvent(ev, "MAC_MOVED", "aa:bb:cc:dd:ee:01", 48) {
		t.Fatal("should not MAC_MOVED to trunk")
	}
}

func TestComputeFDBAccessEvents_SkipMovedFromTrunk(t *testing.T) {
	prev := map[string]int{"aa:bb:cc:dd:ee:01": 48}
	cur := map[string]int{"aa:bb:cc:dd:ee:01": 49}
	roles := roleFromMap(map[int]string{48: "trunk", 49: "trunk"})
	ev := computeFDBAccessEvents(prev, cur, roles, nil, nil)
	if len(ev) != 0 {
		t.Fatalf("trunk→trunk should be silent, got types %v", eventTypes(ev))
	}
}
