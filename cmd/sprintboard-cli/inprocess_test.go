package main

// In-process subcommand tests.
//
// main_test.go covers the same subcommands by building the binary and running
// it with os/exec. That proves the wiring end to end, but a subprocess is not
// instrumented, so the whole package reported 0.0% coverage while being well
// tested. These tests call the run* functions directly so the coverage profile
// reflects what is actually exercised, and so a failure points at a line
// rather than at an exit code.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

// testFlags returns commonFlags pointed at an isolated SQLite file.
// SPRINTBOARD_DB_URL is cleared so openStore cannot reach a real Postgres.
func testFlags(t *testing.T) *commonFlags {
	t.Helper()
	t.Setenv("SPRINTBOARD_DB_URL", "")
	return &commonFlags{
		dbPath:  filepath.Join(t.TempDir(), "cli.db"),
		baseURL: defaultBaseURL,
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what
// was written. The run* functions print their results, so this is the only way
// to assert on them without changing their signatures.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	out := <-done
	_ = r.Close()
	return out
}

func TestSplitCommon_ConsumesGlobalFlagsBeforeSubcommandOnly(t *testing.T) {
	cases := []struct {
		name     string
		argv     []string
		wantSub  string
		wantDB   string
		wantJSON bool
	}{
		{"flags before subcommand", []string{"-db", "/tmp/a.db", "-json", "list"}, "list", "/tmp/a.db", true},
		// Global flags are consumed only BEFORE the subcommand: splitCommon
		// returns at the first non-common arg and hands everything after it to
		// the subcommand's own FlagSet. `-db` here therefore belongs to `list`,
		// not to commonFlags. Pinning the real contract so a future "fix" that
		// changes it has to change this test deliberately.
		{"flags after subcommand stay in the tail", []string{"list", "-db", "/tmp/b.db"}, "list", "", false},
		{"no flags", []string{"seed"}, "seed", "", false},
		{"empty argv", nil, "", "", false},
		{"verbose only", []string{"-v", "health"}, "health", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cf := &commonFlags{}
			_, sub, _ := splitCommon(cf, tc.argv)
			if sub != tc.wantSub {
				t.Errorf("sub = %q, want %q", sub, tc.wantSub)
			}
			if cf.dbPath != tc.wantDB {
				t.Errorf("dbPath = %q, want %q", cf.dbPath, tc.wantDB)
			}
			if cf.jsonOut != tc.wantJSON {
				t.Errorf("jsonOut = %v, want %v", cf.jsonOut, tc.wantJSON)
			}
		})
	}
}

func TestDefaultSeedTickets_AreWellFormed(t *testing.T) {
	seeds := defaultSeedTickets()
	if len(seeds) == 0 {
		t.Fatal("no default seed tickets")
	}
	seen := map[string]bool{}
	for _, s := range seeds {
		if s.ID == "" || s.Title == "" {
			t.Errorf("seed ticket missing id or title: %+v", s)
		}
		if seen[s.ID] {
			t.Errorf("duplicate seed id %q -- seeding would not be idempotent", s.ID)
		}
		seen[s.ID] = true
	}
}

func TestRunSeed_IsIdempotent(t *testing.T) {
	cf := testFlags(t)

	if rc := runSeed(cf, nil); rc != 0 {
		t.Fatalf("first seed rc = %d, want 0", rc)
	}

	store, closer, err := openStore(cf)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	first, err := store.ListTickets("")
	if err != nil {
		t.Fatalf("ListTickets: %v", err)
	}
	_ = closer()
	if len(first) == 0 {
		t.Fatal("seed inserted nothing")
	}

	// Positive control on idempotency: a second seed must not duplicate rows.
	if rc := runSeed(cf, nil); rc != 0 {
		t.Fatalf("second seed rc = %d, want 0", rc)
	}
	store2, closer2, err := openStore(cf)
	if err != nil {
		t.Fatalf("openStore 2: %v", err)
	}
	second, err := store2.ListTickets("")
	if err != nil {
		t.Fatalf("ListTickets 2: %v", err)
	}
	_ = closer2()
	if len(second) != len(first) {
		t.Errorf("ticket count %d -> %d across two seeds; seeding is not idempotent",
			len(first), len(second))
	}
}

func TestRunSeed_OnlyNewFlag(t *testing.T) {
	cf := testFlags(t)
	if rc := runSeed(cf, []string{"-only-new"}); rc != 0 {
		t.Fatalf("seed --only-new rc = %d, want 0", rc)
	}
}

