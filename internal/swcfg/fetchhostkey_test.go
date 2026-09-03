package swcfg

import "testing"

func TestSSHHostKeyProfilesOrder(t *testing.T) {
	ps := sshHostKeyProfiles()
	if len(ps) < 3 {
		t.Fatalf("want at least 3 profiles, got %d", len(ps))
	}
	if ps[0].name != "modern" {
		t.Fatalf("first profile should be modern, got %q", ps[0].name)
	}
	if ps[len(ps)-1].name != "legacy" {
		t.Fatalf("last profile should be legacy, got %q", ps[len(ps)-1].name)
	}
	foundGEXSHA1 := false
	for _, p := range ps {
		for _, k := range p.keyExchanges {
			if k == "diffie-hellman-group-exchange-sha1" || k == "diffie-hellman-group1-sha1" {
				foundGEXSHA1 = true
			}
		}
	}
	if !foundGEXSHA1 {
		t.Fatal("legacy DH-SHA1 kex must be in some profile")
	}
}
