package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLoader(t *testing.T) {
	t.Parallel()

	l := NewLoader()
	if l == nil {
		t.Fatal("NewLoader() returned nil")
	}
}

func TestLoader_Load(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "task.yaml")
	content := []byte(`id: loader-test
repo: org/repo
title: Loader test
`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	l := NewLoader()
	task, err := l.Load(path)
	if err != nil {
		t.Fatalf("Loader.Load() error = %v", err)
	}
	if task.ID != "loader-test" {
		t.Errorf("ID = %q, want %q", task.ID, "loader-test")
	}
	if task.Repo != "org/repo" {
		t.Errorf("Repo = %q, want %q", task.Repo, "org/repo")
	}
	if task.Title != "Loader test" {
		t.Errorf("Title = %q, want %q", task.Title, "Loader test")
	}
}

func TestLoader_Load_MissingFile(t *testing.T) {
	t.Parallel()

	l := NewLoader()
	_, err := l.Load("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