func TestRunList_FiltersAndJSON(t *testing.T) {
	cf := testFlags(t)
	if rc := runSeed(cf, nil); rc != 0 {
		t.Fatalf("seed rc = %d", rc)
	}

	if rc := runList(cf, nil); rc != 0 {
		t.Errorf("list rc = %d, want 0", rc)
	}

	// A sprint filter that matches nothing must still succeed, not error.
	if rc := runList(cf, []string{"-sprint", "no-such-sprint"}); rc != 0 {
		t.Errorf("list with unmatched sprint rc = %d, want 0", rc)
	}

	// A status filter that matches nothing likewise.
	if rc := runList(cf, []string{"-status", "no-such-status"}); rc != 0 {
		t.Errorf("list with unmatched status rc = %d, want 0", rc)
	}

	cf.jsonOut = true
	out := captureStdout(t, func() {
		if rc := runList(cf, nil); rc != 0 {
			t.Errorf("list -json rc = %d, want 0", rc)
		}
	})
	if strings.TrimSpace(out) == "" {
		t.Fatal("list -json printed nothing")
	}
	var any interface{}
	if err := json.Unmarshal([]byte(out), &any); err != nil {
		t.Errorf("list -json emitted invalid JSON: %v\n%s", err, out)
	}
}

func TestRunList_RejectsBadFlag(t *testing.T) {
	cf := testFlags(t)
	if rc := runList(cf, []string{"-nonexistent-flag"}); rc == 0 {
		t.Error("list accepted an unknown flag; want a non-zero exit")
	}
}

func TestRunClean_DryRunLeavesRowsIntact(t *testing.T) {
	cf := testFlags(t)

	store, closer, err := openStore(cf)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	// A stale row matching the default `session-operator-*` pattern.
	if err := store.CreateTicket(sprintboard.Ticket{
		ID: "session-operator-stale", Title: "stale stub", Status: sprintboard.StatusBacklog,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	_ = closer()

	if rc := runClean(cf, []string{"-dry-run", "-stale-days", "0"}); rc != 0 {
		t.Fatalf("clean --dry-run rc = %d, want 0", rc)
	}

	// The whole point of --dry-run: the row must survive.
	store2, closer2, err := openStore(cf)
	if err != nil {
		t.Fatalf("openStore 2: %v", err)
	}
	if _, err := store2.GetTicket("session-operator-stale"); err != nil {
		t.Errorf("--dry-run deleted the row: %v", err)
	}
	_ = closer2()
}

func TestRunClean_RemovesStaleRows(t *testing.T) {
	cf := testFlags(t)

	store, closer, err := openStore(cf)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	if err := store.CreateTicket(sprintboard.Ticket{
		ID: "session-operator-gone", Title: "stale stub", Status: sprintboard.StatusBacklog,
	}); err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if err := store.CreateTicket(sprintboard.Ticket{
		ID: "keep-me", Title: "real work", Status: sprintboard.StatusBacklog,
	}); err != nil {
		t.Fatalf("CreateTicket keep: %v", err)
	}
	_ = closer()

	if rc := runClean(cf, []string{"-stale-days", "0"}); rc != 0 {
		t.Fatalf("clean rc = %d, want 0", rc)
	}

	store2, closer2, err := openStore(cf)
	if err != nil {
		t.Fatalf("openStore 2: %v", err)
	}
	defer func() { _ = closer2() }()
	if _, err := store2.GetTicket("session-operator-gone"); err == nil {
		t.Error("stale row survived clean")
	}
	// Positive control: clean must not be a blanket delete.
	if _, err := store2.GetTicket("keep-me"); err != nil {
		t.Errorf("clean removed a non-matching ticket: %v", err)
	}
}

func TestRunHealth_ReachableAndUnreachable(t *testing.T) {
	cf := testFlags(t)
	if rc := runSeed(cf, nil); rc != 0 {
		t.Fatalf("seed rc = %d", rc)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	cf.baseURL = ts.URL
	if rc := runHealth(cf, nil); rc != 0 {
		t.Errorf("health against a healthy endpoint rc = %d, want 0", rc)
	}

	cf.jsonOut = true
	out := captureStdout(t, func() { _ = runHealth(cf, nil) })
	if strings.TrimSpace(out) != "" {
		var any interface{}
		if err := json.Unmarshal([]byte(out), &any); err != nil {
			t.Errorf("health -json emitted invalid JSON: %v\n%s", err, out)
		}
	}

	// An unreachable board must be reported, not silently treated as healthy.
	cf.jsonOut = false
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	downURL := down.URL
	down.Close() // closed: connection refused
	cf.baseURL = downURL
	if rc := runHealth(cf, nil); rc == 0 {
		t.Error("health reported success against an unreachable board")
	}
}

func TestOpenStore_ReportsBadPath(t *testing.T) {
	t.Setenv("SPRINTBOARD_DB_URL", "")
	// A path whose parent is a FILE cannot be created as a directory.
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	cf := &commonFlags{dbPath: filepath.Join(f, "nested", "cli.db")}
	if _, closer, err := openStore(cf); err == nil {
		_ = closer()
		t.Error("openStore accepted an uncreatable path")
	}
}

func TestLogf_VerboseOnly(t *testing.T) {
	// logf writes to stderr only when -v is set; exercising both branches.
	logf(&commonFlags{verbose: false}, "quiet %d", 1)
	logf(&commonFlags{verbose: true}, "loud %d", 2)
}
