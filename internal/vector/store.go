package vector

import "context"

type Point struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
	Score   float64                `json:"score,omitempty"`
}

type SearchResult struct {
	Points []Point `json:"points"`
}

type VectorStore interface {
	Name() string
	Upsert(ctx context.Context, collection string, points []Point) error
	Search(ctx context.Context, collection string, vector []float32, limit int) ([]Point, error)
	DeleteCollection(ctx context.Context, collection string) error
}
