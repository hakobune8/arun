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

package guideline

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kazyamaz200/agentos/internal/vector"
	"gopkg.in/yaml.v3"
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

func TestNewStore_ReturnsValidStore(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	s := NewStore(vs, emb)

	if s == nil {
		t.Fatal("NewStore returned nil")
	}
}

func TestStore_Add(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}}
	s := NewStore(vs, emb)
	ctx := context.Background()

	g := Guideline{
		ID:    "gl-1",
		Title: "Test Guideline",
		Rule:  "Always write tests",
		Tags:  []string{"testing", "go"},
	}

	err := s.Add(ctx, &g)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	results, err := s.Search(ctx, "testing", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ID != "gl-1" {
		t.Errorf("ID = %q, want %q", results[0].ID, "gl-1")
	}
	if results[0].Title != "Test Guideline" {
		t.Errorf("Title = %q, want %q", results[0].Title, "Test Guideline")
	}
	if results[0].Rule != "Always write tests" {
		t.Errorf("Rule = %q, want %q", results[0].Rule, "Always write tests")
	}
}

func TestStore_Search_ReturnsMatchingGuidelines(t *testing.T) {
	t.Parallel()

	vs := vector.NewLocalStore(t.TempDir())
	emb := &mockEmbedder{fixedVec: []float32{0.5, 0.5, 0.5}}
	s := NewStore(vs, emb)
	ctx := context.Background()

	s.Add(ctx, &Guideline{ID: "gl-1", Title: "One", Rule: "Rule one"})  //nolint:errcheck // test helper, error checked via search
	s.Add(ctx, &Guideline{ID: "gl-2", Title: "Two", Rule: "Rule two"})  //nolint:errcheck // test helper, error checked via search

	results, err := s.Search(ctx, "query", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestStore_Add_EmptyIDGeneratesOne(t *testing.T) {
	t.Parallel()

	s := NewStore(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	err := s.Add(context.Background(), &Guideline{Title: "No ID", Rule: "foo"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	results, _ := s.Search(context.Background(), "foo", 10)
	if len(results) == 0 {
		t.Fatal("no results found")
	}
	if results[0].ID == "" {
		t.Error("ID should not be empty")
	}
}

func TestStore_Add_WithExample(t *testing.T) {
	t.Parallel()

	s := NewStore(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	err := s.Add(context.Background(), &Guideline{
		ID:      "gl-ex",
		Title:   "Example",
		Rule:    "Include examples",
		Example: "See below",
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	results, _ := s.Search(context.Background(), "example", 10)
	if len(results) == 0 {
		t.Fatal("no results found")
	}
	if results[0].Example != "See below" {
		t.Errorf("Example = %q, want %q", results[0].Example, "See below")
	}
}

func TestLoadDirectory_LoadsYAMLFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := NewStore(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})

	guidelines := []Guideline{
		{ID: "gl-1", Title: "Test", Rule: "Write tests"},
	}
	data, err := yaml.Marshal(guidelines)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "guidelines.yaml"), data, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := s.LoadDirectory(dir); err != nil {
		t.Fatalf("LoadDirectory: %v", err)
	}

	results, err := s.Search(context.Background(), "tests", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no guidelines loaded")
	}
}

func TestLoadDirectory_NonExistentDirReturnsNil(t *testing.T) {
	t.Parallel()

	s := NewStore(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	err := s.LoadDirectory("/nonexistent/path")
	if err != nil {
		t.Fatalf("LoadDirectory with non-existent dir: %v", err)
	}
}

func TestLoadDirectory_SkipsNonYAMLFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	s := NewStore(vector.NewLocalStore(t.TempDir()), &mockEmbedder{fixedVec: []float32{0.1, 0.2, 0.3}})
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0o600) //nolint:errcheck // test helper, error checked via search

	err := s.LoadDirectory(dir)
	if err != nil {
		t.Fatalf("LoadDirectory: %v", err)
	}
}
