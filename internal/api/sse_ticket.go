package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

type sseTicket struct {
	username string
	exp      time.Time
}

type sseTicketStore struct {
	mu      sync.Mutex
	tickets map[string]sseTicket
}

func newSSETicketStore() *sseTicketStore {
	return &sseTicketStore{tickets: make(map[string]sseTicket)}
}

func (s *sseTicketStore) issue(username string, ttl time.Duration) (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, t := range s.tickets {
		if now.After(t.exp) {
			delete(s.tickets, k)
		}
	}
	s.tickets[raw] = sseTicket{username: username, exp: now.Add(ttl)}
	return raw, nil
}

func (s *sseTicketStore) consume(raw string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[raw]
	if !ok {
		return "", false
	}
	delete(s.tickets, raw)
	if time.Now().After(t.exp) {
		return "", false
	}
	return t.username, true
}

func (s *Server) handleIssueSSETicket(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthDisabled {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"ticket":     "",
			"expires_in": 0,
		})
		return
	}
	u, _ := r.Context().Value(authUserKey).(string)
	if u == "" {
		writeError(w, http.StatusUnauthorized, "требуется вход")
		return
	}
	ticket, err := s.sseTickets.issue(u, 60*time.Second)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ticket":     ticket,
		"expires_in": 60,
	})
}
