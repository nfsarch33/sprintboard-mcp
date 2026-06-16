// sprintboard-cli.go — v13910 S5.1: operator/admin CLI for the SprintBoard store.
//
// Provides direct database access to a SprintBoard SQLite (or PostgreSQL)
// store, for catalog maintenance, health checks, and seeding. Designed to
// run as a one-shot admin command from the same host that runs
// sprintboard-api. Reads SPRINTBOARD_DB_URL for PostgreSQL, otherwise opens
// the default SQLite path.
//
// Subcommands:
//
//	sprintboard clean      — Remove stale ticket rows (older than N days
//	                         AND matching a configurable ID pattern).
//	                         Default pattern: `session-operator-*`; default
//	                         age: 30 days. Use --dry-run to preview.
//	sprintboard seed       — Insert v13910-relevant catalog tickets
//	                         (S5-S8 ticket stubs + Helixon fleet
//	                         candidates). Idempotent: re-running with the
//	                         same IDs is a no-op.
//	sprintboard health     — Probe the running sprintboard-api /healthz
//	                         endpoint and report reachability + recent
//	                         ticket counts + stale-catalog warnings.
//	sprintboard list       — List tickets (filterable by sprint ID or
//	                         status) for operator eyeballing.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

const (
	defaultBaseURL    = "http://localhost:9400"
	defaultStaleDays  = 30
	defaultStaleWarn  = 7
	defaultSeedSprint = "v13910"
)

