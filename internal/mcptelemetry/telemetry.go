package mcptelemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type ToolEvent struct {
	Timestamp  string `json:"ts"`
	Tool       string `json:"tool"`
	AgentID    string `json:"agent_id"`
	DurationMS int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

type Config struct {
	Enabled     bool
	LogPath     string
	IncludeArgs bool
}

type Recorder struct {
	mu      sync.Mutex
	config  Config
	file    *os.File
	encoder *json.Encoder
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Enabled:     envBool("AGENTRACE_ENABLED", true),
		LogPath:     envString("AGENTRACE_LOG_PATH", filepath.Join(home, "logs", "runx", "agentrace-mcp.ndjson")),
		IncludeArgs: envBool("AGENTRACE_INCLUDE_ARGS", false),
	}
}

func New(cfg Config) (*Recorder, error) {
	if !cfg.Enabled {
		return &Recorder{config: cfg}, nil
	}

	dir := filepath.Dir(cfg.LogPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}

	f, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &Recorder{
		config:  cfg,
		file:    f,
		encoder: json.NewEncoder(f),
	}, nil
}

func (r *Recorder) Record(tool string, agentID string, duration time.Duration, isError bool, errMsg string) {
	if !r.config.Enabled || r.file == nil {
		return
	}

	event := ToolEvent{
		Timestamp:  time.Now().Format(time.RFC3339),
		Tool:       tool,
		AgentID:    agentID,
		DurationMS: duration.Milliseconds(),
		Success:    !isError,
	}
	if isError && errMsg != "" {
		event.Error = errMsg
	}

	r.mu.Lock()
	r.encoder.Encode(event)
	r.mu.Unlock()
}

func (r *Recorder) Close() error {
	if r.file != nil {
		return r.file.Close()
	}
	return nil
}

func (r *Recorder) Enabled() bool {
	return r.config.Enabled
}

func envBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	return v == "true" || v == "1" || v == "yes"
}

func envString(key string, defaultVal string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	return v
}
