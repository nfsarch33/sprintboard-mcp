package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	"github.com/nfsarch33/sprintboard-mcp/internal/sprintboard"
)

func testStore(t *testing.T) *sprintboard.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := sprintboard.NewStore(dbPath)
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	return store
}

func testServer(t *testing.T) *Server {
	t.Helper()
	store := testStore(t)
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	return NewServer(store, logger)
}

func TestHandleHealthz_OK(t *testing.T) {
	t.Parallel()
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", resp["status"])
	}
}

func TestHandleReadyz_OK(t *testing.T) {
	t.Parallel()
	srv := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %s", resp["status"])
	}
}

func TestHandleReadyz_ShuttingDown(t *testing.T) {
	t.Parallel()
	srv := testServer(t)
	srv.SetShuttingDown()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["status"] != "shutting_down" {
		t.Errorf("expected status=shutting_down, got %s", resp["status"])
	}
}

func TestStorePing(t *testing.T) {
	t.Parallel()
	store := testStore(t)
	if err := store.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}
}

// TestHandleHealthz_RepeatedCallsNoLeak (v14597, closes CF-v14590-01)
// proves that calling /healthz repeatedly does NOT leak the SQLite
// connection. The previous implementation used `s.db.QueryRow().Err()`
// which leaks the underlying *Rows when Scan() is not called, eventually
// hanging subsequent requests behind the single conn in SetMaxOpenConns(1).
func TestHandleHealthz_RepeatedCallsNoLeak(t *testing.T) {
	t.Parallel()
	srv := testServer(t)
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			srv.Handler().ServeHTTP(w, req)
			close(done)
		}()
		select {
		case <-done:
			if w.Code != http.StatusOK {
				t.Fatalf("iter %d: expected 200, got %d", i, w.Code)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("iter %d: /healthz hung (likely Ping() Rows leak)", i)
		}
	}
}
