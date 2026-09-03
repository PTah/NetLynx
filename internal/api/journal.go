package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/journallog"
)

func (s *Server) handleJournalMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, journallog.Available())
}

func (s *Server) handleJournalLines(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	cats := splitCSV(q.Get("categories"))
	lines, err := journallog.ReadLines(r.Context(), journallog.Query{
		Limit:      limit,
		Since:      q.Get("since"),
		Until:      q.Get("until"),
		LevelID:    q.Get("level"),
		Categories: cats,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lines": lines,
		"count": len(lines),
	})
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) requireAdminSSE(r *http.Request) (username string, ok bool) {
	u, ok := s.authorizeSSE(r)
	if !ok {
		return "", false
	}
	if s.cfg.AuthDisabled {
		return u, true
	}
	row, err := s.st.GetAuthUserByUsername(r.Context(), u)
	if err != nil || !row.IsActive {
		return "", false
	}
	if strings.ToLower(strings.TrimSpace(row.Role)) != roleAdmin {
		return "", false
	}
	return row.Username, true
}

func (s *Server) handleJournalStream(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminSSE(r); !ok {
		writeError(w, http.StatusUnauthorized, "нужен admin (sse-ticket или Bearer)")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming не поддерживается")
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	cats := splitCSV(q.Get("categories"))
	level := q.Get("level")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	ch := make(chan string, 64)
	errCh := make(chan error, 1)
	go func() {
		errCh <- journallog.Follow(r.Context(), journallog.Query{
			Limit:      limit,
			LevelID:    level,
			Categories: cats,
			Follow:     true,
		}, ch)
	}()

	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case err := <-errCh:
			if err != nil && r.Context().Err() == nil {
				payload, _ := json.Marshal(err.Error())
				_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", payload)
				flusher.Flush()
			}
			return
		case line := <-ch:
			payload, _ := json.Marshal(line)
			_, _ = fmt.Fprintf(w, "event: log\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-tick.C:
			_, _ = fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
