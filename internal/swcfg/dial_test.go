package swcfg

import (
	"errors"
	"testing"
)

func TestClassifySSHError(t *testing.T) {
	cases := []struct {
		err  string
		want string
	}{
		{"ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password], no supported methods remain", "auth"},
		{"ssh: handshake failed: crypto/rsa: verification error", "hostkey"},
		{"SSH: ключ хоста 1.2.3.4 изменился (возможна подмена)", "hostkey"},
		{"dial tcp: i/o timeout", "timeout"},
		{"something else", "other"},
	}
	for _, tc := range cases {
		got := ClassifySSHError(errors.New(tc.err))
		if got != tc.want {
			t.Fatalf("%q: got %s want %s", tc.err, got, tc.want)
		}
	}
}

func TestIsHostKeyRecoverableNotAuth(t *testing.T) {
	auth := errors.New("ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]")
	if isHostKeyRecoverable(auth) {
		t.Fatal("auth must not be hostkey-recoverable")
	}
	rsa := errors.New("ssh: handshake failed: crypto/rsa: verification error")
	if !isHostKeyRecoverable(rsa) {
		t.Fatal("rsa verification should be recoverable")
	}
}

func TestKnownHostsLineMatchesHost(t *testing.T) {
	if !knownHostsLineMatchesHost("10.0.0.1 ssh-rsa AAAA", "10.0.0.1", 22) {
		t.Fatal("expected match")
	}
	if knownHostsLineMatchesHost("10.0.0.1 ssh-rsa AAAA", "10.0.0.1", 22) {
		t.Fatal("unexpected match")
	}
	if !knownHostsLineMatchesHost("[10.0.0.1]:2222 ssh-rsa AAAA", "10.0.0.1", 2222) {
		t.Fatal("expected ported match")
	}
}
