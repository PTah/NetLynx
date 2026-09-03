package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

func (s *Server) handleGetMACInvestigationSettings(w http.ResponseWriter, r *http.Request) {
	row, err := s.st.GetMACInvestigationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, row)
}

type patchMACInvestigationSettingsBody struct {
	TrackWiFiClients   *bool   `json:"track_wifi_clients"`
	WiFiClientIPPrefix *string `json:"wifi_client_ip_prefix"`
}

func (s *Server) handlePatchMACInvestigationSettings(w http.ResponseWriter, r *http.Request) {
	cur, err := s.st.GetMACInvestigationSettings(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body patchMACInvestigationSettingsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	next := cur
	if body.TrackWiFiClients != nil {
		next.TrackWiFiClients = *body.TrackWiFiClients
	}
	if body.WiFiClientIPPrefix != nil {
		p := strings.TrimSpace(*body.WiFiClientIPPrefix)
		if p == "" {
			writeError(w, http.StatusBadRequest, "wifi_client_ip_prefix: не может быть пустым")
			return
		}
		if err := store.ValidateWiFiClientIPPrefix(p); err != nil {
			writeError(w, http.StatusBadRequest, "wifi_client_ip_prefix: "+err.Error())
			return
		}
		next.WiFiClientIPPrefix = p
	}
	if err := s.st.UpsertMACInvestigationSettings(r.Context(), next); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.wifiFilterInvalidator != nil {
		s.wifiFilterInvalidator.InvalidateWiFiFilterCache()
	}
	s.audit(r, "settings.mac_investigation.update", "settings", nil, map[string]interface{}{
		"track_wifi_clients":    next.TrackWiFiClients,
		"wifi_client_ip_prefix": next.WiFiClientIPPrefix,
	})
	s.handleGetMACInvestigationSettings(w, r)
}
