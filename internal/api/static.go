package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// spaDir отдаёт файлы из каталога сборки Vite; если файла нет — index.html (маршруты React).
func spaDir(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, "/")
		if strings.Contains(rel, "..") {
			http.Error(w, "неверный путь", http.StatusBadRequest)
			return
		}
		if rel != "" {
			p := filepath.Join(root, rel)
			fi, err := os.Stat(p)
			if err == nil && !fi.IsDir() {
				http.ServeFile(w, r, p)
				return
			}
		}
		http.ServeFile(w, r, filepath.Join(root, "index.html"))
	})
}
