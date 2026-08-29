// Package ndjson is the canonical, race-safe append-only NDJSON writer for
// sprintboard-mcp. Mirrors helixon-ec/internal/ndjson and
// helix-dev-tools/internal/ndjson so all three repos share an identical
// contract for new NDJSON sinks.
package ndjson

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Writer appends NDJSON events to a backing io.WriteCloser.
type Writer struct {
	mu   sync.Mutex
	w    io.WriteCloser
	path string
}

// Open creates (or appends to) an NDJSON log file under path. Empty path
// returns a nil Writer that silently no-ops Append/Close.
func Open(path string) (*Writer, error) {
	if path == "" {
		return nil, nil
	}
	// 0750/0600: these are per-service audit logs, readable only by the user
	// running the process. Matches Store.Open's 0700 on the database directory.
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("ndjson: mkdir %s: %w", filepath.Dir(path), err)
	}
	// #nosec G304 -- opening a caller-supplied path is this constructor's entire
	// purpose; the path comes from process configuration, never from request
	// data. Callers that accept untrusted input must validate before calling.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("ndjson: open %s: %w", path, err)
	}
	return &Writer{w: f, path: path}, nil
}

// NewWriter wraps an arbitrary io.WriteCloser. Useful in tests.
func NewWriter(wc io.WriteCloser) *Writer {
	if wc == nil {
		return nil
	}
	return &Writer{w: wc}
}

// Path returns the backing file path, or "" for an in-memory writer.
func (w *Writer) Path() string {
	if w == nil {
		return ""
	}
	return w.path
}

// Append marshals event and writes one NDJSON line. Safe on nil Writer.
func (w *Writer) Append(event any) error {
	if w == nil || w.w == nil {
		return nil
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("ndjson: marshal: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.w.Write(data); err != nil {
		return fmt.Errorf("ndjson: write payload: %w", err)
	}
	if _, err := w.w.Write([]byte("\n")); err != nil {
		return fmt.Errorf("ndjson: write newline: %w", err)
	}
	return nil
}

// AppendOne is a convenience for one-shot appends.
func AppendOne(path string, event any) error {
	w, err := Open(path)
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	// Close is reported, not discarded. This writer backs an append-only audit
	// log; a Close that fails (ENOSPC, I/O error) can lose the very line just
	// appended, and returning nil there would tell the caller the event was
	// durably recorded when it was not. Append's error still wins, since it is
	// the more specific failure.
	appendErr := w.Append(event)
	closeErr := w.Close()
	if appendErr != nil {
		return appendErr
	}
	if closeErr != nil {
		return fmt.Errorf("ndjson: close %s: %w", path, closeErr)
	}
	return nil
}

// Close releases the underlying writer. Safe on nil and idempotent.
func (w *Writer) Close() error {
	if w == nil || w.w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.w.Close()
	w.w = nil
	return err
}
