package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nfsarch33/helixon-common/jwtauth"
	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// sharedTestToken is a repeated byte on purpose: a hex-looking fixture reads
// as a credential to a secret scanner and fails the gate for every commit
// after it. Length is what the constructor checks; content is irrelevant.
func sharedTestToken() []byte {
	return []byte(strings.Repeat("s", 45))
}

func newSharedOnly(t *testing.T, allowAnon bool, logger *slog.Logger) *JWTAuthenticator {
	t.Helper()
	if logger == nil {
		logger = jwtTestLogger()
	}
	a, err := NewJWTAuthenticator(JWTAuthConfig{SharedToken: sharedTestToken(), AllowAnonymous: allowAnon, Logger: logger})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	return a
}

func TestNewJWTAuthenticator_SharedTokenOnlyHasNoVerifier(t *testing.T) {
	a := newSharedOnly(t, false, nil)
	if a.verifier != nil {
		t.Fatal("verifier must be nil in shared-token-only mode")
	}
}

func TestNewJWTAuthenticator_RejectsShortSharedToken(t *testing.T) {
	_, err := NewJWTAuthenticator(JWTAuthConfig{SharedToken: []byte(strings.Repeat("s", 31))})
	if err == nil {
		t.Fatal("expected error for a 31-byte shared token")
	}
}

func TestNewJWTAuthenticator_RequiresSecretOrSharedToken(t *testing.T) {
	if _, err := NewJWTAuthenticator(JWTAuthConfig{}); err == nil {
		t.Fatal("expected error when neither secret nor shared token is set")
	}
}

func TestJWT_Middleware_SharedTokenAccepted(t *testing.T) {
	a := newSharedOnly(t, false, nil)
	h := a.Middleware(echoClaimsHandler())
	req := httptest.NewRequest("POST", "/api/v1/tickets", nil)
	req.Header.Set("Authorization", "Bearer "+string(sharedTestToken()))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got map[string]string
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["subject"] != sharedTokenSubject {
		t.Errorf("subject = %q, want %q", got["subject"], sharedTokenSubject)
	}
	if got["tenant_id"] != "" {
		t.Errorf("tenant_id = %q, want empty: the shared token carries no tenant", got["tenant_id"])
	}
}

// The positive control for the whole sprint: in required mode a request
// without the token is refused. If this ever passes with 200, nothing above
// it means anything.
func TestJWT_Middleware_SharedTokenMissingIs401InRequiredMode(t *testing.T) {
	a := newSharedOnly(t, false, nil)
	h := a.Middleware(echoClaimsHandler())
	req := httptest.NewRequest("POST", "/api/v1/tickets", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// Shared-token-only mode has no verifier; a wrong bearer must be a 401, not a
// nil-pointer panic.
func TestJWT_Middleware_WrongBearerIs401NotPanicWithoutVerifier(t *testing.T) {
	a := newSharedOnly(t, false, nil)
	h := a.Middleware(echoClaimsHandler())
	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	req.Header.Set("Authorization", "Bearer "+strings.Repeat("w", 40))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestJWT_Middleware_SharedTokenAndJWTCoexist(t *testing.T) {
	a, err := NewJWTAuthenticator(JWTAuthConfig{
		Secret: testJWTSecret(), SharedToken: sharedTestToken(), Issuer: "helixon-test", Logger: jwtTestLogger(),
	})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	h := a.Middleware(echoClaimsHandler())

	tok := mintTestToken(t, a, jwtauth.Claims{Subject: "agent:x", TenantID: "t-1"})
	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("jwt: status = %d, want 200", w.Code)
	}
	var got map[string]string
	_ = json.NewDecoder(w.Body).Decode(&got)
	if got["subject"] != "agent:x" || got["tenant_id"] != "t-1" {
		t.Errorf("jwt claims not attached: %v", got)
	}

	req = httptest.NewRequest("GET", "/api/v1/sprints", nil)
	req.Header.Set("Authorization", "Bearer "+string(sharedTestToken()))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("shared: status = %d, want 200", w.Code)
	}
}

func TestJWT_Middleware_BootstrapLogsAnonymousCensus(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	a := newSharedOnly(t, true, logger)
	h := a.Middleware(echoClaimsHandler())
	req := httptest.NewRequest("POST", "/api/v1/tickets", nil)
	req.Header.Set("User-Agent", "census-probe/1.0")
	req.RemoteAddr = "127.0.0.1:41234"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (bootstrap allows anonymous)", w.Code)
	}
	out := buf.String()
	for _, want := range []string{"anonymous request", "method=POST", "path=/api/v1/tickets", "remote=127.0.0.1:41234", "user_agent=census-probe/1.0"} {
		if !strings.Contains(out, want) {
			t.Errorf("census log lacks %q; got: %s", want, out)
		}
	}
}

// Mutation control for the comparison: a static check that the middleware
// compares the shared token in constant time and never with ==. A timing
// assertion cannot fail reliably in CI; a source assertion can.
func TestJWT_Middleware_SharedTokenUsesConstantTimeCompare(t *testing.T) {
	src, err := os.ReadFile("jwt_middleware.go")
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	s := string(src)
	if !strings.Contains(s, "subtle.ConstantTimeCompare") {
		t.Fatal("jwt_middleware.go must compare the shared token with subtle.ConstantTimeCompare")
	}
	for _, bad := range []string{"== string(a.cfg.SharedToken)", "string(a.cfg.SharedToken) ==", "token == string("} {
		if strings.Contains(s, bad) {
			t.Fatalf("jwt_middleware.go compares the shared token with ==: %q", bad)
		}
	}
}

// The wire test for the handler order: a rejected request must appear in the
// access log with its method and path, which is only true when the JWT
// middleware runs INSIDE the access logger.
func TestServer_Handler_LogsRejectedRequests(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	srv := &Server{logger: logger, metrics: sprintboard.NewMetrics()}
	srv.mux = http.NewServeMux()
	srv.mux.HandleFunc("/api/v1/sprints", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	srv.jwt = newSharedOnly(t, false, logger)

	req := httptest.NewRequest("GET", "/api/v1/sprints", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	out := buf.String()
	if !strings.Contains(out, "msg=request") || !strings.Contains(out, "path=/api/v1/sprints") {
		t.Fatalf("the 401 was not access-logged; JWT must run inside withMiddleware. log: %s", out)
	}
}
