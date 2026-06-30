package memory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRepositoryStore_ListScopesByRepoBranchAndStatus(t *testing.T) {
	t.Parallel()

	store, err := NewRepositoryStore(filepath.Join(t.TempDir(), "repo-memory.json"))
	if err != nil {
		t.Fatalf("NewRepositoryStore() error = %v", err)
	}
	ctx := context.Background()
	entries := []RepositoryEntry{
		{Repo: "https://github.com/example/app.git", Branch: "main", Type: "validation", Content: "run npm test", Status: RepositoryMemoryApproved},
		{Repo: "example/app", Branch: "dev", Type: "validation", Content: "run go test", Status: RepositoryMemoryApproved},
		{Repo: "example/other", Branch: "main", Type: "pitfall", Content: "avoid secrets", Status: RepositoryMemoryApproved},
		{Repo: "example/app", Branch: "main", Type: "pitfall", Content: "pending review", Status: RepositoryMemoryPending},
	}
	for i := range entries {
		if err := store.Save(ctx, &entries[i]); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	results, err := store.List(ctx, &RepositoryQuery{Repo: "example/app", Branch: "main", Status: RepositoryMemoryApproved})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1: %+v", len(results), results)
	}
	if results[0].Content != "run npm test" {
		t.Fatalf("Content = %q, want npm test", results[0].Content)
	}
}

func TestRepositoryStore_ApproveAndDelete(t *testing.T) {
	t.Parallel()

	store, err := NewRepositoryStore(filepath.Join(t.TempDir(), "repo-memory.json"))
	if err != nil {
		t.Fatalf("NewRepositoryStore() error = %v", err)
	}
	ctx := context.Background()
	entry := &RepositoryEntry{
		Repo:    "example/app",
		Branch:  "main",
		Type:    "architecture",
		Content: "Use internal/server for Web UI API handlers.",
		Status:  RepositoryMemoryPending,
	}
	if err := store.Save(ctx, entry); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	approved, err := store.Approve(ctx, entry.ID, "kazyamaz200")
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if approved.Status != RepositoryMemoryApproved || approved.ApprovedBy != "kazyamaz200" || approved.ApprovedAt == nil {
		t.Fatalf("approved entry = %+v", approved)
	}

	if err := store.Delete(ctx, entry.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	results, err := store.List(ctx, &RepositoryQuery{Repo: "example/app", Branch: "main"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results after delete, want 0", len(results))
	}
}

func TestRepositoryStore_PinnedResultsFirst(t *testing.T) {
	t.Parallel()

	store, err := NewRepositoryStore(filepath.Join(t.TempDir(), "repo-memory.json"))
	if err != nil {
		t.Fatalf("NewRepositoryStore() error = %v", err)
	}
	ctx := context.Background()
	first := &RepositoryEntry{Repo: "example/app", Branch: "main", Content: "normal", Status: RepositoryMemoryApproved}
	second := &RepositoryEntry{Repo: "example/app", Branch: "main", Content: "pinned", Status: RepositoryMemoryApproved, Pinned: true}
	if err := store.Save(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, second); err != nil {
		t.Fatal(err)
	}
	results, err := store.List(ctx, &RepositoryQuery{Repo: "example/app", Branch: "main"})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(results) != 2 || !results[0].Pinned {
		t.Fatalf("results = %+v, want pinned first", results)
	}
}
