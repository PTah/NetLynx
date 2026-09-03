package store

import (
	"strings"
	"testing"
)

func TestCanonicalizeConfigText_stripsUptimeAndSNTP(t *testing.T) {
	a := `!Current Configuration:
!
!System Description "EdgeSwitch 24 250W, 1.9.2-lite, Linux 3.6.5-03329b4a, 0.0.0.0000000"
!System Software Version "1.9.2-lite"
!System Up Time          "1030 days 4 hrs 11 mins 45 secs"
!Additional Packages     QOS,IPv6 Management,Routing
!Current SNTP Synchronized Time: Sep  2 23:22:32 2026 UTC
!
vlan database
vlan 10`
	b := `!Current Configuration:
!
!System Description "EdgeSwitch 24 250W, 1.9.2-lite, Linux 3.6.5-03329b4a, 0.0.0.0000000"
!System Software Version "1.9.2-lite"
!System Up Time          "1030 days 4 hrs 44 mins 18 secs"
!Additional Packages     QOS,IPv6 Management,Routing
!Current SNTP Synchronized Time: Sep  2 23:55:05 2026 UTC
!
vlan database
vlan 10`
	got := CanonicalizeConfigText(a)
	if got != CanonicalizeConfigText(b) {
		t.Fatalf("uptime/SNTP must not change canonical config\nA:\n%s\nB:\n%s", got, CanonicalizeConfigText(b))
	}
	if configHash(a) != configHash(b) {
		t.Fatal("hash must match when only uptime/SNTP changed")
	}
	if !strings.Contains(got, "vlan 10") {
		t.Fatal("functional lines must remain")
	}
	if strings.Contains(got, "System Up Time") || strings.Contains(got, "SNTP") {
		t.Fatal("volatile lines must be stripped")
	}
}

func TestCanonicalizeConfigText_realChangeStillDiffers(t *testing.T) {
	a := "!System Up Time \"1\"\nvlan 10\n"
	b := "!System Up Time \"2\"\nvlan 20\n"
	if configHash(a) == configHash(b) {
		t.Fatal("vlan change must change hash")
	}
}

func TestCanonicalizeConfigText_ntpClockPeriodAndRouterOSHeader(t *testing.T) {
	a := "hostname x\nntp clock-period 17179870\n"
	b := "hostname x\nntp clock-period 17179899\n"
	if configHash(a) != configHash(b) {
		t.Fatal("ntp clock-period must be ignored")
	}
	rosA := "# sep/03/2026 10:11:36 by RouterOS 7.15\n/ip address add\n"
	rosB := "# sep/03/2026 10:44:01 by RouterOS 7.15\n/ip address add\n"
	if configHash(rosA) != configHash(rosB) {
		t.Fatal("RouterOS export header time must be ignored")
	}
}
