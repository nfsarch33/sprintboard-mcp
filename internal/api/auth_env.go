package api

import (
	"fmt"
	"log/slog"
	"strings"
)

// Authentication is configured from two environment variables read by
// cmd/sprintboard-api at start-up:
//
//	SPRINTBOARD_API_TOKEN  the shared bearer every writer presents; rendered
//	                       by the operator's secret bootstrap; >= 32 bytes
//	SPRINTBOARD_AUTH_MODE  off | bootstrap | required
//
// Modes:
//   - off: no middleware; the API is unauthenticated and the log says so.
//     The default when no token is set.
//   - bootstrap: the shared token is accepted; requests WITHOUT a bearer are
//     still allowed and each one is logged with its remote address and user
//     agent, the census that precedes the flip. The default when a token is
//     set and no mode is given.
//   - required: every non-health route needs the shared token; anonymous
//     requests are answered 401.
//
// The default with a token present is bootstrap, not required, so that
// deploying a binary that understands auth can never lock a caller out by
// itself: the flip to required is an explicit configuration change with its
// own positive control (an unauthenticated POST must answer 401).
const (
	AuthModeOff       = "off"
	AuthModeBootstrap = "bootstrap"
	AuthModeRequired  = "required"

	EnvAPIToken = "SPRINTBOARD_API_TOKEN"
	EnvAuthMode = "SPRINTBOARD_AUTH_MODE"

	minSharedTokenBytes = 32
)

// AuthFromEnv builds the authenticator the server should install, or nil
// when the mode is off. The returned mode is the effective one after
// defaulting. getenv is injected so the resolution is testable without the
// process environment; error messages never carry the token's value.
func AuthFromEnv(getenv func(string) string, logger *slog.Logger) (*JWTAuthenticator, string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	token := strings.TrimSpace(getenv(EnvAPIToken))
	mode := strings.ToLower(strings.TrimSpace(getenv(EnvAuthMode)))
	if mode == "" {
		if token == "" {
			mode = AuthModeOff
		} else {
			mode = AuthModeBootstrap
		}
	}
	switch mode {
	case AuthModeOff:
		if token != "" {
			logger.Warn("auth mode is off although a shared token is configured", "env", EnvAuthMode)
		}
		return nil, mode, nil
	case AuthModeBootstrap, AuthModeRequired:
	default:
		return nil, "", fmt.Errorf("%s=%q is not one of %s|%s|%s", EnvAuthMode, mode, AuthModeOff, AuthModeBootstrap, AuthModeRequired)
	}
	if token == "" {
		return nil, "", fmt.Errorf("%s=%s requires %s to be set", EnvAuthMode, mode, EnvAPIToken)
	}
	if len(token) < minSharedTokenBytes {
		return nil, "", fmt.Errorf("%s must be at least %d bytes (got %d)", EnvAPIToken, minSharedTokenBytes, len(token))
	}
	auth, err := NewJWTAuthenticator(JWTAuthConfig{
		SharedToken:    []byte(token),
		AllowAnonymous: mode == AuthModeBootstrap,
		Logger:         logger,
	})
	if err != nil {
		return nil, "", err
	}
	return auth, mode, nil
}
