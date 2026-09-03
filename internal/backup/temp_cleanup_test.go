package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupStaleTemp(t *testing.T) {
	dir := t.TempDir()

	dump := filepath.Join(dir, "netlynx-dump-123.sql")
	if err := os.WriteFile(dump, []byte("sql"), 0o600); err != nil {
		t.Fatal(err)
	}
	zip := filepath.Join(dir, "netlynx-backup-456.zip")
	if err := os.WriteFile(zip, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dir, "other-file.txt")
	if err := os.WriteFile(keep, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	cleanupStaleTempDir(dir, nil)

	if _, err := os.Stat(dump); !os.IsNotExist(err) {
		t.Fatal("dump should be removed")
	}
	if _, err := os.Stat(zip); !os.IsNotExist(err) {
		t.Fatal("zip should be removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatal("unrelated file should remain")
	}
}
