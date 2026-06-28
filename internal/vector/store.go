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
//nolint:revive // stutter is acceptable for package-level interface
type VectorStore interface {
	Name() string
	Upsert(ctx context.Context, collection string, points []Point) error
	Search(ctx context.Context, collection string, vector []float32, limit int) ([]Point, error)
	DeleteCollection(ctx context.Context, collection string) error
}
