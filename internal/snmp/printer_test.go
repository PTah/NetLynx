package snmp

import "testing"

func TestTonerPercentFromCurrentMax(t *testing.T) {
	pct, unknown, some := tonerPercent(1680, true, 12000, true, 7)
	if unknown || some || pct == nil {
		t.Fatalf("expected pct, got unknown=%v some=%v pct=%v", unknown, some, pct)
	}
	want := float32(1680) * 100 / 12000
	if *pct < want-0.1 || *pct > want+0.1 {
		t.Fatalf("pct=%v want ~%v", *pct, want)
	}
}

func TestTonerPercentAlreadyPercent(t *testing.T) {
	pct, unknown, some := tonerPercent(42, true, -2, true, prtSuppliesUnitPercent)
	if unknown || some || pct == nil || *pct != 42 {
		t.Fatalf("got pct=%v unknown=%v some=%v", pct, unknown, some)
	}
}

func TestTonerPercentSpecials(t *testing.T) {
	_, unknown, some := tonerPercent(prtLevelUnknown, true, 100, true, prtSuppliesUnitPercent)
	if !unknown || some {
		t.Fatalf(" -2 should be unknown")
	}
	_, unknown, some = tonerPercent(prtLevelSome, true, 100, true, prtSuppliesUnitPercent)
	if unknown || !some {
		t.Fatalf(" -3 should be remaining_ok")
	}
}

func TestClassifyTonerKey(t *testing.T) {
	cases := map[string]string{
		"Black Toner Cartridge": "black",
		"cyan":                  "cyan",
		"Magenta":               "magenta",
		"Yellow Toner":          "yellow",
		"чёрный тонер":          "black",
		"waste toner":           "other",
	}
	for in, want := range cases {
		if got := classifyTonerKey(in); got != want {
			t.Errorf("%q: got %s want %s", in, got, want)
		}
	}
}

func TestPickPageCountMax(t *testing.T) {
	v := pickPageCount(map[string]int64{"1.1": 100, "1.2": 250, "2.1": -2})
	if v == nil || *v != 250 {
		t.Fatalf("got %v", v)
	}
}

func TestLooksLikePrinter(t *testing.T) {
	if !LooksLikePrinter("mfu", "") {
		t.Fatal("mfu category")
	}
	if !LooksLikePrinter("switch", "KYOCERA ECOSYS M2040dn") {
		t.Fatal("kyocera sysDescr")
	}
	if LooksLikePrinter("switch", "Cisco IOS Software, C2960") {
		t.Fatal("cisco is not a printer")
	}
}

func TestIsWasteDescr(t *testing.T) {
	if !isWasteDescr("Waste Toner Box") {
		t.Fatal("waste")
	}
	if isWasteDescr("Black Toner") {
		t.Fatal("black is not waste")
	}
}
