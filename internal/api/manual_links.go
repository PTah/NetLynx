package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListManualLinks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := strings.TrimSpace(q.Get("status"))
	limit := 200
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			writeError(w, http.StatusBadRequest, "limit: 1–500")
			return
		}
		limit = n
	}
	var deviceID *int64
	if v := strings.TrimSpace(q.Get("device_id")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "неверный device_id")
			return
		}
		deviceID = &id
	}
	list, err := s.st.ListManualLinks(r.Context(), deviceID, status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.ManualTopologyLink{}
	}
	writeJSON(w, http.StatusOK, list)
}

type manualLinkBody struct {
	ADeviceID int64   `json:"a_device_id"`
	AIfIndex  int     `json:"a_if_index"`
	BDeviceID int64   `json:"b_device_id"`
	BIfIndex  int     `json:"b_if_index"`
	Note      *string `json:"note"`
}

func (s *Server) handleCreateManualLink(w http.ResponseWriter, r *http.Request) {
	var body manualLinkBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	var createdBy *string
	if u, ok := r.Context().Value(authUserKey).(string); ok && u != "" {
		createdBy = &u
	}
	link, err := s.st.CreateManualLink(r.Context(), body.ADeviceID, body.AIfIndex, body.BDeviceID, body.BIfIndex, body.Note, createdBy)
	if err != nil {
		if errors.Is(err, store.ErrManualLinkInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, store.ErrManualLinkConflict) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "manual_link.create", "manual_topology_link", &link.ID, map[string]interface{}{
		"a_device_id": link.ADeviceID, "a_if_index": link.AIfIndex,
		"b_device_id": link.BDeviceID, "b_if_index": link.BIfIndex,
	})
	writeJSON(w, http.StatusCreated, link)
}

type manualLinkPatchBody struct {
	ADeviceID *int64  `json:"a_device_id"`
	AIfIndex  *int    `json:"a_if_index"`
	BDeviceID *int64  `json:"b_device_id"`
	BIfIndex  *int    `json:"b_if_index"`
	Note      *string `json:"note"`
	Status    *string `json:"status"`
}

func (s *Server) handlePatchManualLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body manualLinkPatchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	link, err := s.st.UpdateManualLink(r.Context(), id, store.ManualLinkPatch{
		ADeviceID: body.ADeviceID,
		AIfIndex:  body.AIfIndex,
		BDeviceID: body.BDeviceID,
		BIfIndex:  body.BIfIndex,
		Note:      body.Note,
		Status:    body.Status,
	})
	if err != nil {
		if errors.Is(err, store.ErrManualLinkNotFound) {
			writeError(w, http.StatusNotFound, "связь не найдена")
			return
		}
		if errors.Is(err, store.ErrManualLinkSuperseded) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, store.ErrManualLinkInvalid) || errors.Is(err, store.ErrManualLinkConflict) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "manual_link.update", "manual_topology_link", &link.ID, nil)
	writeJSON(w, http.StatusOK, link)
}

func (s *Server) handleDeleteManualLink(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	if err := s.st.DeleteManualLink(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrManualLinkNotFound) {
			writeError(w, http.StatusNotFound, "связь не найдена")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "manual_link.delete", "manual_topology_link", &id, nil)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "id": id})
}
