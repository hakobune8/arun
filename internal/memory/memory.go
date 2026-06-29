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

// Package memory provides pluggable memory stores for agents with
// support for vector, JSON, and SQLite backends.
package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/kazyamaz200/agentos/internal/embedding"
	"github.com/kazyamaz200/agentos/internal/vector"
)

// Entry represents a single memory entry with content, type, and metadata.
type Entry struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Content   string                 `json:"content"`
	Type      string                 `json:"type"`
	Metadata  map[string]interface{} `json:"metadata"`
	Vector    []float32              `json:"-"`
}

// Config defines which memory backend to use and its options.
type Config struct {
	// Backend selects the implementation: "vector", "json", "sqlite".
	// Defaults to "json".
	Backend string `yaml:"backend"`

	// Path is the file path for JSON or SQLite backends.
	Path string `yaml:"path"`

	// Collection is the vector collection name (for vector backend).
	Collection string `yaml:"collection"`
}

// DefaultConfig returns a default memory configuration using JSON backend.
func DefaultConfig() Config {
	return Config{
		Backend: "json",
		Path:    ".agentos/memory.jsonl",
	}
}

// New creates a Store from the given config and optional dependencies.
// For the vector backend, vs and embed must be provided.
func New(ctx context.Context, cfg Config, vs vector.VectorStore, embed embedding.Embedder) (Store, error) {
	switch cfg.Backend {
	case "vector":
		if vs == nil {
			return nil, fmt.Errorf("vector store required for vector memory backend")
		}
		if embed == nil {
			return nil, fmt.Errorf("embedder required for vector memory backend")
		}
		s := NewVectorStore(vs, embed)
		if cfg.Collection != "" {
			s.collection = cfg.Collection
		}
		return s, nil

	case "json":
		path := cfg.Path
		if path == "" {
			path = ".agentos/memory.jsonl"
		}
		return NewJSONStore(path)

	case "sqlite":
		return nil, fmt.Errorf("sqlite memory backend not yet implemented")

	default:
		return nil, fmt.Errorf("unknown memory backend: %q (options: vector, json, sqlite)", cfg.Backend)
	}
}

// MemoryStore is a type alias for backward compatibility.
// Deprecated: Use Store interface instead.
//nolint:revive // stutter is acceptable for backward compatibility
type MemoryStore = VectorStore

// NewMemoryStore creates a VectorStore.
// Deprecated: Use New() with Config instead.
func NewMemoryStore(vs vector.VectorStore, embed embedding.Embedder) *VectorStore {
	return NewVectorStore(vs, embed)
}
