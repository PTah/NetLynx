package store

import "testing"

func TestNormalizeManualLinkEnds(t *testing.T) {
	a, ai, b, bi, err := NormalizeManualLinkEnds(10, 3, 5, 7)
	if err != nil {
		t.Fatal(err)
	}
	if a != 5 || ai != 7 || b != 10 || bi != 3 {
		t.Fatalf("want swap to 5/7 — 10/3, got %d/%d — %d/%d", a, ai, b, bi)
	}
	_, _, _, _, err = NormalizeManualLinkEnds(1, 1, 1, 2)
	if err == nil {
		t.Fatal("same device should fail")
	}
}

func TestParseRemotePortIfIndex(t *testing.T) {
	cases := []struct {
		in   string
		want int
		ok   bool
	}{
		{"0/13", 13, true},
		{"GigabitEthernet0/24", 24, true},
		{"Port: 8", 8, true},
		{"15", 15, true},
		{"", 0, false},
		{"ethernet1/0/12", 12, true},
	}
	for _, c := range cases {
		got, ok := ParseRemotePortIfIndex(c.in)
		if ok != c.ok || got != c.want {
			t.Fatalf("%q: got %d/%v want %d/%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestManualLinkMatchesNeighbor(t *testing.T) {
	link := ManualTopologyLink{
		ADeviceID: 5, AIfIndex: 10,
		BDeviceID: 8, BIfIndex: 20,
		Status: ManualLinkStatusActive,
	}
	if !ManualLinkMatchesNeighbor(link, 5, 10, 8, "0/20") {
		t.Fatal("exact port match")
	}
	if ManualLinkMatchesNeighbor(link, 5, 10, 8, "0/99") {
		t.Fatal("wrong remote port must not match when parseable")
	}
	if !ManualLinkMatchesNeighbor(link, 5, 10, 8, "weird-port-id") {
		t.Fatal("unparseable remote port: match by devices+local if")
	}
	if ManualLinkMatchesNeighbor(link, 5, 11, 8, "0/20") {
		t.Fatal("wrong local if")
	}
	link.Status = ManualLinkStatusSuperseded
	if ManualLinkMatchesNeighbor(link, 5, 10, 8, "0/20") {
		t.Fatal("superseded must not match")
	}
}
