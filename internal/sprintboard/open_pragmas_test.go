package sprintboard

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Every connection the pool opens gets the pragmas, not only the first one. A pragma
// issued once with db.Exec configures one connection; its replacement ran with
// busy_timeout 0 and failed a write at once with "database is locked".
func TestOpen_EveryConnectionGetsThePragmas(t *testing.T) {
	s := testStore(t)
	raw := s.db.raw
	raw.SetMaxIdleConns(0) // no connection is reused: each query below opens a fresh one
	for i := 0; i < 3; i++ {
		var busy, syn int
		var mode string
		if err := raw.QueryRow("PRAGMA busy_timeout").Scan(&busy); err != nil {
			t.Fatalf("busy_timeout: %v", err)
		}
		if err := raw.QueryRow("PRAGMA synchronous").Scan(&syn); err != nil {
			t.Fatalf("synchronous: %v", err)
		}
		if err := raw.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatalf("journal_mode: %v", err)
		}
		if busy != 5000 || syn != 1 || mode != "wal" {
			t.Fatalf("fresh connection %d: busy_timeout=%d synchronous=%d journal_mode=%s; want 5000, 1 (NORMAL), wal", i, busy, syn, mode)
		}
	}
}

// A claim must not fail because another writer committed while it ran. A deferred
// transaction reads first and upgrades to a write later; in WAL mode that upgrade fails
// at once with SQLITE_BUSY when another connection committed in between, and no busy
// timeout retries it. The claim's transaction now takes the write lock up front.
func TestClaimTicket_SurvivesAConcurrentWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	for _, id := range []string{"T1", "T2"} {
		if err := s.CreateTicket(Ticket{ID: id, SprintID: "S1", Title: id, Status: StatusReady}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}

	other, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("second writer: %v", err)
	}
	t.Cleanup(func() { _ = other.Close() })
	ctx := context.Background()
	conn, err := other.Conn(ctx)
	if err != nil {
		t.Fatalf("second writer conn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatalf("second writer begin: %v", err)
	}
	if _, err := conn.ExecContext(ctx, "UPDATE tickets SET title = 'touched' WHERE id = 'T2'"); err != nil {
		t.Fatalf("second writer update: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(300 * time.Millisecond)
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			t.Errorf("second writer commit: %v", err)
		}
	}()

	res, err := s.ClaimTicket("T1", "agent-a")
	wg.Wait()
	if err != nil || !res.Success {
		t.Fatalf("claim while another writer commits: success=%v err=%v", res.Success, err)
	}
}
