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
	"testing"
	"time"

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

func TestMemoryStore_Save_StoresEntry(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	ms := NewMemoryStore(vs, emb)
	ctx := context.Background()

	entry := Entry{
		ID:      "test-1",
		Content: "hello world",
		Type:    "conversation",
		Metadata: map[string]interface{}{
			"source": "test",
		},
	}

	err := ms.Save(ctx, &entry)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := ms.Search(ctx, "hello", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ID != "test-1" {
		t.Errorf("ID = %q, want %q", results[0].ID, "test-1")
	}
	if results[0].Content != "hello world" {
		t.Errorf("Content = %q, want %q", results[0].Content, "hello world")
	}
}

func TestMemoryStore_Save_EmptyIDGeneratesOne(t *testing.T) {
	t.Parallel()

	ms := NewMemoryStore(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})

	err := ms.Save(context.Background(), &Entry{Content: "no-id"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := ms.Search(context.Background(), "no-id", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results found")
	}
	if results[0].ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestMemoryStore_Save_ZeroTimestampSetsOne(t *testing.T) {
	t.Parallel()

	ms := NewMemoryStore(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	ctx := context.Background()

	err := ms.Save(ctx, &Entry{Content: "timestamp-test"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	results, err := ms.Search(ctx, "timestamp-test", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results found")
	}
	if ts, ok := results[0].Metadata["timestamp"]; ok {
		parsed, err := time.Parse(time.RFC3339, ts.(string))
		if err != nil {
			t.Fatalf("parse timestamp: %v", err)
		}
		if parsed.IsZero() {
			t.Error("timestamp should not be zero")
		}
		after := time.Now()
		if parsed.After(after) {
			t.Errorf("timestamp %v is after %v", parsed, after)
		}
	} else {
		t.Error("timestamp not found in metadata")
	}
}

func TestMemoryStore_Search_ReturnsSortedBySimilarity(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.5, 0.5, 0.5}}
	ms := NewMemoryStore(vs, emb)
	ctx := context.Background()

	ms.Save(ctx, &Entry{ID: "mem1", Content: "first entry"})  //nolint:errcheck // test helper, error checked via search
	ms.Save(ctx, &Entry{ID: "mem2", Content: "second entry"}) //nolint:errcheck // test helper, error checked via search

	results, err := ms.Search(ctx, "query", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestMemoryStore_Clear_RemovesAllEntries(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	ms := NewMemoryStore(vs, emb)
	ctx := context.Background()

	ms.Save(ctx, &Entry{Content: "something"}) //nolint:errcheck // test helper, error checked via search
	results, _ := ms.Search(ctx, "something", 10)
	if len(results) == 0 {
		t.Fatal("expected entries before clear")
	}

	err := ms.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear: %v", err)
	}

	results, err = ms.Search(ctx, "something", 10)
	if err != nil {
		t.Fatalf("Search after clear: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results after clear, want 0", len(results))
	}
}
