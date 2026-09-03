package poller

import "testing"

func TestPoeMapHasActive(t *testing.T) {
	if poeMapHasActive(nil) {
		t.Fatal("nil")
	}
	if poeMapHasActive(map[int]bool{}) {
		t.Fatal("empty")
	}
	if poeMapHasActive(map[int]bool{1: false, 2: false}) {
		t.Fatal("all false should not count as active")
	}
	if !poeMapHasActive(map[int]bool{1: false, 3: true}) {
		t.Fatal("expected active")
	}
}

func TestMergePoeMapTrueWins(t *testing.T) {
	dst := map[int]bool{1: false}
	mergePoeMap(dst, map[int]bool{1: true, 2: false})
	if !dst[1] {
		t.Fatal("true should win")
	}
	if dst[2] {
		t.Fatal("new false")
	}
}
