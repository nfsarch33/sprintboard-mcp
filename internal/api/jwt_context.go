package api

import (
	"context"

	"github.com/nfsarch33/helixon-common/jwtauth"
)

type ctxKey string

const (
	ctxClaimsKey ctxKey = "sprintboard.jwt.claims"
)

// withClaims returns a derived context carrying the verified JWT claims.
func withClaims(parent context.Context, claims *jwtauth.Claims) context.Context {
	return context.WithValue(parent, ctxClaimsKey, claims)
}

// claimsFrom retrieves the verified JWT claims from the request context, or
// nil if the request is anonymous / did not pass through the JWT middleware.
func claimsFrom(ctx context.Context) *jwtauth.Claims {
	v, _ := ctx.Value(ctxClaimsKey).(*jwtauth.Claims)
	return v
}

// tenantFrom returns the tenant_id claim, or "" if anonymous / unauthenticated.
func tenantFrom(ctx context.Context) string {
	c := claimsFrom(ctx)
	if c == nil {
		return ""
	}
	return c.TenantID
}

// subjectFrom returns the sub claim, or "" if anonymous / unauthenticated.
func subjectFrom(ctx context.Context) string {
	c := claimsFrom(ctx)
	if c == nil {
		return ""
	}
	return c.Subject
}
