package api

import (
	"net/http"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/sysmon"
)

func (s *Server) handleSystemStats(w http.ResponseWriter, _ *http.Request) {
	host := sysmon.ReadSnapshot()
	db := s.st.DBPoolStats()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"host": host,
		"db":   db,
	})
}
