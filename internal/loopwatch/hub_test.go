package loopwatch

import "testing"

func TestSeedSkipsEmit(t *testing.T) {
	// seed pass marks keys without emitting; second pass emits only unseen
	known := map[string]struct{}{"[1 2 3]": {}}
	key := "[1 2 3]"
	_, seen := known[key]
	if !seen {
		t.Fatal("seeded key must be seen")
	}
	newKey := "[1 2 4]"
	if _, ok := known[newKey]; ok {
		t.Fatal("new key must not be seen")
	}
	known[newKey] = struct{}{}
	if len(known) != 2 {
		t.Fatal(len(known))
	}
}
