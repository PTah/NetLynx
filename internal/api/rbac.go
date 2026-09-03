package api

import (
	"context"
	"net/http"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
)

type authRoleKey struct{}

const roleAdmin = store.RoleAdmin
const roleOperator = store.RoleOperator
const roleViewer = store.RoleViewer

// resolveAuthRole возвращает роль активного пользователя из БД.
// Fail-closed: при ошибке БД, отсутствующем или неактивном пользователе — ok=false (не admin).
func (s *Server) resolveAuthRole(r *http.Request) (role string, ok bool) {
	if s.cfg.AuthDisabled {
		return roleAdmin, true
	}
	// Env Basic — аварийный admin, даже если DB-пользователь с тем же логином понижен/существует.
	if s.cfg.AdminPassword != "" {
		if u, p, ok := r.BasicAuth(); ok && secureStringEqual(u, s.cfg.AdminUser) && secureStringEqual(p, s.cfg.AdminPassword) {
			return roleAdmin, true
		}
	}
	if u, ok := r.Context().Value(authUserKey).(string); ok && u != "" {
		row, err := s.st.GetAuthUserByUsername(r.Context(), u)
		if err == nil && row.IsActive {
			return row.Role, true
		}
	}
	return "", false
}

func (s *Server) requireMinRole(minRole string) func(http.Handler) http.Handler {
	order := map[string]int{roleViewer: 1, roleOperator: 2, roleAdmin: 3}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.cfg.AuthDisabled {
				next.ServeHTTP(w, r)
				return
			}
			role, ok := s.resolveAuthRole(r)
			if !ok {
				writeError(w, http.StatusUnauthorized, "требуется вход")
				return
			}
			if order[role] < order[minRole] {
				writeError(w, http.StatusForbidden, "недостаточно прав")
				return
			}
			ctx := context.WithValue(r.Context(), authRoleKey{}, role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (s *Server) audit(r *http.Request, action, entityType string, entityID *int64, details map[string]interface{}) {
	u, _ := r.Context().Value(authUserKey).(string)
	var uid *int64
	if u != "" {
		if row, err := s.st.GetAuthUserByUsername(r.Context(), u); err == nil {
			uid = &row.ID
		}
	}
	_ = s.st.InsertAudit(r.Context(), uid, u, action, entityType, entityID, details)
}
