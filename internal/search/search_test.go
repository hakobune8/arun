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

package search

import (
	"context"
	"testing"

	"github.com/kazyamaz200/agentos/internal/guideline"
	"github.com/kazyamaz200/agentos/internal/memory"
	"github.com/kazyamaz200/agentos/internal/vector"
)

type mockEmbedder struct {
	fixedVec []float32
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = m.fixedVec
	}
	return result, nil
}

func (m *mockEmbedder) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return m.fixedVec, nil
}

func (m *mockEmbedder) Model() string { return "test" }

func TestNewService_CreatesValidService(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	s := NewService(vs, emb)

	if s == nil {
		t.Fatal("NewService returned nil")
	}
	if s.MemoryStore() == nil {
		t.Error("MemoryStore() is nil")
	}
	if s.GuidelineStore() == nil {
		t.Error("GuidelineStore() is nil")
	}
}

func TestService_Search_TypeAll(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	svc := NewService(vs, emb)
	ctx := context.Background()

	svc.MemoryStore().Save(ctx, &memory.Entry{ID: "mem1", Content: "memory entry"})             //nolint:errcheck // test helper, error checked via search
	svc.GuidelineStore().Add(ctx, &guideline.Guideline{ID: "gl1", Title: "guideline", Rule: "do things"}) //nolint:errcheck // test helper, error checked via search

	results, err := svc.Search(ctx, "test", TypeAll, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results from TypeAll search")
	}
}

func TestService_Search_TypeGuideline(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	svc := NewService(vs, emb)
	ctx := context.Background()

	svc.GuidelineStore().Add(ctx, &guideline.Guideline{ID: "gl1", Title: "Test GL", Rule: "testing"}) //nolint:errcheck // test helper, error checked via search

	results, err := svc.Search(ctx, "test", TypeGuideline, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected guidelines from TypeGuideline search")
	}
	for _, r := range results {
		if r.Source != TypeGuideline {
			t.Errorf("result source = %q, want %q", r.Source, TypeGuideline)
		}
	}
}

func TestService_Search_TypeMemory(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	svc := NewService(vs, emb)
	ctx := context.Background()

	svc.MemoryStore().Save(ctx, &memory.Entry{ID: "mem1", Content: "my memory"})  //nolint:errcheck // test helper, error checked via search

	results, err := svc.Search(ctx, "memory", TypeMemory, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected memory results from TypeMemory search")
	}
	for _, r := range results {
		if r.Source != TypeMemory {
			t.Errorf("result source = %q, want %q", r.Source, TypeMemory)
		}
	}
}

func TestService_Search_TypePR(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	svc := NewService(vs, emb)
	ctx := context.Background()

	svc.MemoryStore().Save(ctx, &memory.Entry{ID: "pr1", Content: "PR content", Type: "pr"}) //nolint:errcheck // test helper, error checked via search

	results, err := svc.Search(ctx, "PR", TypePR, 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected PR results from TypePR search")
	}
	for _, r := range results {
		if r.Source != TypePR {
			t.Errorf("result source = %q, want %q", r.Source, TypePR)
		}
	}
}

func TestService_Search_UnknownTypeReturnsError(t *testing.T) {
	t.Parallel()

	svc := NewService(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	_, err := svc.Search(context.Background(), "test", "unknown_type", 10)
	if err == nil {
		t.Fatal("expected error for unknown search type")
	}
}

func TestService_Search_LimitZeroDefaultsToTen(t *testing.T) {
	t.Parallel()

	svc := NewService(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	_, err := svc.Search(context.Background(), "test", TypeMemory, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestService_Search_LimitNegativeDefaultsToTen(t *testing.T) {
	t.Parallel()

	svc := NewService(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	_, err := svc.Search(context.Background(), "test", TypeMemory, -5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestService_MemoryStore(t *testing.T) {
	t.Parallel()

	svc := NewService(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	ms := svc.MemoryStore()
	if ms == nil {
		t.Error("MemoryStore() returned nil")
	}
}

func TestService_GuidelineStore(t *testing.T) {
	t.Parallel()

	svc := NewService(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	gs := svc.GuidelineStore()
	if gs == nil {
		t.Error("GuidelineStore() returned nil")
	}
}
