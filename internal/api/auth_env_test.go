package api

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// A repeated byte, not a hex-looking literal, so no secret scanner mistakes
// the fixture for a credential.
var goodToken = strings.Repeat("g", 39)

func TestAuthFromEnv_OffWhenTokenUnset(t *testing.T) {
	auth, mode, err := AuthFromEnv(envOf(map[string]string{}), jwtTestLogger())
	if err != nil || auth != nil || mode != AuthModeOff {
		t.Fatalf("got auth=%v mode=%q err=%v; want nil/off/nil", auth, mode, err)
	}
}

func TestAuthFromEnv_BootstrapIsTheDefaultWhenTokenSet(t *testing.T) {
	auth, mode, err := AuthFromEnv(envOf(map[string]string{EnvAPIToken: goodToken}), jwtTestLogger())
	if err != nil || auth == nil {
		t.Fatalf("got auth=%v err=%v", auth, err)
	}
	if mode != AuthModeBootstrap || !auth.cfg.AllowAnonymous {
		t.Fatalf("mode=%q allowAnonymous=%v; want bootstrap/true", mode, auth.cfg.AllowAnonymous)
	}
}

func TestAuthFromEnv_RequiredWhenAsked(t *testing.T) {
	auth, mode, err := AuthFromEnv(envOf(map[string]string{EnvAPIToken: goodToken, EnvAuthMode: "required"}), jwtTestLogger())
	if err != nil || auth == nil {
		t.Fatalf("got auth=%v err=%v", auth, err)
	}
	if mode != AuthModeRequired || auth.cfg.AllowAnonymous {
		t.Fatalf("mode=%q allowAnonymous=%v; want required/false", mode, auth.cfg.AllowAnonymous)
	}
}

func TestAuthFromEnv_RequiredWithoutTokenErrors(t *testing.T) {
	if _, _, err := AuthFromEnv(envOf(map[string]string{EnvAuthMode: "required"}), jwtTestLogger()); err == nil {
		t.Fatal("expected an error: required mode with no token")
	}
}

func TestAuthFromEnv_ShortTokenErrorsWithoutEchoingIt(t *testing.T) {
	short := strings.Repeat("g", 25)
	_, _, err := AuthFromEnv(envOf(map[string]string{EnvAPIToken: short, EnvAuthMode: "required"}), jwtTestLogger())
	if err == nil {
		t.Fatal("expected an error for a short token")
	}
	if strings.Contains(err.Error(), short) {
		t.Fatalf("error message echoes the token value: %v", err)
	}
}

func TestAuthFromEnv_UnknownModeErrors(t *testing.T) {
	if _, _, err := AuthFromEnv(envOf(map[string]string{EnvAPIToken: goodToken, EnvAuthMode: "maybe"}), jwtTestLogger()); err == nil {
		t.Fatal("expected an error for an unknown mode")
	}
}

func TestAuthFromEnv_OffWithTokenIsHonoured(t *testing.T) {
	auth, mode, err := AuthFromEnv(envOf(map[string]string{EnvAPIToken: goodToken, EnvAuthMode: "off"}), jwtTestLogger())
	if err != nil || auth != nil || mode != AuthModeOff {
		t.Fatalf("got auth=%v mode=%q err=%v; want nil/off/nil", auth, mode, err)
	}
}
