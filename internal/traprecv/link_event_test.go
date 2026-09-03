package traprecv

import "testing"

func TestLinkEventType(t *testing.T) {
	typ, sev, ok := linkEventType("linkUp")
	if !ok || typ != "LINK_UP" || sev != "info" {
		t.Fatalf("linkUp: %v %v %v", typ, sev, ok)
	}
	typ, sev, ok = linkEventType("linkDown")
	if !ok || typ != "LINK_DOWN" || sev != "warning" {
		t.Fatalf("linkDown: %v %v %v", typ, sev, ok)
	}
	if _, _, ok = linkEventType("other"); ok {
		t.Fatal("other must fail")
	}
}

func TestLinkExpectedOper(t *testing.T) {
	op, ok := linkExpectedOper("linkUp")
	if !ok || op != 1 {
		t.Fatal("up")
	}
	op, ok = linkExpectedOper("linkDown")
	if !ok || op != 2 {
		t.Fatal("down")
	}
}
