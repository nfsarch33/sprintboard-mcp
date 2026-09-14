package api

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/nfsarch33/helixon-common/jwtauth"
)

// The request-context key for verified claims lives in jwt_context.go
// (ctxKey/ctxClaimsKey), alongside withClaims/claimsFrom/tenantFrom. A second
// unused pair was declared here and shadowed nothing; it is removed rather
// than nolint-ed, because two context keys for one value is a bug waiting to
// happen the moment someone reads from the wrong one.

// JWTAuthConfig configures the JWT middleware. Issuer is optional; Secret
// must be at least 32 bytes (the underlying jwtauth.NewVerifier enforces this).
// AllowAnonymous, if true, lets requests through without a token — useful for
// the bootstrapping period before tokens are issued everywhere.
type JWTAuthConfig struct {
	Secret []byte
	// SharedToken, when set, is an opaque bearer accepted by constant-time
	// comparison in addition to (or instead of) signed JWTs. It is the single
	// shared credential the operator's secret bootstrap renders for every
	// writer, and must be at least 32 bytes. A request presenting it carries
	// the synthetic claims {Subject: "shared-token"} and NO tenant claim:
	// tenant scoping on this server is a caller-asserted parameter, not a
	// property of the credential.
	SharedToken     []byte
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

// sharedTokenSubject is the synthetic subject attached to requests that
// authenticate with the shared token.
const sharedTokenSubject = "shared-token"

// NewJWTAuthenticator builds a JWTAuthenticator from a JWT secret, a shared
// token, or both. Each credential that is set must be at least 32 bytes
// (jwtauth.NewVerifier enforces the same for the secret). With only a shared
// token the verifier is nil, and Middleware never dereferences it.
func NewJWTAuthenticator(cfg JWTAuthConfig) (*JWTAuthenticator, error) {
	if len(cfg.Secret) == 0 && len(cfg.SharedToken) == 0 {
		return nil, errors.New("jwt secret or shared token must be set")
	}
	if len(cfg.Secret) > 0 && len(cfg.Secret) < 32 {
		return nil, errors.New("jwt secret must be at least 32 bytes")
	}
	if len(cfg.SharedToken) > 0 && len(cfg.SharedToken) < 32 {
		return nil, errors.New("shared token must be at least 32 bytes")
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	a := &JWTAuthenticator{cfg: cfg}
	if len(cfg.Secret) > 0 {
		v, err := jwtauth.NewVerifier(cfg.Secret, cfg.Issuer)
		if err != nil {
			return nil, err
		}
		a.verifier = v
	}
	return a, nil
}

// matchesSharedToken reports whether token equals the configured shared
// token, compared in constant time. False when no shared token is configured.
func (a *JWTAuthenticator) matchesSharedToken(token string) bool {
	if len(a.cfg.SharedToken) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), a.cfg.SharedToken) == 1
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
				// Bootstrap census: every anonymous caller is named, so the
				// flip to required mode is a decision, never a discovery.
				a.cfg.Logger.Info("anonymous request",
					"method", r.Method,
					"path", r.URL.Path,
					"remote", r.RemoteAddr,
					"user_agent", r.UserAgent(),
				)
				next.ServeHTTP(w, r)
				return
			}
			writeErr(w, http.StatusUnauthorized, errors.New("missing bearer token"))
			return
		}

		if a.matchesSharedToken(token) {
			next.ServeHTTP(w, r.WithContext(withClaims(r.Context(), &jwtauth.Claims{Subject: sharedTokenSubject})))
			return
		}
		if a.verifier == nil {
			// Shared-token-only mode: a bearer that is not the shared token has
			// nothing else to be checked against. Say so without touching the
			// nil verifier.
			a.cfg.Logger.Warn("bearer rejected",
				"reason", "not the shared token and no jwt verifier configured",
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
				"user_agent", r.UserAgent(),
			)
			writeErr(w, http.StatusUnauthorized, errors.New("bearer token not accepted"))
			return
		}

		claims, err := a.verifier.Verify(token)
		if err != nil {
			a.cfg.Logger.Warn("jwt rejected",
				"err", err,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
				"user_agent", r.UserAgent(),
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
