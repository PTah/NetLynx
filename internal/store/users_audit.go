package store

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

type AuthUser struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	IsActive     bool
}

type AuthUserPublic struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	IsActive bool   `json:"is_active"`
}

type AuditEntry struct {
	ID         int64                  `json:"id"`
	UserID     *int64                 `json:"user_id,omitempty"`
	Username   *string                `json:"username,omitempty"`
	Action     string                 `json:"action"`
	EntityType *string                `json:"entity_type,omitempty"`
	EntityID   *int64                 `json:"entity_id,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

func validRole(role string) bool {
	switch strings.TrimSpace(role) {
	case RoleAdmin, RoleOperator, RoleViewer:
		return true
	default:
		return false
	}
}

// EnsureAdminUser создаёт bootstrap-админа только если пользователя ещё нет.
// Пароль существующего admin не перезаписывается (смена — через UI/API).
func (s *Store) EnsureAdminUser(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO users (username, password_hash, role, is_active, updated_at)
		VALUES ($1, $2, 'admin', true, now())
		ON CONFLICT (username) DO NOTHING`,
		username, string(hash),
	)
	return err
}

func (s *Store) GetAuthUserByUsername(ctx context.Context, username string) (*AuthUser, error) {
	var u AuthUser
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, role, is_active
		FROM users WHERE username = $1`, strings.TrimSpace(username),
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.IsActive)
	if err == pgx.ErrNoRows {
		return nil, ErrAuthUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) GetAuthUserByID(ctx context.Context, id int64) (*AuthUser, error) {
	var u AuthUser
	err := s.pool.QueryRow(ctx, `
		SELECT id, username, password_hash, role, is_active
		FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.IsActive)
	if err == pgx.ErrNoRows {
		return nil, ErrAuthUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) ListAuthUsers(ctx context.Context) ([]AuthUserPublic, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, username, role, is_active FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuthUserPublic
	for rows.Next() {
		var u AuthUserPublic
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.IsActive); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) CreateAuthUser(ctx context.Context, username, password, role string) (*AuthUserPublic, error) {
	username = strings.TrimSpace(username)
	role = strings.TrimSpace(role)
	if username == "" || password == "" {
		return nil, errors.New("username и password обязательны")
	}
	if !validRole(role) {
		return nil, errors.New("недопустимая роль")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	var u AuthUserPublic
	err = s.pool.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, role, is_active, updated_at)
		VALUES ($1, $2, $3, true, now())
		RETURNING id, username, role, is_active`,
		username, string(hash), role,
	).Scan(&u.ID, &u.Username, &u.Role, &u.IsActive)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) countActiveAdmins(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM users WHERE role = $1 AND is_active`, RoleAdmin).Scan(&n)
	return n, err
}

func (s *Store) wouldRemoveLastAdmin(ctx context.Context, cur *AuthUser, newRole string, newActive bool) (bool, error) {
	if cur == nil || cur.Role != RoleAdmin || !cur.IsActive {
		return false, nil
	}
	if newRole == RoleAdmin && newActive {
		return false, nil
	}
	n, err := s.countActiveAdmins(ctx)
	if err != nil {
		return false, err
	}
	return n <= 1, nil
}

func (s *Store) UpdateAuthUser(ctx context.Context, id int64, role *string, isActive *bool, newPassword *string) error {
	cur, err := s.GetAuthUserByID(ctx, id)
	if err != nil {
		return err
	}
	newRole := cur.Role
	if role != nil {
		r := strings.TrimSpace(*role)
		if !validRole(r) {
			return errors.New("недопустимая роль")
		}
		newRole = r
	}
	newActive := cur.IsActive
	if isActive != nil {
		newActive = *isActive
	}
	if ok, err := s.wouldRemoveLastAdmin(ctx, cur, newRole, newActive); err != nil {
		return err
	} else if ok {
		return ErrLastAdmin
	}

	sets := []string{"updated_at = now()"}
	args := []interface{}{id}
	n := 2
	if role != nil {
		sets = append(sets, "role = $"+itoa(n))
		args = append(args, newRole)
		n++
	}
	if isActive != nil {
		sets = append(sets, "is_active = $"+itoa(n))
		args = append(args, *isActive)
		n++
	}
	if newPassword != nil && strings.TrimSpace(*newPassword) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		sets = append(sets, "password_hash = $"+itoa(n))
		args = append(args, string(hash))
		n++
	}
	q := "UPDATE users SET " + strings.Join(sets, ", ") + " WHERE id = $1"
	tag, err := s.pool.Exec(ctx, q, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAuthUserNotFound
	}
	return nil
}

func (s *Store) DeleteAuthUser(ctx context.Context, id int64) error {
	cur, err := s.GetAuthUserByID(ctx, id)
	if err != nil {
		return err
	}
	if ok, err := s.wouldRemoveLastAdmin(ctx, cur, "", false); err != nil {
		return err
	} else if ok {
		return ErrLastAdmin
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAuthUserNotFound
	}
	return nil
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func (s *Store) InsertAudit(ctx context.Context, userID *int64, username, action, entityType string, entityID *int64, details map[string]interface{}) error {
	var raw []byte
	if details != nil {
		b, err := json.Marshal(details)
		if err != nil {
			return err
		}
		raw = b
	}
	var un *string
	if strings.TrimSpace(username) != "" {
		u := strings.TrimSpace(username)
		un = &u
	}
	var et *string
	if strings.TrimSpace(entityType) != "" {
		t := strings.TrimSpace(entityType)
		et = &t
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audit_log (user_id, username, action, entity_type, entity_id, details)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, un, action, et, entityID, raw)
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, username, action, entity_type, entity_id, details, created_at
		FROM audit_log ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var raw []byte
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.Action, &e.EntityType, &e.EntityID, &raw, &e.CreatedAt); err != nil {
			return nil, err
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &e.Details)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) UpdateDeviceFDBStatus(ctx context.Context, deviceID int64, status string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE devices SET fdb_monitoring_status = $2, updated_at = now() WHERE id = $1`,
		deviceID, status)
	return err
}
