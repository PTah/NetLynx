package backup

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

func RotateDir(dir string, retainDays int, now time.Time) error {
	if retainDays < 1 {
		retainDays = 3
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	cutoff := now.Add(-time.Duration(retainDays) * 24 * time.Hour)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "netlynx-") || !strings.HasSuffix(name, ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
	return nil
}
