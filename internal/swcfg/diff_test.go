package swcfg

import "testing"

func TestLineDiffBasic(t *testing.T) {
	old := "interface Gi1\n description old\n shutdown\n"
	new := "interface Gi1\n description new\n no shutdown\n"
	lines := LineDiff(old, new)
	var ins, del int
	for _, l := range lines {
		switch l.Kind {
		case DiffInsert:
			ins++
		case DiffDelete:
			del++
		}
	}
	if ins < 2 || del < 2 {
		t.Fatalf("expected inserts/deletes, got %+v stats=%+v", lines, DiffStatsFrom(lines))
	}
}

func TestLineDiffIdentical(t *testing.T) {
	text := "hostname sw1\nvlan 10\n"
	lines := LineDiff(text, text)
	for _, l := range lines {
		if l.Kind != DiffEqual {
			t.Fatalf("want all equal, got %+v", lines)
		}
	}
}
