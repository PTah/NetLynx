package backup

import (
	"log/slog"
	"os"
	"path/filepath"
)

var staleTempPatterns = []string{"netlynx-dump-*", "netlynx-backup-*", "netlynx-zip-*"}

// CleanupStaleTemp удаляет осиротевшие temp-файлы бэкапа (после OOM/kill, когда defer не успел).
func CleanupStaleTemp(log *slog.Logger) {
	cleanupStaleTempDir(os.TempDir(), log)
}

func cleanupStaleTempDir(dir string, log *slog.Logger) {
	var removed int
	var freed int64
	for _, pat := range staleTempPatterns {
		matches, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			continue
		}
		for _, p := range matches {
			if st, err := os.Stat(p); err == nil {
				freed += st.Size()
			}
			if err := os.Remove(p); err != nil {
				if log != nil {
					log.Warn("backup: не удалось удалить temp", "path", p, "err", err)
				}
				continue
			}
			removed++
		}
	}
	if removed > 0 && log != nil {
		log.Info("backup: удалены устаревшие temp-файлы", "count", removed, "bytes", freed)
	}
}
