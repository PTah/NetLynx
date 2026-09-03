package notify

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRetryAfterHeader(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "13")
	if d := parseRetryAfter(h, nil); d != 13*time.Second {
		t.Fatalf("header: %s", d)
	}
}

func TestParseRetryAfterJSON(t *testing.T) {
	body := []byte(`{"ok":false,"error_code":429,"parameters":{"retry_after":7}}`)
	if d := parseRetryAfter(http.Header{}, body); d != 7*time.Second {
		t.Fatalf("json: %s", d)
	}
}

func TestCapRetryAfter(t *testing.T) {
	if capRetryAfter(0) != time.Second {
		t.Fatal("zero")
	}
	if capRetryAfter(time.Hour) != maxTelegramRetryAfter {
		t.Fatal("cap")
	}
}

func TestRetryAfterOf(t *testing.T) {
	d, ok := retryAfterOf(&retryAfterError{after: 2 * time.Second, msg: "429"})
	if !ok || d != 2*time.Second {
		t.Fatalf("%v %v", d, ok)
	}
}
