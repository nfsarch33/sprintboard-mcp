// Test for v17200 Story 1: SprintBoard CLOSE_WAIT regression and SQLite
// pool resilience.
//
// Production failure mode (observed 2026-05-27): under sustained heartbeat
// traffic from helixon-agent, sprintboard-api accumulated CLOSE_WAITs
// (3-11 sockets) and eventually wedged the listener; restart cleared it.
//
// Diagnosis: handlers decoded the request body via json.NewDecoder but
// never drained or closed it, so net/http could not return the connection
// to the keep-alive pool. Each heartbeat opened a fresh socket; clients
// then FIN'd, leaving the server-side half-closed.
//
// First fix attempt (rejected): wrap mux in http.TimeoutHandler. Goroutine
// dump under load showed every request blocking in database/sql.(*DB).conn
// because TimeoutHandler runs the inner handler in a detached goroutine; if
// the timer fires while that goroutine still holds the single SQLite
// connection (SetMaxOpenConns(1)), TimeoutHandler returns 503 to the
// client but the inner goroutine keeps the conn forever, cascading into
// permanent queue starvation.
//
// Final fix: drainAndClose every body-decoding handler. Per-request bound
// is delegated to the helixon-agent client (http.Client.Timeout=10s) and
// SQLite busy_timeout=5000.

package api_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptrace"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAgentRegister_NoConnLeak fans out 100 sequential POSTs to /api/v1/agents
// (the helixon-agent heartbeat path) and asserts the keep-alive pool reuses
// the same TCP connection. Without drainAndClose, each request would force
// a fresh dial because the body is never drained.
func TestAgentRegister_NoConnLeak(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	tr := &http.Transport{
		MaxIdleConns:        4,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}

	var fresh, reused int64
	trace := &httptrace.ClientTrace{
		GotConn: func(info httptrace.GotConnInfo) {
			if info.Reused {
				atomic.AddInt64(&reused, 1)
			} else {
				atomic.AddInt64(&fresh, 1)
			}
		},
	}

	const n = 100
	const bodyTmpl = `{"agent_id":"leak-test","surface":"helixon-agent","capabilities":"e2e"}`

	for i := 0; i < n; i++ {
		ctx := httptrace.WithClientTrace(context.Background(), trace)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, ts.URL+"/api/v1/agents", bytes.NewBufferString(bodyTmpl))
		if err != nil {
			t.Fatalf("NewRequest %d: %v", i, err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("POST %d: %v", i, err)
		}
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status[%d] = %d, want 201", i, resp.StatusCode)
		}
		_, _ = bytes.NewBuffer(nil).ReadFrom(resp.Body)
		resp.Body.Close()
	}

	gotFresh := atomic.LoadInt64(&fresh)
	gotReused := atomic.LoadInt64(&reused)
	if gotFresh > 2 {
		t.Fatalf("connection leak: fresh=%d reused=%d (want fresh<=2 for n=%d POSTs)", gotFresh, gotReused, n)
	}
	if gotReused < int64(n-2) {
		t.Fatalf("keep-alive broken: fresh=%d reused=%d (want reused>=%d)", gotFresh, gotReused, n-2)
	}
	t.Logf("fresh=%d reused=%d for %d POSTs", gotFresh, gotReused, n)
}

// TestConcurrentHeartbeatsDoNotDeadlock fires 16 concurrent POST /api/v1/agents
// at the same store. With SetMaxOpenConns(1) the SQLite writer is a queue,
// but every request must still complete; if any handler holds the conn past
// its return path (the v17200 TimeoutHandler regression), the pool starves
// and the test wedges past the deadline.
//
// Each request body is the same heartbeat shape helixon-agent sends; the
// handler under test is the same handleAgentRegister registered on the
// production mux.
func TestConcurrentHeartbeatsDoNotDeadlock(t *testing.T) {
	ts := setupTestServer(t)
	defer ts.Close()

	const concurrency = 16
	const perGoroutine = 8
	deadline := time.Now().Add(15 * time.Second)

	var wg sync.WaitGroup
	errCh := make(chan error, concurrency*perGoroutine)
	for g := 0; g < concurrency; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			client := &http.Client{Timeout: 5 * time.Second}
			for i := 0; i < perGoroutine; i++ {
				if time.Now().After(deadline) {
					errCh <- context.DeadlineExceeded
					return
				}
				body := bytes.NewBufferString(`{"agent_id":"hb-` +
					itoa(gid) + `-` + itoa(i) +
					`","surface":"helixon-agent","capabilities":"hb"}`)
				resp, err := client.Post(ts.URL+"/api/v1/agents", "application/json", body)
				if err != nil {
					errCh <- err
					return
				}
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusCreated {
					errCh <- &httpStatusErr{want: 201, got: resp.StatusCode}
					return
				}
			}
		}(g)
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatalf("test wedged past 20s -- SQLite pool likely starved")
	}
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent heartbeat error: %v", err)
		}
	}
}

type httpStatusErr struct{ want, got int }

func (e *httpStatusErr) Error() string { return "want " + itoa(e.want) + " got " + itoa(e.got) }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
