package store

import "testing"

func TestFormatFullMAC(t *testing.T) {
	m, ok := FormatFullMAC("345A60F2E65A")
	if !ok || m != "34:5a:60:f2:e6:5a" {
		t.Fatalf("compact got %q %v", m, ok)
	}
	m, ok = FormatFullMAC("18-c0-4d-d6-fb-79")
	if !ok || m != "18:c0:4d:d6:fb:79" {
		t.Fatalf("dashed got %q %v", m, ok)
	}
	if _, ok := FormatFullMAC("aabbcc"); ok {
		t.Fatal("partial must fail")
	}
	if _, ok := FormatFullMAC("192.168.1.1"); ok {
		t.Fatal("ip must fail")
	}
}

func TestClassifySearchQuery(t *testing.T) {
	k, n := ClassifySearchQuery("192.168.1.50")
	if k != SearchQueryIP || n != "192.168.1.50" {
		t.Fatalf("ip %v %q", k, n)
	}
	k, n = ClassifySearchQuery("aa:bb:cc:dd:ee:ff")
	if k != SearchQueryMAC || n != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("mac %v %q", k, n)
	}
	k, n = ClassifySearchQuery("01c0a8aa49")
	if k != SearchQueryMAC || n != "01:c0:a8:aa:49" {
		t.Fatalf("partial10 %v %q", k, n)
	}
}
