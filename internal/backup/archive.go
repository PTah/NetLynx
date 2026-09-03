package backup

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

type Manifest struct {
	CreatedAt    string            `json:"created_at"`
	Version      string            `json:"version,omitempty"`
	Status       string            `json:"status"`
	Notes        []string          `json:"notes,omitempty"`
	SwitchErrors map[string]string `json:"switch_errors,omitempty"`
	SwitchOK     []string          `json:"switch_ok,omitempty"`
}

func BuildZip(created time.Time, version string, sql []byte, envName string, env []byte, configs map[string][]byte, man Manifest) ([]byte, error) {
	tmp, err := os.CreateTemp("", "netlynx-zip-*.zip")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(path)
	if err := BuildZipFile(path, created, version, "", sql, envName, env, configs, man); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// BuildZipFile собирает архив на диск. sqlPath или sqlData (один из двух): дамп БД.
func BuildZipFile(dstPath string, created time.Time, version string, sqlPath string, sqlData []byte, envName string, env []byte, configs map[string][]byte, man Manifest) error {
	man.CreatedAt = created.UTC().Format(time.RFC3339)
	man.Version = version
	if man.Status == "" {
		man.Status = "ok"
	}
	raw, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(f)
	if err := writeZipBytes(zw, "manifest.json", raw); err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	if err := writeZipSQLGz(zw, sqlPath, sqlData); err != nil {
		_ = zw.Close()
		_ = f.Close()
		return err
	}
	if len(env) > 0 {
		name := envName
		if name == "" {
			name = "netlynx.env"
		}
		if err := writeZipBytes(zw, name, env); err != nil {
			_ = zw.Close()
			_ = f.Close()
			return err
		}
	}
	for name, body := range configs {
		if err := writeZipBytes(zw, "configs/"+name, body); err != nil {
			_ = zw.Close()
			_ = f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func writeZipSQLGz(zw *zip.Writer, sqlPath string, sqlData []byte) error {
	w, err := zw.Create("db.sql.gz")
	if err != nil {
		return err
	}
	gw := gzip.NewWriter(w)
	if sqlPath != "" {
		in, err := os.Open(sqlPath)
		if err != nil {
			_ = gw.Close()
			return err
		}
		_, copyErr := io.Copy(gw, in)
		_ = in.Close()
		if copyErr != nil {
			_ = gw.Close()
			return copyErr
		}
	} else {
		if _, err := gw.Write(sqlData); err != nil {
			_ = gw.Close()
			return err
		}
	}
	return gw.Close()
}

func gzipBytes(in []byte) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(in); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeZipBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, bytes.NewReader(data))
	return err
}

func ZipFileName(t time.Time) string {
	return fmt.Sprintf("netlynx-%s.zip", TimestampName(t))
}
