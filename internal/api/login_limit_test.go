package api

import (
	"net/http"
	"testing"
)

func TestClientIPIgnoresForwardedWithoutTrust(t *testing.T) {
	r := &http.Request{RemoteAddr: "203.0.113.10:54321", Header: make(http.Header)}
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	r.Header.Set("X-Real-IP", "5.6.7.8")
	got := clientIP(r, false)
	if got != "203.0.113.10" {
		t.Fatalf("without trust: want RemoteAddr host, got %q", got)
	}
}

func TestClientIPUsesForwardedWithTrust(t *testing.T) {
	r := &http.Request{RemoteAddr: "203.0.113.10:54321", Header: make(http.Header)}
	r.Header.Set("X-Real-IP", "5.6.7.8")
	got := clientIP(r, true)
	if got != "5.6.7.8" {
		t.Fatalf("with trust: want X-Real-IP, got %q", got)
	}
	r2 := &http.Request{RemoteAddr: "203.0.113.10:54321", Header: make(http.Header)}
	r2.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")
	got2 := clientIP(r2, true)
	if got2 != "1.2.3.4" {
		t.Fatalf("with trust XFF: want first hop, got %q", got2)
	}
}
