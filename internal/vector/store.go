// Package vector provides vector storage backends for embeddings.
package vector

import "context"

// Point represents a single vector point with an ID, vector data, payload, and optional score.
type Point struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
	Score   float64                `json:"score,omitempty"`
}

// SearchResult contains the results of a vector search.
type SearchResult struct {
	Points []Point `json:"points"`
}

// VectorStore defines the interface for vector database operations.
type VectorStore interface {
	Name() string
	Upsert(ctx context.Context, collection string, points []Point) error
	Search(ctx context.Context, collection string, vector []float32, limit int) ([]Point, error)
	DeleteCollection(ctx context.Context, collection string) error
}
