package memory

import (
	"context"
	"testing"
)

func TestJSONStore_SaveAndSearch(t *testing.T) {
	t.Parallel()

	s, err := NewJSONStore(t.TempDir() + "/mem.jsonl")
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	ctx := context.Background()
	if err := s.Save(ctx, &Entry{ID: "e1", Content: "hello world", Type: "test"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := s.Save(ctx, &Entry{ID: "e2", Content: "goodbye world", Type: "test"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := s.Save(ctx, &Entry{ID: "e3", Content: "hello agent", Type: "test"}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	results, err := s.Search(ctx, "hello", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].ID != "e1" && results[0].ID != "e3" {
		t.Errorf("unexpected result ID: %q", results[0].ID)
	}

	results, err = s.Search(ctx, "goodbye", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ID != "e2" {
		t.Errorf("ID = %q, want %q", results[0].ID, "e2")
	}
}

func TestJSONStore_Search_Limit(t *testing.T) {
	t.Parallel()

	s, err := NewJSONStore(t.TempDir() + "/mem.jsonl")
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		s.Save(ctx, &Entry{Content: "entry data", Type: "test"}) //nolint:errcheck // fine for test
	}

	results, err := s.Search(ctx, "entry", 3)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
}

func TestJSONStore_Type(t *testing.T) {
	t.Parallel()

	s, err := NewJSONStore(t.TempDir() + "/mem.jsonl")
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	if s.Type() != "json" {
		t.Errorf("Type() = %q, want %q", s.Type(), "json")
	}
}

func TestJSONStore_Clear(t *testing.T) {
	t.Parallel()

	s, err := NewJSONStore(t.TempDir() + "/mem.jsonl")
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}

	ctx := context.Background()
	s.Save(ctx, &Entry{Content: "something"}) //nolint:errcheck // fine for test

	if err := s.Clear(ctx); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	results, err := s.Search(ctx, "something", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results after clear, want 0", len(results))
	}
}

func TestJSONStore_Persistence(t *testing.T) {
	path := t.TempDir() + "/persist.jsonl"

	s1, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	ctx := context.Background()
	s1.Save(ctx, &Entry{ID: "p1", Content: "persistent data", Type: "test"}) //nolint:errcheck // fine for test
	s1.Save(ctx, &Entry{ID: "p2", Content: "more data", Type: "test"})       //nolint:errcheck // fine for test

	// Load from same path
	s2, err := NewJSONStore(path)
	if err != nil {
		t.Fatalf("NewJSONStore() error = %v", err)
	}
	results, err := s2.Search(ctx, "persistent", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].ID != "p1" {
		t.Errorf("ID = %q, want %q", results[0].ID, "p1")
	}
}

func TestVectorStore_ImplementsStore(t *testing.T) {
	t.Parallel()

	var _ Store = (*VectorStore)(nil)
}

func TestJSONStore_ImplementsStore(t *testing.T) {
	t.Parallel()

	var _ Store = (*JSONStore)(nil)
}

func TestNew_VectorBackend(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	vs := NewVectorStore(nil, nil) // will fail embed but creation itself is fine
	if vs.Type() != "vector" {
		t.Errorf("Type() = %q, want %q", vs.Type(), "vector")
	}
	_ = ctx
}

func TestNew_JSONBackend(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := Config{Backend: "json", Path: t.TempDir() + "/test.jsonl"}
	s, err := New(ctx, cfg, nil, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if s.Type() != "json" {
		t.Errorf("Type() = %q, want %q", s.Type(), "json")
	}
}

func TestNew_UnknownBackend(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, err := New(ctx, Config{Backend: "unknown"}, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}
