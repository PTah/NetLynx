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
