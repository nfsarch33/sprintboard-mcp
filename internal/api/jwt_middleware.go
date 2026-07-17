package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nfsarch33/helixon-common/jwtauth"
)

// ctxKeyClaims is the request-context key under which verified JWT claims
// are stored. Handlers can pull claims via claimsFrom(r.Context()).
type ctxKeyClaims string

const claimsContextKey ctxKeyClaims = "sprintboard.jwt.claims"

// JWTAuthConfig configures the JWT middleware. Issuer is optional; Secret
// must be at least 32 bytes (the underlying jwtauth.NewVerifier enforces this).
// AllowAnonymous, if true, lets requests through without a token — useful for
// the bootstrapping period before tokens are issued everywhere.
type JWTAuthConfig struct {
	Secret          []byte
	Issuer          string
	AllowAnonymous  bool
	AnonymousScopes []string // scopes to assign to anonymous requests
	Logger          *slog.Logger
}

// JWTAuthenticator validates incoming JWTs and attaches the verified claims
// to the request context. Per v18680-2; depends on helixon-common/jwtauth
// (v18680-1).
type JWTAuthenticator struct {
	verifier *jwtauth.Verifier
	cfg      JWTAuthConfig
}

// NewJWTAuthenticator builds a JWTAuthenticator with a verified secret. An
// empty or short secret is rejected (matches jwtauth.NewVerifier's contract).
func NewJWTAuthenticator(cfg JWTAuthConfig) (*JWTAuthenticator, error) {
	if len(cfg.Secret) < 32 {
		return nil, errors.New("jwt secret must be at least 32 bytes")
	}
	v, err := jwtauth.NewVerifier(cfg.Secret, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &JWTAuthenticator{verifier: v, cfg: cfg}, nil
}

// Middleware returns an http.Handler middleware that verifies the
// Authorization: Bearer <token> header on every request, attaches the
// claims to the request context, and rejects unauthorized requests with
// 401. Health and metrics endpoints (/healthz, /readyz, /metrics) bypass
// the middleware so monitoring still works without a token.
func (a *JWTAuthenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bypass for health/metrics
		if isBypassedPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token := bearerToken(r)
		if token == "" {
			if a.cfg.AllowAnonymous {
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusUnauthorized, errors.New("missing bearer token"))
			return
		}

		claims, err := a.verifier.Verify(token)
		if err != nil {
			a.cfg.Logger.Warn("jwt rejected",
				"err", err,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
			)
			writeErr(w, http.StatusUnauthorized, err)
			return
		}

		// Stash claims on context for downstream handlers
		next.ServeHTTP(w, r.WithContext(withClaims(r.Context(), claims)))
	})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// isBypassedPath returns true for endpoints that must remain reachable
// without auth (k8s probes, Prometheus scrape, observability).
func isBypassedPath(p string) bool {
	switch p {
	case "/healthz", "/readyz", "/metrics":
		return true
	}
	return false
}
