package sprintboard

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"
)

type VectorEntry struct {
	SourceType string    `json:"source_type"`
	SourceID   string    `json:"source_id"`
	Embedding  []float32 `json:"embedding"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SearchResult struct {
	SourceType string  `json:"source_type"`
	SourceID   string  `json:"source_id"`
	Score      float64 `json:"score"`
}

func (s *Store) migrateVectors() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS embeddings (
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			embedding BLOB NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (source_type, source_id)
		);
		CREATE INDEX IF NOT EXISTS idx_embeddings_type ON embeddings(source_type);
	`)
	return err
}

func (s *Store) StoreEmbedding(sourceType, sourceID string, embedding []float32) error {
	blob, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}

	_, err = s.db.Exec(
		`INSERT OR REPLACE INTO embeddings (source_type, source_id, embedding, updated_at)
		 VALUES (?, ?, ?, ?)`,
		sourceType, sourceID, blob, formatTime(time.Now()),
	)
	return err
}

func (s *Store) SearchSimilar(queryVec []float32, sourceType string, limit int) ([]SearchResult, error) {
	query := `SELECT source_type, source_id, embedding FROM embeddings`
	var args []interface{}
	if sourceType != "" {
		query += ` WHERE source_type = ?`
		args = append(args, sourceType)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var sType, sID string
		var blob []byte
		if err := rows.Scan(&sType, &sID, &blob); err != nil {
			return nil, err
		}

		var stored []float32
		if err := json.Unmarshal(blob, &stored); err != nil {
			continue
		}

		score := cosineSimilarity(queryVec, stored)
		if score > 0 {
			results = append(results, SearchResult{
				SourceType: sType,
				SourceID:   sID,
				Score:      score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, rows.Err()
}

func (s *Store) DeleteEmbedding(sourceType, sourceID string) error {
	_, err := s.db.Exec(
		`DELETE FROM embeddings WHERE source_type = ? AND source_id = ?`,
		sourceType, sourceID,
	)
	return err
}

func (s *Store) EmbeddingCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&count)
	return count, err
}

func (s *Store) HasEmbedding(sourceType, sourceID string) bool {
	var count int
	s.db.QueryRow(
		`SELECT COUNT(*) FROM embeddings WHERE source_type = ? AND source_id = ?`,
		sourceType, sourceID,
	).Scan(&count)
	return count > 0
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}

