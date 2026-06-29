// Copyright 2026 AgentOS Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/kazyamaz200/agentos/internal/embedding"
	"github.com/kazyamaz200/agentos/internal/vector"
)

// VectorStore implements Store using a vector database backend.
type VectorStore struct {
	vs         vector.VectorStore
	embed      embedding.Embedder
	collection string
}

// NewVectorStore creates a VectorStore backed by the given vector store and embedder.
func NewVectorStore(vs vector.VectorStore, embed embedding.Embedder) *VectorStore {
	return &VectorStore{
		vs:         vs,
		embed:      embed,
		collection: "agentos_memory",
	}
}

// Type returns "vector" as the backend identifier.
func (m *VectorStore) Type() string { return "vector" }

// Save stores a memory entry, embedding its content for later search.
func (m *VectorStore) Save(ctx context.Context, entry *Entry) error {
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
		ID:     entry.ID,
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

// Search finds memory entries similar to the given query string.
func (m *VectorStore) Search(ctx context.Context, query string, limit int) ([]Entry, error) {
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
			ID:     p.ID,
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

// Clear removes all stored memory entries.
func (m *VectorStore) Clear(ctx context.Context) error {
	return m.vs.DeleteCollection(ctx, m.collection)
}
