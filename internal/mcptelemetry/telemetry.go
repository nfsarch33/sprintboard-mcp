package mcptelemetry

import (
	"os"
	"path/filepath"
	"time"

	"github.com/nfsarch33/sprintboard-mcp/internal/ndjson"
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
	config Config
	w      *ndjson.Writer
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
	w, err := ndjson.Open(cfg.LogPath)
	if err != nil {
		return nil, err
	}
	return &Recorder{config: cfg, w: w}, nil
}

func (r *Recorder) Record(tool string, agentID string, duration time.Duration, isError bool, errMsg string) {
	if r == nil || !r.config.Enabled || r.w == nil {
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
	_ = r.w.Append(event)
}

func (r *Recorder) Close() error {
	if r == nil {
		return nil
	}
	return r.w.Close()
}

func (r *Recorder) Enabled() bool {
	if r == nil {
		return false
	}
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
