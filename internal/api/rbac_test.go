package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"git.kalinamall.ru/PapaTramp/netlynx/internal/config"
)

func TestResolveAuthRoleBasicBootstrap(t *testing.T) {
	s := &Server{cfg: config.Config{AdminUser: "admin", AdminPassword: "secret-password-32chars!!!!"}}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/devices", nil)
	r.SetBasicAuth("admin", "secret-password-32chars!!!!")
	role, ok := s.resolveAuthRole(r)
	if !ok || role != roleAdmin {
		t.Fatalf("expected admin via basic, got ok=%v role=%q", ok, role)
	}
}

func TestResolveAuthRoleEnvBasicOverridesDemotedDBUser(t *testing.T) {
	s := &Server{cfg: config.Config{AdminUser: "admin", AdminPassword: "secret-password-32chars!!!!"}}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	r.SetBasicAuth("admin", "secret-password-32chars!!!!")
	ctx := r.Context()
	ctx = context.WithValue(ctx, authUserKey, "admin")
	r = r.WithContext(ctx)
	role, ok := s.resolveAuthRole(r)
	if !ok || role != roleAdmin {
		t.Fatalf("env basic must stay admin even if context has demoted username, got ok=%v role=%q", ok, role)
	}
}

func TestLoginLimiterBlocks(t *testing.T) {
	l := newLoginLimiter()
	l.maxFail = 3
	ip, user := "1.2.3.4", "admin"
	for i := 0; i < 3; i++ {
		if !l.allow(ip, user) {
			t.Fatalf("unexpected block before fail %d", i)
		}
		l.fail(ip, user)
	}
	if l.allow(ip, user) {
		t.Fatal("expected block after max fails")
	}
}

func TestPatchUserAuditDetails(t *testing.T) {
	role := "operator"
	active := false
	pass := "secret"
	d := patchUserAuditDetails(&role, &active, &pass)
	if d["role"] != "operator" || d["is_active"] != false || d["password_changed"] != true {
		t.Fatalf("%v", d)
	}
	if _, ok := d["password"]; ok {
		t.Fatal("must not log password")
	}
	empty := ""
	d = patchUserAuditDetails(nil, nil, &empty)
	if len(d) != 0 {
		t.Fatalf("empty password must not set password_changed: %v", d)
	}
}