var usage = `sprintboard-cli — SprintBoard store admin (v13910 S5.1)

Usage:
  sprintboard-cli <subcommand> [flags]

Subcommands:
  clean    Remove stale ticket rows (default: session-operator-* older than 30d)
  seed     Seed the catalog with v13910-relevant ticket stubs
  health   Probe /healthz + report ticket count + stale-stub warnings
  list     List tickets (optionally filter by sprint or status)
  version  Print version and exit

Common flags:
  -db PATH     SQLite path (overrides SPRINTBOARD_DB_URL; default: ~/.config/helix-dev-tools/sprintboard.db)
  -base URL    Base URL of sprintboard-api for health/list probe (default: http://localhost:9400)
  -json        Emit JSON output (machine-readable)
  -v           Verbose logging to stderr

Run ` + "`sprintboard-cli <subcommand> -h`" + ` for subcommand-specific flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	// Global common flags are accepted anywhere in argv. We walk argv
	// to extract them, then pass the remaining (subcommand + its own
	// flags) to the per-subcommand dispatcher.
	cf := &commonFlags{}
	argv := os.Args[1:]
	argv, sub, tail := splitCommon(cf, argv)

	if sub == "" {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	switch sub {
	case "clean":
		os.Exit(runClean(cf, tail))
	case "seed":
		os.Exit(runSeed(cf, tail))
	case "health":
		os.Exit(runHealth(cf, tail))
	case "list":
		os.Exit(runList(cf, tail))
	case "version", "-version", "--version":
		fmt.Println("sprintboard-cli dev (v13910 S5.1)")
		os.Exit(0)
	case "-h", "--help", "help":
		fmt.Print(usage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "sprintboard-cli: unknown subcommand %q\n\n%s", sub, usage)
		os.Exit(2)
	}
}

// splitCommon pulls -db, -base, -json, -v (and their values) out of
// argv and returns the remaining tail. The first non-common arg is
// treated as the subcommand and the rest is its flag tail. We do NOT
// mutate argv in place — the common flags are stored in cf.
func splitCommon(cf *commonFlags, argv []string) (rest []string, sub string, tail []string) {
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		switch arg {
		case "-db":
			if i+1 < len(argv) {
				cf.dbPath = argv[i+1]
				i++
			}
		case "-base":
			if i+1 < len(argv) {
				cf.baseURL = argv[i+1]
				i++
			}
		case "-json":
			cf.jsonOut = true
		case "-v":
			cf.verbose = true
		default:
			// First non-common arg is the subcommand.
			return argv, arg, argv[i+1:]
		}
	}
	return argv, "", nil
}

// ---------------------------------------------------------------------------
// common
// ---------------------------------------------------------------------------

type commonFlags struct {
	dbPath  string
	baseURL string
	jsonOut bool
	verbose bool
}

func openStore(cf *commonFlags) (*sprintboard.Store, func() error, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if dsn := os.Getenv("SPRINTBOARD_DB_URL"); dsn != "" {
		_ = ctx
		store, err := sprintboard.NewStore(cf.dbPath)
		if err != nil {
			return nil, nil, fmt.Errorf("open store (postgres): %w", err)
		}
		return store, store.Close, nil
	}

	path := cf.dbPath
	if path == "" {
		path = sprintboard.DefaultDBPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, nil, fmt.Errorf("create db dir: %w", err)
	}
	store, err := sprintboard.NewStore(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open store (sqlite): %w", err)
	}
	if cf.verbose {
		fmt.Fprintf(os.Stderr, "sprintboard-cli: opened %s (dialect=%s)\n", path, store.Dialect())
	}
	return store, store.Close, nil
}

func logf(cf *commonFlags, format string, args ...interface{}) {
	if cf.verbose {
		fmt.Fprintf(os.Stderr, "sprintboard-cli: "+format+"\n", args...)
	}
}

// ---------------------------------------------------------------------------
// clean
// ---------------------------------------------------------------------------

func runClean(cf *commonFlags, args []string) int {
	fs := flag.NewFlagSet("clean", flag.ContinueOnError)
	pattern := fs.String("pattern", "session-operator-*", "Ticket ID glob pattern to consider stale")
	staleDays := fs.Int("stale-days", defaultStaleDays, "Remove rows whose created_at is older than N days")
	dryRun := fs.Bool("dry-run", false, "Preview the rows that would be removed; do not delete")
	keepSprint := fs.String("keep-sprint", "", "Never delete tickets whose sprint_id matches this value")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	store, closer, err := openStore(cf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closer()

	tickets, err := store.ListTickets("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "list tickets: %v\n", err)
		return 1
	}

	cutoff := time.Now().AddDate(0, 0, -*staleDays)
	type row struct {
		ID        string    `json:"id"`
		Title     string    `json:"title"`
		SprintID  string    `json:"sprint_id,omitempty"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
		AgeDays   int       `json:"age_days"`
	}
	var stale []row
	for _, t := range tickets {
		matched, err := filepath.Match(*pattern, t.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bad pattern %q: %v\n", *pattern, err)
			return 2
		}
		if !matched {
			continue
		}
		if *keepSprint != "" && t.SprintID == *keepSprint {
			continue
		}
		created := t.CreatedAt
		if created.IsZero() {
			// Stub rows (created_at = 0001-01-01) are always stale.
			created = time.Time{}
		}
		ageDays := int(time.Since(created).Hours() / 24)
		if !created.IsZero() && !created.Before(cutoff) {
			continue
		}
		stale = append(stale, row{
			ID:        t.ID,
			Title:     t.Title,
			SprintID:  t.SprintID,
			Status:    string(t.Status),
			CreatedAt: created,
			AgeDays:   ageDays,
		})
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].ID < stale[j].ID })

	report := map[string]interface{}{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"subcommand": "clean",
		"pattern":    *pattern,
		"stale_days": *staleDays,
		"dry_run":    *dryRun,
		"keep_sprint": *keepSprint,
		"stale_count": len(stale),
		"stale":      stale,
	}
	if cf.jsonOut || !*dryRun {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
	} else {
		fmt.Fprintf(os.Stderr, "sprintboard-cli: %d stale rows match pattern=%q older-than=%dd (dry-run; no deletions)\n",
			len(stale), *pattern, *staleDays)
		for _, r := range stale {
			fmt.Fprintf(os.Stderr, "  - %s (sprint=%q status=%q age=%dd)\n", r.ID, r.SprintID, r.Status, r.AgeDays)
		}
	}

	if *dryRun {
		return 0
	}
	removed := 0
	for _, r := range stale {
		if err := store.DeleteTicket(r.ID); err != nil {
			fmt.Fprintf(os.Stderr, "delete %s: %v\n", r.ID, err)
			continue
		}
		removed++
	}
	fmt.Fprintf(os.Stderr, "sprintboard-cli: removed %d stale rows (requested %d)\n", removed, len(stale))
	return 0
}

