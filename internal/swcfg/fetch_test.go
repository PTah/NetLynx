package swcfg

import (
	"fmt"
	"strings"
	"testing"
)

func TestTailHasPager(t *testing.T) {
	page := strings.Repeat("vlan 1\n", 20) + "--More-- or (q)uit"
	if !tailHasPager(page) {
		t.Fatal("more")
	}
	mid := "--More-- or (q)uit\n" + strings.Repeat("interface 0/10\n description foo\n", 8)
	if tailHasPager(mid) {
		t.Fatal("pager not at tail")
	}
}

func TestStripPagerQuit(t *testing.T) {
	in := "interface 0/9\n or (q)uit\r                   \nlldp transmit-tlv port-desc\n--More-- or (q)uit\nvlan 10\n"
	got := stripPager(in)
	if strings.Contains(strings.ToLower(got), "more") || strings.Contains(strings.ToLower(got), "quit") {
		t.Fatalf("pager left: %q", got)
	}
	if !strings.Contains(got, "interface 0/9") || !strings.Contains(got, "lldp transmit-tlv") {
		t.Fatalf("lost cfg: %q", got)
	}
}

func TestIsSSHFlaky(t *testing.T) {
	if !isSSHFlaky(fmt.Errorf("read tcp 1.2.3.4:1->5.6.7.8:22: read: connection reset by peer")) {
		t.Fatal("rst")
	}
	if !isSSHFlaky(fmt.Errorf("EOF")) {
		t.Fatal("eof")
	}
	if isSSHFlaky(fmt.Errorf("вывод CLI не похож на конфиг")) {
		t.Fatal("not flaky")
	}
}
