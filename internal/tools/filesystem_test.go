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
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0o600) //nolint:errcheck // test helper, error checked via tool output

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

	data, err := os.ReadFile(filepath.Join(dir, "subdir", "out.txt"))
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
	os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("old"), 0o600) //nolint:errcheck // test helper, error checked via tool output

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
