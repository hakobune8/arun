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

package sandbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewWorkspace(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace("/tmp/test-root")
	if ws == nil {
		t.Fatal("NewWorkspace returned nil")
	}
}

func TestWorkspace_AbsPath(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace("/project/root")
	path := ws.AbsPath("src/main.go")
	expected := filepath.Join("/project", "root", "src/main.go") //nolint:gocritic // test expects exact path
	if path != expected {
		t.Errorf("AbsPath = %q, want %q", path, expected)
	}
}

func TestWorkspace_RepoAbsPath_Relative(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace("/project/root")
	path := ws.RepoAbsPath("internal/foo.go")
	expected := filepath.Join("/project", "root", "internal/foo.go") //nolint:gocritic // test expects exact path
	if path != expected {
		t.Errorf("RepoAbsPath = %q, want %q", path, expected)
	}
}

func TestWorkspace_RepoAbsPath_Absolute(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace("/project/root")
	path := ws.RepoAbsPath("/absolute/path/file.go")
	abs := filepath.IsAbs("/absolute/path/file.go")
	if abs && path != "/absolute/path/file.go" {
		t.Errorf("RepoAbsPath = %q, want %q", path, "/absolute/path/file.go")
	}
	if !abs && path != filepath.Join("/project", "root", "/absolute/path/file.go") { //nolint:gocritic // test expects exact path
		t.Errorf("RepoAbsPath = %q, want %q", path, filepath.Join("/project", "root", "/absolute/path/file.go")) //nolint:gocritic // test expects exact path
	}
}

func TestWorkspace_RunPath(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace("/project/root")
	if ws.RunPath() != "" {
		t.Errorf("RunPath before PrepareRun = %q, want empty", ws.RunPath())
	}
}

func TestWorkspace_PrepareRun(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace(t.TempDir())
	err := ws.PrepareRun("task-42")
	if err != nil {
		t.Fatalf("PrepareRun: %v", err)
	}
	if ws.TaskID != "task-42" {
		t.Errorf("TaskID = %q, want %q", ws.TaskID, "task-42")
	}
	if ws.RunDir == "" {
		t.Error("RunDir should not be empty")
	}
	if _, err := os.Stat(ws.RunDir); os.IsNotExist(err) {
		t.Error("RunDir was not created")
	}
}

func TestWorkspace_SaveFile(t *testing.T) {
	t.Parallel()

	ws := NewWorkspace(t.TempDir())
	if err := ws.PrepareRun("save-test"); err != nil {
		t.Fatalf("PrepareRun: %v", err)
	}

	data := []byte("hello world")
	if err := ws.SaveFile("test.txt", data); err != nil {
		t.Fatalf("SaveFile: %v", err)
	}

	path := filepath.Join(ws.RunDir, "test.txt")
	read, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(read) != "hello world" {
		t.Errorf("file content = %q, want %q", string(read), "hello world")
	}
}
