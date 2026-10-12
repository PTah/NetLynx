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

func (s *Server) handleGetSecretsStatus(w http.ResponseWriter, r *http.Request) {
	stats, err := s.st.CountPlaintextSecrets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"encryption_enabled": len(s.cfg.SecretsKey) > 0,
		"require_key":        s.cfg.SecretsRequireKey,
		"plaintext":          stats,
	})
}

func (s *Server) handleRewrapSecrets(w http.ResponseWriter, r *http.Request) {
	if len(s.cfg.SecretsKey) == 0 {
		writeError(w, http.StatusBadRequest, "NETLYNX_SECRETS_KEY не задан — задайте ключ и перезапустите службу")
		return
	}
	res, err := s.st.RewrapAllSecrets(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "secrets.rewrap", "system", nil, map[string]interface{}{
		"rewrapped": res.Rewrapped,
		"skipped":   res.Skipped,
		"after":     res.After.Total,
	})
	writeJSON(w, http.StatusOK, res)
}
