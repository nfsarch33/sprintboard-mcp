package ndjson

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type nopCloser struct{ *bytes.Buffer }

func (nopCloser) Close() error { return nil }

func TestOpen_AppendRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "events.ndjson")
	w, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := w.Append(map[string]any{"ev": "ok"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	w.Close()
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), `"ev":"ok"`) {
		t.Fatalf("missing event: %q", string(body))
	}
}

func TestOpen_EmptyPathIsNop(t *testing.T) {
	t.Parallel()
	w, _ := Open("")
	if w != nil {
		t.Fatalf("nil expected")
	}
	if err := w.Append(map[string]any{}); err != nil {
		t.Errorf("nil Append: %v", err)
	}
}

func TestAppend_RaceSafe(t *testing.T) {
	t.Parallel()
	buf := &bytes.Buffer{}
	w := NewWriter(nopCloser{buf})
	const n, k = 16, 32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < k; j++ {
				if err := w.Append(map[string]any{"id": id, "j": j}); err != nil {
					t.Errorf("Append: %v", err)
				}
			}
		}(i)
	}
	wg.Wait()
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if want := n * k; len(lines) != want {
		t.Fatalf("lines = %d, want %d", len(lines), want)
	}
	for idx, line := range lines {
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("line %d not valid JSON: %v -- %q", idx, err, line)
		}
	}
}

func TestAppendOne_OneShot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "shot.ndjson")
	if err := AppendOne(path, map[string]any{"x": 1}); err != nil {
		t.Fatalf("AppendOne: %v", err)
	}
	if err := AppendOne(path, map[string]any{"x": 2}); err != nil {
		t.Fatalf("AppendOne: %v", err)
	}
	body, _ := os.ReadFile(path)
	if cnt := strings.Count(string(body), "\n"); cnt != 2 {
		t.Fatalf("newlines = %d, want 2", cnt)
	}
}
