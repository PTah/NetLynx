package api

import (
	"net/http"
	"strings"
)

const (
	maxJSONBodyBytes   = 1 << 20     // 1 MiB
	maxBackupBodyBytes = 512 << 20   // как ParseMultipartForm у backup
)

func requestBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int64(maxJSONBodyBytes)
		if strings.HasPrefix(r.URL.Path, "/api/v1/backup/") {
			n = maxBackupBodyBytes
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, n)
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-XSS-Protection", "0")
		next.ServeHTTP(w, r)
	})
}
