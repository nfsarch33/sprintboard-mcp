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

	srv := api.NewServer(store, logger)
	httpSrv := &http.Server{
		Addr:         *addr,
		Handler:      srv.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("sprintboard-api starting", "addr", *addr, "db", dp)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "error", err)
	}
	fmt.Fprintln(os.Stderr, "sprintboard-api stopped")
}
