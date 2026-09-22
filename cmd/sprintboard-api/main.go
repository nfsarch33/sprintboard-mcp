package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/api"
	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func main() {
	addr := flag.String("addr", ":9400", "Listen address")
	dbPath := flag.String("db", "", "SQLite database path (default: ~/.config/helix-dev-tools/sprintboard.db)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dp := *dbPath
	if dp == "" {
		dp = sprintboard.DefaultDBPath()
	}

	store, err := sprintboard.NewStore(dp)
	if err != nil {
		logger.Error("open database", "error", err, "path", dp)
		os.Exit(1)
	}

	logger.Info("database opened", "dialect", store.Dialect())

	srv := api.NewServer(store, logger)
	auth, mode, err := api.AuthFromEnv(os.Getenv, logger)
	if err != nil {
		logger.Error("auth configuration", "error", err)
		os.Exit(1)
	}
	if auth == nil {
		logger.Warn("API is UNAUTHENTICATED: set "+api.EnvAPIToken+" (and "+api.EnvAuthMode+"=required) to enforce the shared bearer", "auth_mode", mode)
	} else {
		srv.SetJWTAuth(auth)
		logger.Info("auth configured", "auth_mode", mode)
	}
	httpSrv := &http.Server{
		Addr:         *addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// v18860-1: stale-claim sweeper, DISABLED BY DEFAULT. Arming it is a
	// per-deployment decision: it must only run once the pollers renew their
	// claims (helixon-platform RenewClaim), or it will release tickets out
	// from under legitimately long runs. SPRINTBOARD_STALE_SWEEP_INTERVAL
	// (e.g. "5m") arms it; SPRINTBOARD_STALE_SWEEP_WINDOW (default 30m,
	// matching the MCP server's sweep) is the lease age that expires.
	sweepInterval := parseDurationEnv(os.Getenv("SPRINTBOARD_STALE_SWEEP_INTERVAL"), 0)
	sweepWindow := parseDurationEnv(os.Getenv("SPRINTBOARD_STALE_SWEEP_WINDOW"), 30*time.Minute)
	go api.NewStaleSweeper(store, sweepInterval, sweepWindow, logger).Run(ctx)

	go func() {
		logger.Info("sprintboard-api starting", "addr", *addr, "db", dp)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	srv.SetShuttingDown()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "error", err)
	}
	fmt.Fprintln(os.Stderr, "sprintboard-api stopped")
}

// parseDurationEnv parses a Go duration from an env value, returning fallback
// when unset or unparsable (a bad sweep interval must not take the board
// down; it logs as disabled/defaults instead).
func parseDurationEnv(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
