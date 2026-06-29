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

// Package search provides a unified search service across memories, guidelines, and PRs.
package search

import (
	"context"
	"fmt"

	"github.com/kazyamaz200/agentos/internal/embedding"
	"github.com/kazyamaz200/agentos/internal/guideline"
	"github.com/kazyamaz200/agentos/internal/memory"
	"github.com/kazyamaz200/agentos/internal/vector"
)

// Type represents a search source type.
type Type string

const (
	// TypeMemory searches across agent memory entries.
	TypeMemory Type = "memory"
	// TypeGuideline searches across guideline entries.
	TypeGuideline Type = "guideline"
	// TypePR searches across pull request memories.
	TypePR Type = "pr"
	// TypeAll searches across all available sources.
	TypeAll Type = "all"
)

// Service provides unified search across multiple data sources.
type Service struct {
	memStore *memory.VectorStore
	glStore  *guideline.Store
	vs       vector.VectorStore
	embedder embedding.Embedder
}

// NewService creates a new search service with the given vector store and embedder.
func NewService(vs vector.VectorStore, embedder embedding.Embedder) *Service {
	return &Service{
		memStore: memory.NewVectorStore(vs, embedder),
		glStore:  guideline.NewStore(vs, embedder),
		vs:       vs,
		embedder: embedder,
	}
}

// Result represents a single search result from any source.
type Result struct {
	Source   Type                   `json:"source"`
	ID       string                 `json:"id"`
	Content  string                 `json:"content"`
	Score    float64                `json:"score"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Search performs a search across the specified source type.
func (s *Service) Search(ctx context.Context, query string, searchType Type, limit int) ([]Result, error) {
	if limit <= 0 {
		limit = 10
	}
	switch searchType {
	case TypeMemory:
		return s.searchMemory(ctx, query, limit)
	case TypeGuideline:
		return s.searchGuidelines(ctx, query, limit)
	case TypePR:
		return s.searchPRs(ctx, query, limit)
	case TypeAll:
		return s.searchAll(ctx, query, limit)
	default:
		return nil, fmt.Errorf("unknown search type: %s", searchType)
	}
}

func (s *Service) searchMemory(ctx context.Context, query string, limit int) ([]Result, error) {
	entries, err := s.memStore.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, e := range entries {
		results = append(results, Result{
			Source:   TypeMemory,
			ID:       e.ID,
			Content:  e.Content,
			Metadata: e.Metadata,
		})
	}
	return results, nil
}

func (s *Service) searchGuidelines(ctx context.Context, query string, limit int) ([]Result, error) {
	gls, err := s.glStore.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, g := range gls {
		results = append(results, Result{
			Source:  TypeGuideline,
			ID:      g.ID,
			Content: g.Title + ": " + g.Rule,
			Metadata: map[string]interface{}{
				"title":   g.Title,
				"rule":    g.Rule,
				"tags":    g.Tags,
				"example": g.Example,
			},
		})
	}
	return results, nil
}

func (s *Service) searchPRs(ctx context.Context, query string, limit int) ([]Result, error) {
	entries, err := s.memStore.Search(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	var results []Result
	for _, e := range entries {
		if e.Type == "pr" {
			results = append(results, Result{
				Source:   TypePR,
				ID:       e.ID,
				Content:  e.Content,
				Metadata: e.Metadata,
			})
		}
	}
	return results, nil
}

func (s *Service) searchAll(ctx context.Context, query string, limit int) ([]Result, error) {
	var all []Result
	memResults, _ := s.searchMemory(ctx, query, limit)
	all = append(all, memResults...)
	glResults, _ := s.searchGuidelines(ctx, query, limit)
	all = append(all, glResults...)
	prResults, _ := s.searchPRs(ctx, query, limit)
	all = append(all, prResults...)
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// MemoryStore returns the underlying memory store.
func (s *Service) MemoryStore() *memory.VectorStore {
	return s.memStore
}

// GuidelineStore returns the underlying guideline store.
func (s *Service) GuidelineStore() *guideline.Store {
	return s.glStore
}
