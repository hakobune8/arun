package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/kazyamaz200/agentos/internal/embedding"
	"github.com/kazyamaz200/agentos/internal/vector"
)

type Entry struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Content   string                 `json:"content"`
	Type      string                 `json:"type"`
	Metadata  map[string]interface{} `json:"metadata"`
	Vector    []float32              `json:"-"`
}

type MemoryStore struct {
	vs      vector.VectorStore
	embed   embedding.Embedder
	collection string
}

func NewMemoryStore(vs vector.VectorStore, embed embedding.Embedder) *MemoryStore {
	return &MemoryStore{
		vs:         vs,
		embed:      embed,
		collection: "agentos_memory",
	}
}

func (m *MemoryStore) Save(ctx context.Context, entry Entry) error {
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	vectors, err := m.embed.Embed(ctx, []string{entry.Content})
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	point := vector.Point{
		ID: entry.ID,
		Vector: vectors[0],
		Payload: map[string]interface{}{
			"content":   entry.Content,
			"type":      entry.Type,
			"timestamp": entry.Timestamp.Format(time.RFC3339),
		},
	}
	for k, v := range entry.Metadata {
		point.Payload[k] = v
	}

	return m.vs.Upsert(ctx, m.collection, []vector.Point{point})
}

func (m *MemoryStore) Search(ctx context.Context, query string, limit int) ([]Entry, error) {
	vec, err := m.embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	points, err := m.vs.Search(ctx, m.collection, vec, limit)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	var entries []Entry
	for _, p := range points {
		entry := Entry{
			ID:    p.ID,
			Vector: p.Vector,
		}
		if content, ok := p.Payload["content"]; ok {
			entry.Content = fmt.Sprintf("%v", content)
		}
		if t, ok := p.Payload["type"]; ok {
			entry.Type = fmt.Sprintf("%v", t)
		}
		entry.Metadata = p.Payload
		entries = append(entries, entry)
	}

	return entries, nil
}

func (m *MemoryStore) Clear(ctx context.Context) error {
	return m.vs.DeleteCollection(ctx, m.collection)
}
