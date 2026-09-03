package backup

import (
	"context"
	"testing"
	"time"
)

func TestDumpDatabaseBadURL(t *testing.T) {
	_, err := DumpDatabase(context.Background(), "://", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPgQuoteIdent(t *testing.T) {
	if got := pgQuoteIdent(`devices`); got != `"devices"` {
		t.Fatalf("got %s", got)
	}
	if got := pgQuoteIdent(`weird"name`); got != `"weird""name"` {
		t.Fatalf("got %s", got)
	}
}

func TestPgQualify(t *testing.T) {
	if got := pgQualify("public", "events"); got != `"public"."events"` {
		t.Fatalf("got %s", got)
	}
}

func TestDirAccessibleEmpty(t *testing.T) {
	if dirAccessible("") || dirAccessible("/no/such/invetor/dir") {
		t.Fatal("expected inaccessible")
	}
}

func TestTimestampNameLocal(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Vladivostok")
	if err != nil {
		t.Skip(err)
	}
	ts := time.Date(2026, 8, 20, 7, 5, 23, 0, loc)
	if got := TimestampName(ts); got != "20260820-0705" {
		t.Fatalf("got %q want %q", got, "20260820-0705")
	}
}
