package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFileTool_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "hello world"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	tool := NewReadFileTool(dir)
	out := tool.Run(context.Background(), ToolInput{"file": "test.txt"})
	if !out.Success {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	if out.Data != content {
		t.Errorf("got %q, want %q", out.Data, content)
	}
}

func TestReadFileTool_NoFile(t *testing.T) {
	t.Parallel()

	tool := NewReadFileTool(t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"file": ""})
	if out.Success {
		t.Fatal("expected failure for empty file path")
	}
}

func TestReadFileTool_NonExistent(t *testing.T) {
	t.Parallel()

	tool := NewReadFileTool(t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"file": "nonexistent.txt"})
	if out.Success {
		t.Fatal("expected failure for non-existent file")
	}
}

func TestReadFileTool_PathTraversal(t *testing.T) {
	t.Parallel()

	tool := NewReadFileTool(t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"file": "../etc/passwd"})
	if out.Success {
		t.Fatal("expected failure for path traversal")
	}
}

func TestWriteFileTool_Success(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tool := NewWriteFileTool(dir)
	out := tool.Run(context.Background(), ToolInput{
		"file":    "subdir/out.txt",
		"content": "test content",
	})
	if !out.Success {
		t.Fatalf("unexpected error: %s", out.Error)
	}

	data, err := os.ReadFile(filepath.Join(dir, "subdir/out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test content" {
		t.Errorf("got %q, want %q", string(data), "test content")
	}
}

func TestWriteFileTool_NoFile(t *testing.T) {
	t.Parallel()

	tool := NewWriteFileTool(t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"content": "data"})
	if out.Success {
		t.Fatal("expected failure for empty file path")
	}
}

func TestWriteFileTool_NoContent(t *testing.T) {
	t.Parallel()

	tool := NewWriteFileTool(t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"file": "f.txt"})
	if out.Success {
		t.Fatal("expected failure for empty content")
	}
}

func TestWriteFileTool_Overwrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("old"), 0644)

	tool := NewWriteFileTool(dir)
	out := tool.Run(context.Background(), ToolInput{
		"file":    "existing.txt",
		"content": "new",
	})
	if !out.Success {
		t.Fatalf("unexpected error: %s", out.Error)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "existing.txt"))
	if string(data) != "new" {
		t.Errorf("got %q, want %q", string(data), "new")
	}
}

func TestReadWriteFileTool_Name(t *testing.T) {
	t.Parallel()

	if got := NewReadFileTool(".").Name(); got != "read_file" {
		t.Errorf("ReadFileTool.Name() = %q", got)
	}
	if got := NewWriteFileTool(".").Name(); got != "write_file" {
		t.Errorf("WriteFileTool.Name() = %q", got)
	}
}
