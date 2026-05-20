package sprintboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedEmptyInput(t *testing.T) {
	e := NewEmbedder(EmbedderConfig{Dimension: 384})
	vec, err := e.Embed("")
	if err != nil {
		t.Fatalf("Embed empty: %v", err)
	}
	if len(vec) != 384 {
		t.Errorf("expected 384 dims, got %d", len(vec))
	}
	for _, v := range vec {
		if v != 0 {
			t.Error("zero vector expected for empty input")
			break
		}
	}
}

func TestEmbedTFIDFFallback(t *testing.T) {
	e := NewEmbedder(EmbedderConfig{Dimension: 128})
	vec, err := e.Embed("hello world testing")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vec) != 128 {
		t.Errorf("expected 128 dims, got %d", len(vec))
	}

	var nonZero int
	for _, v := range vec {
		if v != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Error("expected non-zero vector for non-empty input")
	}
}

func TestEmbedTFIDFNormalized(t *testing.T) {
	e := NewEmbedder(EmbedderConfig{Dimension: 64})
	vec, err := e.Embed("the quick brown fox jumps over the lazy dog")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	var normSq float64
	for _, v := range vec {
		normSq += float64(v) * float64(v)
	}
	if normSq < 0.99 || normSq > 1.01 {
		t.Errorf("expected unit norm, got %f", normSq)
	}
}

func TestEmbedSimilarTexts(t *testing.T) {
	e := NewEmbedder(EmbedderConfig{Dimension: 256})
	vec1, _ := e.Embed("implement the sprint board MCP server")
	vec2, _ := e.Embed("build the sprint board MCP binary")
	vec3, _ := e.Embed("fix the database connection timeout")

	sim12 := cosineSimilarity(vec1, vec2)
	sim13 := cosineSimilarity(vec1, vec3)

	if sim12 <= sim13 {
		t.Errorf("similar texts should score higher: sim(1,2)=%f, sim(1,3)=%f", sim12, sim13)
	}
}

func TestEmbedViaHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.Error(w, "not found", 404)
			return
		}
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{"embedding": []float32{0.1, 0.2, 0.3, 0.4}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	e := NewEmbedder(EmbedderConfig{EmbeddingURL: server.URL, Dimension: 4})
	vec, err := e.Embed("test input")
	if err != nil {
		t.Fatalf("Embed via HTTP: %v", err)
	}
	if len(vec) != 4 {
		t.Errorf("expected 4 dims from mock, got %d", len(vec))
	}
	if vec[0] != 0.1 || vec[3] != 0.4 {
		t.Errorf("unexpected values: %v", vec)
	}
}

func TestEmbedHTTPFallsBackToTFIDF(t *testing.T) {
	e := NewEmbedder(EmbedderConfig{EmbeddingURL: "http://localhost:19999", Dimension: 64})
	vec, err := e.Embed("fallback test")
	if err != nil {
		t.Fatalf("should not error on fallback: %v", err)
	}
	if len(vec) != 64 {
		t.Errorf("expected 64 dims, got %d", len(vec))
	}
	var nonZero int
	for _, v := range vec {
		if v != 0 {
			nonZero++
		}
	}
	if nonZero == 0 {
		t.Error("TF-IDF fallback should produce non-zero vector")
	}
}

func TestEmbedHTTPBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", 500)
	}))
	defer server.Close()

	e := NewEmbedder(EmbedderConfig{EmbeddingURL: server.URL, Dimension: 64})
	vec, err := e.Embed("test")
	if err != nil {
		t.Fatalf("should fallback, not error: %v", err)
	}
	if len(vec) != 64 {
		t.Errorf("expected fallback 64 dims, got %d", len(vec))
	}
}

func TestDefaultEmbedderConfig(t *testing.T) {
	cfg := DefaultEmbedderConfig()
	if cfg.Dimension != 384 {
		t.Errorf("expected 384, got %d", cfg.Dimension)
	}
}
