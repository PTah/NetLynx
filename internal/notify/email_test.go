package notify

import (
	"strings"
	"testing"
)

func TestHeaderLineValueStripsCRLF(t *testing.T) {
	t.Parallel()
	got := headerLineValue("alert\r\nBcc: evil@example.com")
	if got != "alertBcc: evil@example.com" {
		t.Fatalf("got %q", got)
	}
	if headerLineValue("  ok@host  ") != "ok@host" {
		t.Fatal("trim")
	}
}

func TestPickSMTPAuthPrefersLOGIN(t *testing.T) {
	t.Parallel()
	// Без реального клиента: проверяем только loginAuth challenge.
	a := loginAuth{username: "noreply", password: "secret"}
	mech, initial, err := a.Start(nil)
	if err != nil || mech != "LOGIN" || initial != nil {
		t.Fatalf("Start: %q %v %v", mech, initial, err)
	}
	u, err := a.Next([]byte("Username:"), true)
	if err != nil || string(u) != "noreply" {
		t.Fatalf("user: %q %v", u, err)
	}
	p, err := a.Next([]byte("Password:"), true)
	if err != nil || string(p) != "secret" {
		t.Fatalf("pass: %q %v", p, err)
	}
}

func TestBuildMultipartHasAttachment(t *testing.T) {
	t.Parallel()
	raw, err := buildMultipart("from@ex", []string{"to@ex"}, "subj", "hello", []Attachment{{Name: "netlynx.zip", Data: []byte("PK")}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "multipart/mixed") {
		t.Fatal("missing mixed")
	}
	if !strings.Contains(s, `filename="netlynx.zip"`) {
		t.Fatal("missing filename")
	}
	if !strings.Contains(s, "hello") {
		t.Fatal("missing body")
	}
}
