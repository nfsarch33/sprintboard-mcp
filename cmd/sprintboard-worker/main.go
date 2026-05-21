package main

import (
	"flag"
	"log"
	"log/slog"
	"os"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
	spbtemporal "github.com/nfsarch33/sprintboard-mcp/internal/temporal"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	dbPath := flag.String("db", "", "SQLite database path (default: ~/.config/helix-dev-tools/sprintboard.db)")
	temporalAddr := flag.String("temporal-addr", "localhost:7233", "Temporal server address")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dp := *dbPath
	if dp == "" {
		dp = sprintboard.DefaultDBPath()
	}

	store, err := sprintboard.NewStore(dp)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()

	c, err := client.Dial(client.Options{
		HostPort: *temporalAddr,
	})
	if err != nil {
		log.Fatalf("connect to temporal: %v", err)
	}
	defer c.Close()

	w := worker.New(c, spbtemporal.TaskQueue, worker.Options{})

	acts := &spbtemporal.Activities{Store: store}

	w.RegisterWorkflow(spbtemporal.SprintWorkflow)
	w.RegisterWorkflow(spbtemporal.TicketWorkflow)

	w.RegisterActivityWithOptions(acts.ActivateSprint, activity.RegisterOptions{Name: spbtemporal.ActivateSprint})
	w.RegisterActivityWithOptions(acts.CloseSprint, activity.RegisterOptions{Name: spbtemporal.CloseSprint})
	w.RegisterActivityWithOptions(acts.ClaimTicketActivity, activity.RegisterOptions{Name: spbtemporal.ClaimTicketActivity})
	w.RegisterActivityWithOptions(acts.CompleteTicketActivity, activity.RegisterOptions{Name: spbtemporal.CompleteTicketActivity})
	w.RegisterActivityWithOptions(acts.HeartbeatActivity, activity.RegisterOptions{Name: spbtemporal.HeartbeatActivity})
	w.RegisterActivityWithOptions(acts.HandoffActivity, activity.RegisterOptions{Name: spbtemporal.HandoffActivity})

	logger.Info("sprintboard-worker starting", "task_queue", spbtemporal.TaskQueue, "temporal", *temporalAddr, "db", dp)

	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatalf("worker failed: %v", err)
	}
}
