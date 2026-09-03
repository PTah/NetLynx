package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/store"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const refreshCookieName = "invetor_refresh"

type authClaims struct {
	Username string `json:"usr"`
	jwt.RegisteredClaims
}

type authContextKey string

const authUserKey authContextKey = "auth_user"

func secureStringEqual(a, b string) bool {
	if len(a) != len(b) {
		// сравниваем с собой, чтобы не коротко замыкаться только по длине
		subtle.ConstantTimeCompare([]byte(a), []byte(a))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthDisabled {
			next.ServeHTTP(w, r)
			return
		}
		if u, ok := s.userFromBearer(r); ok {
			row, err := s.st.GetAuthUserByUsername(r.Context(), u)
			if err != nil || !row.IsActive {
				writeError(w, http.StatusUnauthorized, "пользователь недоступен")
				return
			}
			ctx := context.WithValue(r.Context(), authUserKey, row.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		u, p, ok := r.BasicAuth()
		if ok {
			ip := clientIP(r, s.cfg.TrustProxy)
			if !s.loginLimit.allow(ip, u) {
				writeError(w, http.StatusTooManyRequests, "слишком много попыток входа, подождите")
				return
			}
			userOK := secureStringEqual(u, s.cfg.AdminUser)
			passOK := secureStringEqual(p, s.cfg.AdminPassword)
			if userOK && passOK {
				s.loginLimit.success(ip, u)
				ctx := context.WithValue(r.Context(), authUserKey, u)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			s.loginLimit.fail(ip, u)
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="NetLynx", Bearer`)
		writeError(w, http.StatusUnauthorized, "требуется вход")
	})
}

func (s *Server) userFromBearer(r *http.Request) (string, bool) {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		return "", false
	}
	tokenStr := strings.TrimSpace(raw[7:])
	if tokenStr == "" || s.cfg.JWTSecret == "" {
		return "", false
	}
	claims := &authClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid || claims.Username == "" {
		return "", false
	}
	return claims.Username, true
}

func (s *Server) issueAccessToken(username string) (string, int64, error) {
	now := time.Now().UTC()
	exp := now.Add(s.cfg.AccessTokenTTL)
	claims := authClaims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	j := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok, err := j.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", 0, err
	}
	return tok, int64(s.cfg.AccessTokenTTL / time.Second), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func newRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, hashToken(raw), nil
}

func (s *Server) setRefreshCookie(w http.ResponseWriter, raw string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    raw,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl / time.Second),
	})
}

func (s *Server) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

type loginBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthDisabled {
		writeError(w, http.StatusBadRequest, "аутентификация отключена на сервере")
		return
	}
	var b loginBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "тело запроса слишком большое")
			return
		}
		writeError(w, http.StatusBadRequest, "неверный JSON")
		return
	}
	b.Username = strings.TrimSpace(b.Username)
	if b.Username == "" || b.Password == "" {
		writeError(w, http.StatusBadRequest, "username и password обязательны")
		return
	}
	ip := clientIP(r, s.cfg.TrustProxy)
	if !s.loginLimit.allow(ip, b.Username) {
		writeError(w, http.StatusTooManyRequests, "слишком много попыток входа, подождите")
		return
	}
	u, err := s.st.GetAuthUserByUsername(r.Context(), b.Username)
	if err != nil || !u.IsActive {
		s.loginLimit.fail(ip, b.Username)
		writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(b.Password)); err != nil {
		s.loginLimit.fail(ip, b.Username)
		writeError(w, http.StatusUnauthorized, "неверный логин или пароль")
		return
	}
	s.loginLimit.success(ip, b.Username)
	refreshRaw, refreshHash, err := newRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.st.CreateAuthSession(r.Context(), u.ID, refreshHash, time.Now().UTC().Add(s.cfg.RefreshTokenTTL)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	access, ttlSec, err := s.issueAccessToken(u.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setRefreshCookie(w, refreshRaw, s.cfg.RefreshTokenTTL)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": access,
		"token_type":   "Bearer",
		"expires_in":   ttlSec,
		"user": map[string]interface{}{
			"id":       u.ID,
			"username": u.Username,
			"role":     u.Role,
		},
	})
}

func (s *Server) handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookieName)
	if err != nil || strings.TrimSpace(c.Value) == "" {
		writeError(w, http.StatusUnauthorized, "нет refresh cookie")
		return
	}
	oldHash := hashToken(c.Value)
	refreshRaw, refreshHash, err := newRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	userID, _, err := s.st.RotateAuthSession(r.Context(), oldHash, refreshHash, time.Now().UTC().Add(s.cfg.RefreshTokenTTL))
	if err != nil {
		s.clearRefreshCookie(w)
		if errors.Is(err, store.ErrAuthTokenReuse) {
			writeError(w, http.StatusUnauthorized, "refresh reuse: сессия отозвана")
			return
		}
		writeError(w, http.StatusUnauthorized, "refresh сессия недействительна")
		return
	}
	u, err := s.st.GetAuthUserByID(r.Context(), userID)
	if err != nil || !u.IsActive {
		_ = s.st.RevokeAuthSessionByHash(r.Context(), refreshHash)
		writeError(w, http.StatusUnauthorized, "пользователь недоступен")
		return
	}
	access, ttlSec, err := s.issueAccessToken(u.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.setRefreshCookie(w, refreshRaw, s.cfg.RefreshTokenTTL)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": access,
		"token_type":   "Bearer",
		"expires_in":   ttlSec,
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(refreshCookieName); err == nil && strings.TrimSpace(c.Value) != "" {
		hash := hashToken(c.Value)
		if sess, err := s.st.GetAuthSessionByHash(r.Context(), hash); err == nil && sess.FamilyID != "" {
			if err := s.st.RevokeAuthSessionFamily(r.Context(), sess.FamilyID); err != nil {
				slog.Warn("logout revoke family", "err", err)
			}
		} else if err := s.st.RevokeAuthSessionByHash(r.Context(), hash); err != nil {
			slog.Warn("logout revoke session", "err", err)
		}
	}
	s.clearRefreshCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthDisabled {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id": 0, "username": "local", "role": roleAdmin,
		})
		return
	}
	u, _ := r.Context().Value(authUserKey).(string)
	if u == "" {
		if uu, ok := s.userFromBearer(r); ok {
			u = uu
		}
	}
	if u == "" {
		writeError(w, http.StatusUnauthorized, "требуется вход")
		return
	}
	row, err := s.st.GetAuthUserByUsername(r.Context(), u)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "пользователь не найден")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       row.ID,
		"username": row.Username,
		"role":     row.Role,
	})
}
