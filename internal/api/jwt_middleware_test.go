package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nfsarch33/helixon-common/jwtauth"
	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func jwtTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func testJWTSecret() []byte {
	return []byte("0123456789abcdef0123456789abcdef0123456789abcdef") // 54 bytes
}

func mintTestToken(t *testing.T, auth *JWTAuthenticator, claims jwtauth.Claims) string {
	t.Helper()
	if claims.Expires == 0 {
		now := time.Now().Unix()
		claims.Expires = now + 3600
	}
	tok, err := auth.verifier.Sign(claims, time.Hour)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	return tok
}

// minimal downstream handler that echoes the claims (or anonymous) for assertions
func echoClaimsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := claimsFrom(r.Context())
		out := map[string]string{"subject": "", "tenant_id": ""}
		if c != nil {
			out["subject"] = c.Subject
			out["tenant_id"] = c.TenantID
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
}

func newAuthWithAuthenticator(t *testing.T, allowAnon bool) *JWTAuthenticator {
	t.Helper()
	a, err := NewJWTAuthenticator(JWTAuthConfig{
		Secret:         testJWTSecret(),
		Issuer:         "helixon-test",
		AllowAnonymous: allowAnon,
		Logger:         jwtTestLogger(),
	})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	return a
}

func TestNewJWTAuthenticator_RejectsShortSecret(t *testing.T) {
	if _, err := NewJWTAuthenticator(JWTAuthConfig{Secret: []byte("short")}); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestJWT_Middleware_RejectsMissingToken(t *testing.T) {
	auth := newAuthWithAuthenticator(t, false)
	h := auth.Middleware(echoClaimsHandler())

	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestJWT_Middleware_AcceptsValidToken(t *testing.T) {
	auth := newAuthWithAuthenticator(t, false)
	tok := mintTestToken(t, auth, jwtauth.Claims{
		Subject:  "agent:helixon-platform",
		TenantID: "tenant-abc",
	})
	h := auth.Middleware(echoClaimsHandler())

	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["subject"] != "agent:helixon-platform" {
		t.Errorf("subject = %q", got["subject"])
	}
	if got["tenant_id"] != "tenant-abc" {
		t.Errorf("tenant_id = %q", got["tenant_id"])
	}
}

func TestJWT_Middleware_RejectsBadSignature(t *testing.T) {
	auth := newAuthWithAuthenticator(t, false)
	tok := mintTestToken(t, auth, jwtauth.Claims{Subject: "x", TenantID: "t"})
	// Tamper signature
	tampered := tok[:len(tok)-2] + "XX"
	h := auth.Middleware(echoClaimsHandler())

	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	req.Header.Set("Authorization", "Bearer "+tampered)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestJWT_Middleware_BypassesHealthAndMetrics(t *testing.T) {
	auth := newAuthWithAuthenticator(t, false)
	h := auth.Middleware(echoClaimsHandler())

	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200 (bypassed)", path, w.Code)
		}
	}
}

func TestJWT_Middleware_AllowAnonymousPassesThrough(t *testing.T) {
	auth := newAuthWithAuthenticator(t, true)
	h := auth.Middleware(echoClaimsHandler())

	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (anonymous allowed)", w.Code)
	}
}

func TestJWT_Middleware_RejectsNonBearerAuth(t *testing.T) {
	auth := newAuthWithAuthenticator(t, false)
	h := auth.Middleware(echoClaimsHandler())

	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestClaimsHelpers(t *testing.T) {
	if tenantFrom(context.Background()) != "" {
		t.Errorf("expected empty tenant from empty ctx")
	}
	if subjectFrom(context.Background()) != "" {
		t.Errorf("expected empty subject from empty ctx")
	}
	c := &jwtauth.Claims{Subject: "x", TenantID: "t-1"}
	ctx := withClaims(context.Background(), c)
	if tenantFrom(ctx) != "t-1" {
		t.Errorf("tenantFrom(ctx) = %q, want t-1", tenantFrom(ctx))
	}
	if subjectFrom(ctx) != "x" {
		t.Errorf("subjectFrom(ctx) = %q, want x", subjectFrom(ctx))
	}
	if claimsFrom(ctx) != c {
		t.Errorf("claimsFrom(ctx) round-trip mismatch")
	}
}

func TestServer_JWTWiring_RequiresToken(t *testing.T) {
	// Build a server with the SQLite-free path: a stub store is hard here,
	// so we focus on the JWT wiring (which doesn't touch the store). The
	// store nil path is exercised in TestServer_NewServer via Handler().
	auth := newAuthWithAuthenticator(t, false)

	// Build a minimal handler that mimics the server's Handler() pipeline:
	// echo + jwt
	h := auth.Middleware(echoClaimsHandler())
	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("wiring check: status = %d, want 401", w.Code)
	}
}

// Smoke test: integration with a real Server instance still serves /healthz
// and rejects /api/* without a token. We can't easily exercise /api/* happy
// path here because that needs a real Store. This is a wiring-only assertion.
func TestServer_JWTWiring_HealthzBypasses(t *testing.T) {
	srv := &Server{
		logger:  jwtTestLogger(),
		metrics: sprintboard.NewMetrics(),
	}
	srv.mux = http.NewServeMux()
	srv.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	auth := newAuthWithAuthenticator(t, false)
	srv.jwt = auth

	// healthz should work
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200 (jwt bypassed)", w.Code)
	}

	// /api/v1/* should reject without token (no handler registered, but
	// jwt middleware rejects BEFORE mux dispatch)
	req = httptest.NewRequest("GET", "/api/v1/sprints", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("/api/v1/sprints status = %d, want 401 (jwt enforced)", w.Code)
	}
}

// Ensure slog logger is captured; ensures test compiles even if unused
var _ = os.Stderr

// silence unused import warning on strings when no test uses it
var _ = strings.Contains
