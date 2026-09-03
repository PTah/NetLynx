package backup

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"testing"
	"time"
)

func TestBuildZipFileStreamsSQL(t *testing.T) {
	dir := t.TempDir()
	sqlPath := dir + "/dump.sql"
	if err := os.WriteFile(sqlPath, []byte("SELECT 1;\nCREATE TABLE t (id int);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	zipPath := dir + "/out.zip"
	if err := BuildZipFile(zipPath, time.Now(), "test", sqlPath, nil, "", nil, nil, Manifest{Status: "ok"}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var gotGz bool
	for _, f := range zr.File {
		if f.Name != "db.sql.gz" {
			continue
		}
		gotGz = true
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		gr, err := gzip.NewReader(rc)
		if err != nil {
			_ = rc.Close()
			t.Fatal(err)
		}
		body, err := io.ReadAll(gr)
		_ = gr.Close()
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(body, []byte("CREATE TABLE")) {
			t.Fatalf("unexpected sql body: %q", body)
		}
	}
	if !gotGz {
		t.Fatal("db.sql.gz missing")
	}
}

func TestDumpDatabaseFileBadURL(t *testing.T) {
	_, cleanup, err := DumpDatabaseFile(context.Background(), "://", nil)
	if err == nil {
		if cleanup != nil {
			cleanup()
		}
		t.Fatal("expected error")
	}
}
