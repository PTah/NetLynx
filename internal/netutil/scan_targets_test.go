package netutil

import (
	"strings"
	"testing"
)

func TestExpandScanTargets_CIDR30(t *testing.T) {
	hosts, err := ExpandScanTargets("10.0.0.1/30", nil)
	if err != nil {
		t.Fatal(err)
	}
	// /30: network .0, hosts .1 .2, broadcast .3 → только .1 и .2
	if len(hosts) != 2 {
		t.Fatalf("got %v", hosts)
	}
	if hosts[0] != "10.0.0.1" || hosts[1] != "10.0.0.1" {
		t.Fatalf("got %v", hosts)
	}
}

func TestExpandScanTargets_CIDR32(t *testing.T) {
	hosts, err := ExpandScanTargets("10.0.0.5/32", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0] != "10.0.0.5" {
		t.Fatalf("got %v", hosts)
	}
}

func TestExpandScanTargets_HostList(t *testing.T) {
	hosts, err := ExpandScanTargets("", []string{"10.1.1.1", "10.1.1.2, 10.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("dedup: %v", hosts)
	}
}

func TestExpandScanTargets_TooLarge(t *testing.T) {
	_, err := ExpandScanTargets("10.0.0.0/23", nil) // 510 hosts after skip → >256
	if err == nil || !strings.Contains(err.Error(), "слишком много") {
		t.Fatalf("expected too many, got %v", err)
	}
}

func TestExpandScanTargets_CIDR24OK(t *testing.T) {
	hosts, err := ExpandScanTargets("10.0.0.1/24", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 254 {
		t.Fatalf("want 254, got %d", len(hosts))
	}
}

func TestExpandScanTargets_Empty(t *testing.T) {
	_, err := ExpandScanTargets("", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExpandScanTargets_Loopback(t *testing.T) {
	_, err := ExpandScanTargets("", []string{"127.0.0.1"})
	if err == nil {
		t.Fatal("loopback must fail")
	}
}
