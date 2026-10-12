package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListAuthUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.AuthUserPublic{}
	}
	writeJSON(w, http.StatusOK, list)
}

type createUserBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var body createUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	u, err := s.st.CreateAuthUser(r.Context(), body.Username, body.Password, body.Role)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "user.create", "user", &u.ID, map[string]interface{}{"username": u.Username, "role": u.Role})
	writeJSON(w, http.StatusCreated, u)
}

type patchUserBody struct {
	Role       *string `json:"role"`
	IsActive   *bool   `json:"is_active"`
	NewPassword *string `json:"password"`
}

func (s *Server) handlePatchUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var body patchUserBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	if err := s.st.UpdateAuthUser(r.Context(), id, body.Role, body.IsActive, body.NewPassword); err != nil {
		if err == store.ErrAuthUserNotFound {
			writeError(w, http.StatusNotFound, "пользователь не найден")
			return
		}
		if err == store.ErrLastAdmin {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "user.update", "user", &id, patchUserAuditDetails(body.Role, body.IsActive, body.NewPassword))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func patchUserAuditDetails(role *string, isActive *bool, newPassword *string) map[string]interface{} {
	d := map[string]interface{}{}
	if role != nil {
		d["role"] = strings.TrimSpace(*role)
	}
	if isActive != nil {
		d["is_active"] = *isActive
	}
	if newPassword != nil && strings.TrimSpace(*newPassword) != "" {
		d["password_changed"] = true
	}
	return d
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "неверный id")
		return
	}
	var username string
	if u, err := s.st.GetAuthUserByID(r.Context(), id); err == nil && u != nil {
		username = u.Username
	}
	if err := s.st.DeleteAuthUser(r.Context(), id); err != nil {
		if err == store.ErrAuthUserNotFound {
			writeError(w, http.StatusNotFound, "пользователь не найден")
			return
		}
		if err == store.ErrLastAdmin {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(r, "user.delete", "user", &id, map[string]interface{}{"username": username})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	list, err := s.st.ListAudit(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []store.AuditEntry{}
	}
	writeJSON(w, http.StatusOK, list)
}