// ---------------------------------------------------------------------------
// seed
// ---------------------------------------------------------------------------

type seedTicket struct {
	ID                 string
	Title              string
	Description        string
	Status             string
	OwnerAgent         string
	Priority           int
	AcceptanceCriteria string
}

func defaultSeedTickets() []seedTicket {
	now := time.Now().UTC()
	_ = now
	return []seedTicket{
		{
			ID:          "v13910-s5-t1",
			Title:       "SprintBoard MCP+server fix (S5.1)",
			Description: "Catalog cleanup (`sprintboard clean`), seed tickets (`sprintboard seed`), restartPolicy:Always, ServiceMonitor, helix-dev-tools doctor sprintboard subcommand. See plans/v13910 §5 S5.1 and evidence/sprintboard-mcp-v13910-2026-06-17.md.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-devops-agent",
			Priority:    100,
			AcceptanceCriteria: "`sprintboard clean --dry-run` reports 0 rows; `sprintboard seed` is idempotent; `doctor sprintboard` shows 0 stale `session-operator-*` rows >7d; ServiceMonitor wired; restartPolicy:Always present.",
		},
		{
			ID:          "v13910-s5-t2",
			Title:       "win1/wsl1 reboot auto-recovery audit (S5.2)",
			Description: "19-component audit. Per-component table with restart-mechanism, status, fix-needed. Handoff: sop/handoff-win1-wsl1-reboot-recovery-v13910.md. Update evidence/auto-restart-audit-v13910-2026-06-17.md.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-devops-agent",
			Priority:    95,
			AcceptanceCriteria: "19 components audited (Engram x6, Agentrace, EvoSpine-DRL, Temporal x4, Prometheus x5, OTel, k3s, llm-cluster-router, Harbor x7, fleet-agent, fleet-doctor, vllm-3090, vllm-4070ti, llm-router, SprintBoard MCP+server = 38+1=39+1=40). Handoff doc 7-section format per Rule §1.",
		},
		{
			ID:          "v13910-s5-t3",
			Title:       "S5 closeout + 3-way SHA + handoff (S5.3)",
			Description: "Merge v13910-s5 branches to main on wsl1 SOT, push to GitLab, capture tree-SHA parity, write handoff doc per Rule §1, update carry-forward-register.ndjson, record Agentrace span sprint:v13910-s5.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    90,
			AcceptanceCriteria: "Branches merged; 3-way SHA proof at evidence/three-way-sha-v13910-s5-2026-06-17.md; handoff at sop/handoff-v13910-s5-sprintboard-reboot.md; CF register updated; Agentrace span recorded.",
		},
		{
			ID:          "v13910-s6-t1",
			Title:       "Comprehensive code review (S6.1)",
			Description: "v13900 + v13910 S1-S5 diffs. golangci-lint, govulncheck, gosec, ruff, mypy, bandit, kubeval, conftest, shellcheck. Wire each linter as GitLab CI job. Wire CI-failure notifier hook.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    80,
			AcceptanceCriteria: "evidence/code-review-v13900-v13910-2026-06-17.md with per-MR findings + sign-off. All linters in GitLab CI. CI-failure-notify hook installed and tested.",
		},
		{
			ID:          "v13910-s6-t2",
			Title:       "Workspace housekeeping + cleanup (S6.2)",
			Description: "Run helix-dev-tools doctor workspace across all touched repos. Resolve github-sync blocker. Delete merged local branches. Update .gitignore.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-devops-agent",
			Priority:    75,
			AcceptanceCriteria: "evidence/workspace-housekeeping-v13910-2026-06-17.md with per-repo clean-state. github-sync blocker resolved OR workaround documented + applied.",
		},
		{
			ID:          "v13910-s6-t3",
			Title:       "EvoSpine self-eval cycle (S6.3)",
			Description: "Run the Observe-Reflect-Heal-Evolve-Promote cycle. Surface top 3 improvement candidates with (a) current state, (b) target state, (c) experiment, (d) success metric, (e) carry to v13911/v13912.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    70,
			AcceptanceCriteria: "evidence/evospine-self-eval-v13910-2026-06-17.md with 3 candidates.",
		},
		{
			ID:          "v13910-s7-t1",
			Title:       "Eval agent/harness (S7.1)",
			Description: "Build cursor-global-kb/eval/harness (or helix-dev-tools-phasea/internal/eval/harness.go) with 6 scenarios (ticket-intake, code-edit, test-run, PR-creation, CI-run, handoff-write). Wire as nightly GitLab CI job + Grafana dashboard.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    65,
			AcceptanceCriteria: "evidence/eval-harness-v13910-2026-06-17.md with first eval run results.",
		},
		{
			ID:          "v13910-s7-t2",
			Title:       "Helixon fleet autonomy loop (S7.2)",
			Description: "Wire HelixonFleetContinuousEvalWorkflow into Temporal. Runs every 6h. Picks latest agent artifact, runs eval suite, posts score to Prometheus + Agenttrace, alerts on regression >5%.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-devops-agent",
			Priority:    60,
			AcceptanceCriteria: "evidence/helixon-fleet-autonomy-v13910-2026-06-17.md with workflow registration confirmation.",
		},
		{
			ID:          "v13910-s7-t3",
			Title:       "Skill transfer to fleet agents (S7.3)",
			Description: "Append v13910 closeout learnings to helixon-fleet-{coding,devops}-agent.md. Add 3 new global rules (worktree/stash hygiene, branch-review-before-new-PR, quality-over-time).",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    55,
			AcceptanceCriteria: "evidence/helixon-fleet-skill-transfer-v13910-2026-06-17.md with the diff.",
		},
		{
			ID:          "v13910-s8-t1",
			Title:       "Final closeout + 3-way SHA (S8.1)",
			Description: "Aggregate all 7 sprint handoff docs + Agentrace spans + closeout MRs into session-handoffs/overnight-closeout-2026-06-16-v13910.md. Capture wsl1 SOT, GitLab main, GitHub main SHAs across 4 repos. Update daily-startup-prompt.md + vendor-mirror-index.yaml.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    50,
			AcceptanceCriteria: "Final closeout handoff at sop/handoff-v13910-final-2026-06-17.md (7 sections).",
		},
		{
			ID:          "v13910-s8-t2",
			Title:       "Carry-forwards to v13911 (S8.2)",
			Description: "Document all v13911 carry-forwards: engram ns delete, O3/O4 rename, op signin cron, HARBOR_ROBOT_TOKEN rotation, fleet-doctor, vendor-mirror-repo research, vLLM v0.7+ upgrade, multi-tenant fleet isolation, estimate-tuning retraining, S6.3 candidates.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    45,
			AcceptanceCriteria: "carry-forward-register.ndjson has v13911 backlog populated; new plan file drafted.",
		},
		// Helixon fleet candidates — derived from the S2.3 + S4 cycle evidence
		// (see evidence/s4-housekeeping-2026-06-17.md and the S4.2/S4.3
		// execution traces). These represent small, well-defined, reversible
		// tickets that an autonomous fleet coding agent can pick up safely.
		{
			ID:          "fleet-cand-001",
			Title:       "sprintboard-mcp: add metrics endpoint",
			Description: "Expose a /metrics endpoint on sprintboard-api that emits Prometheus counters: tickets_created_total, tickets_claimed_total, tickets_completed_total, agents_registered_total. Follow the same JSON-handler + middleware-chain pattern as /healthz.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    40,
			AcceptanceCriteria: "`curl :9400/metrics` returns Prometheus text format. Counters increment on the corresponding API calls. Unit tests cover increment + format.",
		},
		{
			ID:          "fleet-cand-002",
			Title:       "sprintboard-mcp: add ticket priority sort",
			Description: "The /api/v1/sprints/{id}/tickets endpoint currently sorts by `priority DESC, created_at ASC` which is good, but agents consume it without a stable sort tiebreaker on equal priority. Add `id ASC` as a final tiebreaker for deterministic ordering.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    35,
			AcceptanceCriteria: "Unit test: insert 5 tickets with priority=0; list them; assert order is deterministic across 100 calls.",
		},
		{
			ID:          "fleet-cand-003",
			Title:       "cursor-global-kb: backport S3.7 sampling guardrails to S3 handoff",
			Description: "S4-T1 from S4 (PROPOSED-ONLY, soft go-live). Add a §3a (corrected matrix) and §3b (sampling guardrails) to sop/handoff-v13910-s3-gpu-deploy.md. Spec in evidence/s4-ticket-spec-S4-T1-2026-06-17.md.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    30,
			AcceptanceCriteria: "Diff matches evidence/s4-ticket-spec-S4-T1-2026-06-17.md §3 byte-for-byte. link-integrity check on all 4 relative .md links passes.",
		},
		{
			ID:          "fleet-cand-004",
			Title:       "helix-dev-tools-phasea: doctor_llm §15 floor + S3.7 sampling tests",
			Description: "S4-T2 from S4 (PROPOSED-ONLY, soft go-live). Add 6 regression tests + 2 supporting const maps to internal/cli/doctor_llm* for §15 capability-floor + S3.7 sampling-presence coverage. Spec in evidence/s4-ticket-spec-S4-T2-2026-06-17.md.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-coding-agent",
			Priority:    30,
			AcceptanceCriteria: "go test ./internal/cli -run DoctorLLM -v passes 11 tests (5 existing + 6 new). Coverage on internal/cli/doctor_llm.go >= 80%.",
		},
		{
			ID:          "fleet-cand-005",
			Title:       "sprintboard-mcp: ServiceMonitor + restartPolicy:Always verification",
			Description: "Verify the k3s deployment manifest in cursor-global-kb/k8s/sprintboard/deployment.yaml has restartPolicy: Always (currently missing) + a ServiceMonitor (currently missing). Add both, write evidence/three-way-sha-sprintboard-v13910-2026-06-17.md.",
			Status:      "ready",
			OwnerAgent:  "helixon-fleet-devops-agent",
			Priority:    25,
			AcceptanceCriteria: "kustomize build cursor-global-kb/k8s/sprintboard/ | grep -E 'restartPolicy|ServiceMonitor' returns 2 hits. Evidence file committed.",
		},
	}
}

