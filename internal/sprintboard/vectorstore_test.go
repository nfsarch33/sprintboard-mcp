package sprintboard

import (
	"fmt"
	"math"
	"testing"
)

func TestStoreAndSearchEmbedding(t *testing.T) {
	s := testStore(t)

	vec1 := []float32{1.0, 0.0, 0.0, 0.0}
	vec2 := []float32{0.0, 1.0, 0.0, 0.0}
	vec3 := []float32{0.9, 0.1, 0.0, 0.0}

	s.StoreEmbedding("ticket", "t-001", vec1)
	s.StoreEmbedding("ticket", "t-002", vec2)
	s.StoreEmbedding("ticket", "t-003", vec3)

	results, err := s.SearchSimilar(vec1, "ticket", 10)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].SourceID != "t-001" {
		t.Errorf("expected t-001 first (exact match), got %q", results[0].SourceID)
	}
	if results[1].SourceID != "t-003" {
		t.Errorf("expected t-003 second (high similarity), got %q", results[1].SourceID)
	}
}

func TestSearchBySourceType(t *testing.T) {
	s := testStore(t)

	vec := []float32{1.0, 0.5, 0.0}
	s.StoreEmbedding("ticket", "t-1", vec)
	s.StoreEmbedding("sprint", "s-1", vec)
	s.StoreEmbedding("handoff", "h-1", vec)

	results, _ := s.SearchSimilar(vec, "ticket", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 ticket result, got %d", len(results))
	}

	allResults, _ := s.SearchSimilar(vec, "", 10)
	if len(allResults) != 3 {
		t.Errorf("expected 3 results without filter, got %d", len(allResults))
	}
}

func TestSearchLimit(t *testing.T) {
	s := testStore(t)

	vec := []float32{1.0, 0.0}
	for i := 0; i < 20; i++ {
		s.StoreEmbedding("ticket", fmt.Sprintf("t-%d", i), vec)
	}

	results, _ := s.SearchSimilar(vec, "", 5)
	if len(results) != 5 {
		t.Errorf("expected 5 results with limit, got %d", len(results))
	}
}

func TestDeleteEmbedding(t *testing.T) {
	s := testStore(t)

	vec := []float32{1.0, 0.0}
	s.StoreEmbedding("ticket", "del-me", vec)

	s.DeleteEmbedding("ticket", "del-me")

	results, _ := s.SearchSimilar(vec, "", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(results))
	}
}

func TestEmbeddingCount(t *testing.T) {
	s := testStore(t)

	s.StoreEmbedding("ticket", "a", []float32{1, 0})
	s.StoreEmbedding("sprint", "b", []float32{0, 1})

	count, err := s.EmbeddingCount()
	if err != nil {
		t.Fatalf("EmbeddingCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

func TestHasEmbedding(t *testing.T) {
	s := testStore(t)

	s.StoreEmbedding("ticket", "exists", []float32{1, 0})

	if !s.HasEmbedding("ticket", "exists") {
		t.Error("should have embedding")
	}
	if s.HasEmbedding("ticket", "missing") {
		t.Error("should not have embedding")
	}
}

func TestUpsertEmbedding(t *testing.T) {
	s := testStore(t)

	s.StoreEmbedding("ticket", "upsert", []float32{1, 0, 0})
	s.StoreEmbedding("ticket", "upsert", []float32{0, 1, 0})

	count, _ := s.EmbeddingCount()
	if count != 1 {
		t.Errorf("expected 1 after upsert, got %d", count)
	}

	results, _ := s.SearchSimilar([]float32{0, 1, 0}, "ticket", 10)
	if len(results) != 1 || results[0].Score < 0.99 {
		t.Error("upsert should have updated to new vector")
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b []float32
		want float64
	}{
		{"identical", []float32{1, 0, 0}, []float32{1, 0, 0}, 1.0},
		{"orthogonal", []float32{1, 0, 0}, []float32{0, 1, 0}, 0.0},
		{"opposite", []float32{1, 0}, []float32{-1, 0}, -1.0},
		{"similar", []float32{1, 1}, []float32{1, 0.9}, 0.99},
		{"empty", []float32{}, []float32{}, 0.0},
		{"mismatch", []float32{1, 0}, []float32{1, 0, 0}, 0.0},
		{"zero", []float32{0, 0}, []float32{1, 0}, 0.0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cosineSimilarity(tc.a, tc.b)
			if tc.name == "similar" {
				if got < 0.99 {
					t.Errorf("expected ~0.99, got %f", got)
				}
				return
			}
			if math.Abs(got-tc.want) > 0.01 {
				t.Errorf("cosine(%v, %v) = %f, want %f", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestSearchSimilarEmptyDB(t *testing.T) {
	s := testStore(t)

	results, err := s.SearchSimilar([]float32{1, 0, 0}, "", 10)
	if err != nil {
		t.Fatalf("SearchSimilar on empty: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0, got %d", len(results))
	}
}
