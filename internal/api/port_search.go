package api

import (
	"net/http"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func (s *Server) handleSearchPorts(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) < 2 {
		writeError(w, http.StatusBadRequest, "q: минимум 2 символа (описание, MAC или IP)")
		return
	}
	kind, _ := store.ClassifySearchQuery(q)
	if kind == store.SearchQueryIP && len(q) < 7 {
		writeError(w, http.StatusBadRequest, "укажите полный IPv4, например 192.168.1.50")
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	hits, err := s.st.SearchPorts(r.Context(), q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if hits == nil {
		hits = []store.PortSearchHit{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"query": q,
		"hits":  hits,
	})
}
