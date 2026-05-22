package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// Notifier dispatches sprintboard lifecycle events to an external endpoint
// (e.g. a Slack relay, helixon control plane, or test sink). Notify must be
// safe to call from any goroutine and must never block the dispatch path
// for longer than its configured timeout.
type Notifier interface {
	Notify(event string, payload map[string]any)
}

// nopNotifier is the default. It satisfies Notifier with no side effects so
// the Server can call Notify unconditionally.
type nopNotifier struct{}

func (nopNotifier) Notify(string, map[string]any) {}

// httpNotifier POSTs a JSON envelope to a single URL. Failures are logged
// to stderr and never returned; the dispatch caller must not be coupled to
// downstream availability.
type httpNotifier struct {
	url     string
	client  *http.Client
	timeout time.Duration
}

// NewHTTPNotifier returns a Notifier that POSTs every event to url.
// timeout caps each HTTP call; zero falls back to a 2s default.
func NewHTTPNotifier(url string, timeout time.Duration) Notifier {
	if url == "" {
		return nopNotifier{}
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &httpNotifier{
		url:     url,
		client:  &http.Client{Timeout: timeout},
		timeout: timeout,
	}
}

// Notify serialises event + payload and POSTs them. The send happens on a
// detached goroutine so the dispatch caller is never blocked by webhook
// latency or failure.
func (h *httpNotifier) Notify(event string, payload map[string]any) {
	envelope := map[string]any{
		"event":     event,
		"payload":   payload,
		"emitted_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	body, err := json.Marshal(envelope)
	if err != nil {
		return
	}
	go h.post(body)
}

func (h *httpNotifier) post(body []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}

// resolveNotifier returns the Notifier configured via env (`SPRINTBOARD_WEBHOOK_URL`).
// Empty env yields a no-op so existing deployments are unaffected.
func resolveNotifier() Notifier {
	url := os.Getenv("SPRINTBOARD_WEBHOOK_URL")
	return NewHTTPNotifier(url, 0)
}
