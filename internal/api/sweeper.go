package api

import (
	"context"
	"log/slog"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// StaleSweeper periodically releases in_progress claims whose lease has
// expired (v18860-1). DISABLED BY DEFAULT: it only becomes load-bearing once
// the pollers run claim renewal (helixon-platform side of the ticket), so an
// operator arms it per deployment via SPRINTBOARD_STALE_SWEEP_INTERVAL.
// Until then the read-only GET /api/v1/tickets/stale view is the only
// stale-claim surface, exactly as before.
type StaleSweeper struct {
	store    *sprintboard.Store
	interval time.Duration
	window   time.Duration
	logger   *slog.Logger
}

// NewStaleSweeper returns a sweeper. interval or window <= 0 means disabled;
// Run is then a no-op, which is how the default-off deployment behaves.
func NewStaleSweeper(store *sprintboard.Store, interval, window time.Duration, logger *slog.Logger) *StaleSweeper {
	if logger == nil {
		logger = slog.Default()
	}
	return &StaleSweeper{store: store, interval: interval, window: window, logger: logger}
}

// Run blocks until ctx is done, sweeping every interval. A sweep that fails
// is logged and retried on the next tick; the sweeper never exits on its own.
func (sw *StaleSweeper) Run(ctx context.Context) {
	if sw.interval <= 0 || sw.window <= 0 {
		sw.logger.Info("stale-claim sweeper disabled (SPRINTBOARD_STALE_SWEEP_INTERVAL unset or non-positive)")
		return
	}
	ticker := time.NewTicker(sw.interval)
	defer ticker.Stop()
	sw.logger.Info("stale-claim sweeper armed",
		slog.Duration("interval", sw.interval), slog.Duration("window", sw.window))
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := sw.SweepOnce(ctx); err != nil {
				sw.logger.Error("stale-claim sweep failed", slog.String("error", err.Error()))
			}
		}
	}
}

// SweepOnce releases every expired in_progress claim and returns how many.
func (sw *StaleSweeper) SweepOnce(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	released, err := sw.store.ReleaseStaleClaims(sw.window)
	if err != nil {
		return 0, err
	}
	if released > 0 {
		sw.logger.Warn("stale-claim sweeper released claims",
			slog.Int64("released", released), slog.Duration("window", sw.window))
	}
	return released, nil
}
