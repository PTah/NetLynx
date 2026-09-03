package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const appSecretJWTKey = "jwt_hs256"

var (
	ErrAuthTokenReuse     = errors.New("refresh token reuse detected")
	ErrAuthSessionExpired = errors.New("auth session expired")
)

// EnsureJWTSecret возвращает HS256-секрет: из БД, иначе seed (env / legacy от пароля), иначе новый random.
func (s *Store) EnsureJWTSecret(ctx context.Context, envSecret, adminPassword string) (string, error) {
	var existing string
	err := s.pool.QueryRow(ctx, `SELECT value FROM app_secrets WHERE key = $1`, appSecretJWTKey).Scan(&existing)
	if err == nil && strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing), nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	secret := strings.TrimSpace(envSecret)
	if secret == "" && strings.TrimSpace(adminPassword) != "" {
		sum := sha256.Sum256([]byte("invetor-jwt-v1:" + adminPassword))
		secret = hex.EncodeToString(sum[:])
	}
	if secret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("jwt secret rand: %w", err)
		}
		secret = hex.EncodeToString(b)
	}
	if len(secret) < 32 {
		return "", fmt.Errorf("JWT secret короче 32 символов")
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO app_secrets (key, value, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (key) DO NOTHING`, appSecretJWTKey, secret)
	if err != nil {
		return "", err
	}
	if err := s.pool.QueryRow(ctx, `SELECT value FROM app_secrets WHERE key = $1`, appSecretJWTKey).Scan(&existing); err != nil {
		return "", err
	}
	return strings.TrimSpace(existing), nil
}

type AuthSession struct {
	ID          int64
	UserID      int64
	FamilyID    string
	RefreshHash string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
}

func (s *Store) CreateAuthSession(ctx context.Context, userID int64, refreshHash string, expiresAt time.Time) (familyID string, err error) {
	err = s.pool.QueryRow(ctx, `
		INSERT INTO auth_sessions (user_id, refresh_hash, expires_at, family_id, updated_at)
		VALUES ($1, $2, $3, gen_random_uuid(), now())
		RETURNING family_id::text`, userID, refreshHash, expiresAt,
	).Scan(&familyID)
	return familyID, err
}

func (s *Store) GetAuthSessionByHash(ctx context.Context, refreshHash string) (*AuthSession, error) {
	var sess AuthSession
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, family_id::text, refresh_hash, expires_at, revoked_at
		FROM auth_sessions WHERE refresh_hash = $1`, refreshHash,
	).Scan(&sess.ID, &sess.UserID, &sess.FamilyID, &sess.RefreshHash, &sess.ExpiresAt, &sess.RevokedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrAuthSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) RevokeAuthSessionByHash(ctx context.Context, refreshHash string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = now(), updated_at = now()
		WHERE refresh_hash = $1 AND revoked_at IS NULL`, refreshHash)
	return err
}

func (s *Store) RevokeAuthSessionFamily(ctx context.Context, familyID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = now(), updated_at = now()
		WHERE family_id = $1::uuid AND revoked_at IS NULL`, familyID)
	return err
}

// RotateAuthSession атомарно ротирует refresh; reuse отозванного → revoke всей семьи.
func (s *Store) RotateAuthSession(ctx context.Context, oldHash, newHash string, newExpiresAt time.Time) (userID int64, familyID string, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer tx.Rollback(ctx)

	var (
		id        int64
		uid       int64
		fid       string
		exp       time.Time
		revokedAt *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, family_id::text, expires_at, revoked_at
		FROM auth_sessions WHERE refresh_hash = $1
		FOR UPDATE`, oldHash,
	).Scan(&id, &uid, &fid, &exp, &revokedAt)
	if err == pgx.ErrNoRows {
		return 0, "", ErrAuthSessionNotFound
	}
	if err != nil {
		return 0, "", err
	}

	now := time.Now().UTC()
	if revokedAt != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE auth_sessions SET revoked_at = now(), updated_at = now()
			WHERE family_id = $1::uuid AND revoked_at IS NULL`, fid); err != nil {
			return 0, "", err
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, "", err
		}
		return 0, fid, ErrAuthTokenReuse
	}
	if now.After(exp) {
		return 0, "", ErrAuthSessionExpired
	}

	if _, err := tx.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = now(), updated_at = now()
		WHERE id = $1 AND revoked_at IS NULL`, id); err != nil {
		return 0, "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_sessions (user_id, refresh_hash, expires_at, family_id, rotated_from, updated_at)
		VALUES ($1, $2, $3, $4::uuid, $5, now())`, uid, newHash, newExpiresAt, fid, id); err != nil {
		return 0, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", err
	}
	return uid, fid, nil
}