func runSeed(cf *commonFlags, args []string) int {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	sprintID := fs.String("sprint", defaultSeedSprint, "Sprint ID to assign seeded tickets to")
	onlyNew := fs.Bool("only-new", false, "Only insert tickets whose ID is not already present (skip existing)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	store, closer, err := openStore(cf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closer()

	// Ensure the sprint exists.
	if err := store.CreateSprint(sprintboard.Sprint{
		ID:     *sprintID,
		Name:   "v13910 SprintBoard catalog",
		Status: sprintboard.SprintActive,
		Theme:  "v13910 S5 catalog seed (SprintBoard MCP+server fix + reboot recovery)",
	}); err != nil {
		logf(cf, "create sprint %q: %v (likely already exists; continuing)", *sprintID, err)
	}

	seeds := defaultSeedTickets()
	inserted := 0
	skipped := 0
	report := make([]map[string]interface{}, 0, len(seeds))
	for _, s := range seeds {
		existing, err := store.GetTicket(s.ID)
		if err == nil && existing.ID != "" {
			if *onlyNew {
				skipped++
				report = append(report, map[string]interface{}{"id": s.ID, "action": "skipped-exists"})
				continue
			}
			// Re-insert is not allowed (PK conflict). Report skipped.
			skipped++
			report = append(report, map[string]interface{}{"id": s.ID, "action": "skipped-exists"})
			continue
		}
		t := sprintboard.Ticket{
			ID:                 s.ID,
			SprintID:           *sprintID,
			Title:              s.Title,
			Description:        s.Description,
			Status:             sprintboard.TicketStatus(s.Status),
			OwnerAgent:         s.OwnerAgent,
			Priority:           s.Priority,
			AcceptanceCriteria: s.AcceptanceCriteria,
		}
		if t.Status == "" {
			t.Status = sprintboard.StatusReady
		}
		if err := store.CreateTicket(t); err != nil {
			fmt.Fprintf(os.Stderr, "insert %s: %v\n", s.ID, err)
			report = append(report, map[string]interface{}{"id": s.ID, "action": "error", "error": err.Error()})
			continue
		}
		inserted++
		report = append(report, map[string]interface{}{"id": s.ID, "action": "inserted"})
	}

	out := map[string]interface{}{
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
		"subcommand":     "seed",
		"sprint":         *sprintID,
		"inserted":       inserted,
		"skipped":        skipped,
		"only_new":       *onlyNew,
		"results":        report,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	fmt.Fprintf(os.Stderr, "sprintboard-cli: seed complete (inserted=%d skipped=%d sprint=%q)\n", inserted, skipped, *sprintID)
	return 0
}

// ---------------------------------------------------------------------------
// health
// ---------------------------------------------------------------------------

type healthCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

func runHealth(cf *commonFlags, args []string) int {
	fs := flag.NewFlagSet("health", flag.ContinueOnError)
	staleWarnDays := fs.Int("stale-warn-days", defaultStaleWarn, "Warn if any session-operator-* ticket is older than N days")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	checks := []healthCheck{}
	// 1. /healthz reachable
	client := &http.Client{Timeout: 5 * time.Second}
	healthzURL := strings.TrimRight(cf.baseURL, "/") + "/healthz"
	resp, err := client.Get(healthzURL)
	if err != nil {
		checks = append(checks, healthCheck{Name: "http-healthz", OK: false, Detail: err.Error()})
	} else {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		ok := resp.StatusCode == http.StatusOK
		checks = append(checks, healthCheck{Name: "http-healthz", OK: ok, Detail: fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body))})
	}

	// 2. local store reachable
	store, closer, err := openStore(cf)
	if err != nil {
		checks = append(checks, healthCheck{Name: "store-ping", OK: false, Detail: err.Error()})
	} else {
		defer closer()
		if err := store.Ping(); err != nil {
			checks = append(checks, healthCheck{Name: "store-ping", OK: false, Detail: err.Error()})
		} else {
			checks = append(checks, healthCheck{Name: "store-ping", OK: true, Detail: fmt.Sprintf("dialect=%s", store.Dialect())})
		}

		// 3. ticket counts by status
		tickets, err := store.ListTickets("")
		if err == nil {
			byStatus := map[string]int{}
			oldestStaleOp := time.Time{}
			staleCount := 0
			cutoff := time.Now().AddDate(0, 0, -*staleWarnDays)
			for _, t := range tickets {
				byStatus[string(t.Status)]++
				if !strings.HasPrefix(t.ID, "session-operator-") {
					continue
				}
				if t.CreatedAt.IsZero() || t.CreatedAt.Before(cutoff) {
					staleCount++
					if t.CreatedAt.IsZero() || t.CreatedAt.Before(oldestStaleOp) {
						oldestStaleOp = t.CreatedAt
					}
				}
			}
			checks = append(checks, healthCheck{
				Name:   "ticket-count",
				OK:     len(tickets) > 0,
				Detail: fmt.Sprintf("%d total: %v", len(tickets), byStatus),
			})
			checks = append(checks, healthCheck{
				Name:   "stale-session-operator",
				OK:     staleCount == 0,
				Detail: fmt.Sprintf("%d session-operator-* tickets older than %dd (oldest=%s)", staleCount, *staleWarnDays, oldestStaleOp.Format(time.RFC3339)),
			})
		}
	}

	allOK := true
	for _, c := range checks {
		if !c.OK {
			allOK = false
		}
	}
	out := map[string]interface{}{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"subcommand": "health",
		"base":       cf.baseURL,
		"checks":     checks,
		"ok":         allOK,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	if !allOK {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func runList(cf *commonFlags, args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	sprint := fs.String("sprint", "", "Filter to a specific sprint ID")
	status := fs.String("status", "", "Filter to a specific ticket status")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	store, closer, err := openStore(cf)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer closer()

	tickets, err := store.ListTickets(*sprint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list tickets: %v\n", err)
		return 1
	}
	filtered := make([]sprintboard.Ticket, 0, len(tickets))
	for _, t := range tickets {
		if *status != "" && string(t.Status) != *status {
			continue
		}
		filtered = append(filtered, t)
	}
	out := map[string]interface{}{
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"subcommand": "list",
		"count":      len(filtered),
		"tickets":    filtered,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	return 0
}

// ---------------------------------------------------------------------------
// misc
// ---------------------------------------------------------------------------

var _ = errors.New
var _ = sql.ErrNoRows
