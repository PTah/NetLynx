package store

import "testing"

func TestNormalizeLinkTrapEventsMode(t *testing.T) {
	cases := map[string]string{
		"":           LinkTrapEventsOff,
		"OFF":        LinkTrapEventsOff,
		"per_device": LinkTrapEventsPerDevice,
		"ALL":        LinkTrapEventsAll,
		"junk":       LinkTrapEventsOff,
	}
	for in, want := range cases {
		if got := NormalizeLinkTrapEventsMode(in); got != want {
			t.Fatalf("NormalizeLinkTrapEventsMode(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeLinkTrapEffects(t *testing.T) {
	if NormalizeLinkTrapEffects("full") != LinkTrapEffectsFull {
		t.Fatal("full")
	}
	if NormalizeLinkTrapEffects("") != LinkTrapEffectsNotify {
		t.Fatal("default notify")
	}
	if NormalizeLinkTrapEffects("NOTIFY") != LinkTrapEffectsNotify {
		t.Fatal("notify")
	}
}

func TestAllowLinkTrapEvents(t *testing.T) {
	if AllowLinkTrapEvents(LinkTrapEventsOff, true, true) {
		t.Fatal("off must deny")
	}
	if AllowLinkTrapEvents(LinkTrapEventsAll, false, false) {
		t.Fatal("all without device must deny")
	}
	if !AllowLinkTrapEvents(LinkTrapEventsAll, true, false) {
		t.Fatal("all with device must allow")
	}
	if AllowLinkTrapEvents(LinkTrapEventsPerDevice, true, false) {
		t.Fatal("per_device without flag must deny")
	}
	if !AllowLinkTrapEvents(LinkTrapEventsPerDevice, true, true) {
		t.Fatal("per_device with flag must allow")
	}
}
