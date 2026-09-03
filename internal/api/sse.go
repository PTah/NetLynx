package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (s *Server) authorizeSSE(r *http.Request) (username string, ok bool) {
	if s.cfg.AuthDisabled {
		return "anonymous", true
	}
	if ticket := strings.TrimSpace(r.URL.Query().Get("ticket")); ticket != "" {
		u, ok := s.sseTickets.consume(ticket)
		if !ok {
			return "", false
		}
		row, err := s.st.GetAuthUserByUsername(r.Context(), u)
		if err != nil || !row.IsActive {
			return "", false
		}
		return row.Username, true
	}
	if u, ok := s.userFromBearer(r); ok {
		row, err := s.st.GetAuthUserByUsername(r.Context(), u)
		if err != nil || !row.IsActive {
			return "", false
		}
		return row.Username, true
	}
	return "", false
}

func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authorizeSSE(r); !ok {
		writeError(w, http.StatusUnauthorized, "требуется вход (sse-ticket или Bearer)")
		return
	}
	if s.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "live stream не настроен")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming не поддерживается")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-ch:
			_, _ = fmt.Fprintf(w, "event: invetor_event\ndata: %s\n\n", b)
			flusher.Flush()
		case <-tick.C:
			_, _ = fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}
