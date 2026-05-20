package sprintboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

type EmbedderConfig struct {
	EmbeddingURL string
	Dimension    int
	Timeout      time.Duration
}

func DefaultEmbedderConfig() EmbedderConfig {
	return EmbedderConfig{
		EmbeddingURL: envStr("EMBEDDING_URL", ""),
		Dimension:    384,
		Timeout:      10 * time.Second,
	}
}

type Embedder struct {
	config EmbedderConfig
	client *http.Client
}

func NewEmbedder(cfg EmbedderConfig) *Embedder {
	if cfg.Dimension <= 0 {
		cfg.Dimension = 384
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &Embedder{
		config: cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

func (e *Embedder) Embed(text string) ([]float32, error) {
	if text == "" {
		return make([]float32, e.config.Dimension), nil
	}

	if e.config.EmbeddingURL != "" {
		vec, err := e.embedViaHTTP(text)
		if err == nil {
			return vec, nil
		}
	}

	return e.embedTFIDF(text), nil
}

func (e *Embedder) embedViaHTTP(text string) ([]float32, error) {
	payload := map[string]interface{}{
		"input": text,
		"model": "text-embedding",
	}
	body, _ := json.Marshal(payload)

	resp, err := e.client.Post(e.config.EmbeddingURL+"/v1/embeddings", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding endpoint returned %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	return result.Data[0].Embedding, nil
}

func (e *Embedder) embedTFIDF(text string) []float32 {
	vec := make([]float32, e.config.Dimension)
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return vec
	}

	for _, word := range words {
		idx := hashWord(word) % uint32(e.config.Dimension)
		vec[idx] += 1.0
	}

	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] = float32(float64(vec[i]) / norm)
		}
	}

	return vec
}

func hashWord(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
