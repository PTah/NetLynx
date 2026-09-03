package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseSMB(t *testing.T) {
	h, sh, rel, err := parseSMB(`//nas.local/backups/netlynx`)
	if err != nil {
		t.Fatal(err)
	}
	if h != "nas.local" || sh != "backups" || rel != "netlynx" {
		t.Fatalf("got %s %s %s", h, sh, rel)
	}
	_, _, _, err = parseSMB("not-a-share")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLooksLikeLocalPath(t *testing.T) {
	if !looksLikeLocalPath("/mnt/nas") {
		t.Fatal("/mnt")
	}
	if looksLikeLocalPath("//nas/share") {
		t.Fatal("unc")
	}
	if looksLikeLocalPath("nas:/export/netlynx") {
		t.Fatal("nfs remote should not be treated as local")
	}
}

func TestRotateDir(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "netlynx-old.zip")
	keep := filepath.Join(dir, "netlynx-new.zip")
	if err := os.WriteFile(old, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-5 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if err := RotateDir(dir, 3, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old zip should be removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("new zip should remain")
	}
}
