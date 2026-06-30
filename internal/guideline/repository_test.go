package guideline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryStore_LoadDirectoryAndRank(t *testing.T) {
	dir := t.TempDir()
	store, err := NewRepositoryStore(filepath.Join(dir, "guidelines.json"))
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "repo")
	guidelineDir := filepath.Join(repo, ".agentos", "guidelines")
	if err := os.MkdirAll(guidelineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guidelineDir, "architecture.yaml"), []byte(`
guidelines:
  - title: Server handler convention
    content: Add Web UI APIs under internal/server and test handlers.
    type: architecture
    tags: [go-backend]
    required: true
  - title: Advisory docs
    content: Update docs when public endpoints change.
    type: documentation
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guidelineDir, "frontend.md"), []byte("# Frontend controls\n\nUse compact controls for dashboard workflows."), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadRepositoryDirectory(context.Background(), store, repo, "owner/repo", "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 {
		t.Fatalf("loaded = %d, want 3", len(loaded))
	}
	if _, err := LoadRepositoryDirectory(context.Background(), store, repo, "owner/repo", "main"); err != nil {
		t.Fatal(err)
	}

	results, err := store.List(context.Background(), &RepositoryGuidelineQuery{
		Repo:   "owner/repo",
		Branch: "main",
		Query:  "Add internal server handler",
		Agent:  "go-backend",
		Status: RepositoryGuidelineActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Title != "Server handler convention" || !results[0].Required {
		t.Fatalf("results = %+v, want required server guideline first", results)
	}
	all, err := store.List(context.Background(), &RepositoryGuidelineQuery{Repo: "owner/repo", Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("all = %d, want 3 after reload", len(all))
	}
	if !strings.Contains(results[0].Path, ".agentos/guidelines/architecture.yaml") {
		t.Fatalf("Path = %q, want repository guideline path", results[0].Path)
	}
}

func TestRepositoryStore_Archive(t *testing.T) {
	store, err := NewRepositoryStore(filepath.Join(t.TempDir(), "guidelines.json"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	entry := &RepositoryGuideline{
		Repo:    "owner/repo",
		Branch:  "main",
		Title:   "Run tests",
		Content: "Run go test ./...",
	}
	if err := store.Save(ctx, entry); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Archive(ctx, entry.ID); err != nil {
		t.Fatal(err)
	}
	active, err := store.List(ctx, &RepositoryGuidelineQuery{Repo: "owner/repo", Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active = %+v, want none", active)
	}
	archived, err := store.List(ctx, &RepositoryGuidelineQuery{Repo: "owner/repo", Branch: "main", Status: RepositoryGuidelineArchived})
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || archived[0].ID != entry.ID {
		t.Fatalf("archived = %+v, want archived entry", archived)
	}
}
