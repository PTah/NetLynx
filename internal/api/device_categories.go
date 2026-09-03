package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListDeviceCategories(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListDeviceCategoryDefs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"categories": list})
}

type createDeviceCategoryBody struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Color string `json:"color"`
	Blink bool   `json:"blink"`
}

func (s *Server) handleCreateDeviceCategory(w http.ResponseWriter, r *http.Request) {
	var body createDeviceCategoryBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	d, err := s.st.CreateDeviceCategoryDef(r.Context(), body.ID, body.Label, body.Color, body.Blink)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "device_category.create", "device_category", nil, map[string]interface{}{
		"id": d.ID, "label": d.Label, "color": d.Color, "blink": d.Blink,
	})
	writeJSON(w, http.StatusCreated, d)
}

type patchDeviceCategoryDefBody struct {
	Label *string `json:"label"`
	Color *string `json:"color"`
	Blink *bool   `json:"blink"`
}

func (s *Server) handlePatchDeviceCategoryDef(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" || !store.ValidCategoryID(id) {
		writeError(w, http.StatusBadRequest, "неверный id типа")
		return
	}
	var body patchDeviceCategoryDefBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON: "+err.Error())
		return
	}
	if body.Label == nil && body.Color == nil && body.Blink == nil {
		writeError(w, http.StatusBadRequest, "укажите label, color и/или blink")
		return
	}
	d, err := s.st.UpdateDeviceCategoryDef(r.Context(), id, body.Label, body.Color, body.Blink)
	if err != nil {
		if errors.Is(err, store.ErrCategoryNotFound) {
			writeError(w, http.StatusNotFound, "тип не найден")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "device_category.update", "device_category", nil, map[string]interface{}{
		"id": d.ID, "label": d.Label, "color": d.Color, "blink": d.Blink,
	})
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDeviceCategory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" || !store.ValidCategoryID(id) {
		writeError(w, http.StatusBadRequest, "неверный id типа")
		return
	}
	if err := s.st.DeleteDeviceCategoryDef(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrCategoryNotFound) {
			writeError(w, http.StatusNotFound, "тип не найден")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "device_category.delete", "device_category", nil, map[string]interface{}{"id": id})
	w.WriteHeader(http.StatusNoContent)
}
