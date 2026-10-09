package snmp

import "testing"

func TestCPUProfileMatchCisco(t *testing.T) {
	desc := "Cisco IOS Software, C2960"
	prof := cpuProfile{Name: "generic", OIDs: []string{oidCPUIdleUCD}}
	for _, p := range cpuProfiles {
		for _, needle := range p.MatchAny {
			if containsFold(desc, needle) {
				prof = p
				break
			}
		}
		if prof.Name == p.Name {
			break
		}
	}
	if prof.Name != "cisco" {
		t.Fatalf("expected cisco profile, got %q", prof.Name)
	}
}

func TestCPUProfileMikroTikUsesLoadNotTemperature(t *testing.T) {
	desc := "RouterOS CCR1009-7G-1C-1S+"
	var prof cpuProfile
	for _, p := range cpuProfiles {
		for _, needle := range p.MatchAny {
			if containsFold(desc, needle) {
				prof = p
				break
			}
		}
		if prof.Name == p.Name {
			break
		}
	}
	if prof.Name != "mikrotik" {
		t.Fatalf("expected mikrotik profile, got %q", prof.Name)
	}
	if len(prof.OIDs) == 0 || prof.OIDs[0] != oidCPUMikrotikLoad {
		t.Fatalf("mikrotik must prefer %s, got %v", oidCPUMikrotikLoad, prof.OIDs)
	}
	for _, oid := range prof.OIDs {
		// mtxrHlTemperature — частая ошибка в старых скриптах.
		if oid == "1.3.6.1.4.1.14988.1.1.3.10.0" {
			t.Fatal("mikrotik profile must not use mtxrHlTemperature as CPU")
		}
		if oid == "1.3.6.1.4.1.2021.11.9.0" {
			t.Fatal("ssCpuUser (.11.9) is not idle; do not use as CPU idle OID")
		}
	}
	if oidCPUIdleUCD != "1.3.6.1.4.1.2021.11.11.0" {
		t.Fatalf("ssCpuIdle must be .11.11.0, got %s", oidCPUIdleUCD)
	}
}

func containsFold(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && stringContainsFold(s, sub)
}

func stringContainsFold(s, sub string) bool {
	return len(s) >= len(sub) && indexFold(s, sub) >= 0
}

func indexFold(s, sub string) int {
	ls, lsub := len(s), len(sub)
	for i := 0; i+lsub <= ls; i++ {
		ok := true
		for j := 0; j < lsub; j++ {
			c1, c2 := s[i+j], sub[j]
			if c1 >= 'A' && c1 <= 'Z' {
				c1 += 'a' - 'A'
			}
			if c2 >= 'A' && c2 <= 'Z' {
				c2 += 'a' - 'A'
			}
			if c1 != c2 {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}
