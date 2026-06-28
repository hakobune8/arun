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
	"os/exec"
	"path/filepath"
	"testing"
)

func initGitRepo(t *testing.T, dir string) {
	t.Helper()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // Test code for git operations
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("git init failed: %v", err)
		}
	}
}

func TestGitTool_InitAndBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	tool := NewGitTool(dir)
	out := tool.Run(context.Background(), ToolInput{"subcommand": "branch"})
	if !out.Success {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	data := out.Data.(map[string]string)
	if data["stdout"] != "master" && data["stdout"] != "main" {
		t.Logf("branch name = %q (acceptable)", data["stdout"])
	}
}

func TestGitTool_Status(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	tool := NewGitTool(dir)
	out := tool.Run(context.Background(), ToolInput{"subcommand": "status"})
	if !out.Success {
		t.Fatalf("unexpected error: %s", out.Error)
	}
}

func TestGitTool_AddAndCommit(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0o600) //nolint:errcheck // test helper, error checked via tool output

	tool := NewGitTool(dir)

	addOut := tool.Run(context.Background(), ToolInput{
		"subcommand": "add",
		"args":       "test.txt",
	})
	if !addOut.Success {
		t.Fatalf("add failed: %s", addOut.Error)
	}

	commitOut := tool.Run(context.Background(), ToolInput{
		"subcommand": "commit",
		"message":    "initial commit",
	})
	if !commitOut.Success {
		t.Fatalf("commit failed: %s", commitOut.Error)
	}

	logOut := tool.Run(context.Background(), ToolInput{"subcommand": "log"})
	if !logOut.Success {
		t.Fatalf("log failed: %s", logOut.Error)
	}
}

func TestGitTool_Diff(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello"), 0o600) //nolint:errcheck // test helper, error checked via tool output

	tool := NewGitTool(dir)
	out := tool.Run(context.Background(), ToolInput{"subcommand": "add", "args": "f.txt"})
	if !out.Success {
		t.Fatal(out.Error)
	}
	out = tool.Run(context.Background(), ToolInput{"subcommand": "commit", "message": "msg"})
	if !out.Success {
		t.Fatal(out.Error)
	}

	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("world"), 0o600) //nolint:errcheck // test helper, error checked via tool output

	diff, err := tool.Diff(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if diff == "" {
		t.Error("expected non-empty diff")
	}
}

func TestGitTool_CurrentBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	tool := NewGitTool(dir)
	branch, err := tool.CurrentBranch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if branch == "" {
		t.Error("expected non-empty branch name")
	}
}

func TestGitTool_CheckoutNewBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	tool := NewGitTool(dir)
	out := tool.Run(context.Background(), ToolInput{
		"subcommand": "checkout_new_branch",
		"args":       "feature/test",
	})
	if !out.Success {
		t.Fatalf("checkout failed: %s", out.Error)
	}

	branch, _ := tool.CurrentBranch(context.Background())
	if branch != "feature/test" {
		t.Errorf("branch = %q, want %q", branch, "feature/test")
	}
}

func TestGitTool_UnknownSubcommand(t *testing.T) {
	t.Parallel()

	tool := NewGitTool(t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"subcommand": "unknown"})
	if out.Success {
		t.Fatal("expected failure for unknown subcommand")
	}
}

func TestGitTool_CheckoutNoArgs(t *testing.T) {
	t.Parallel()

	tool := NewGitTool(t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"subcommand": "checkout"})
	if out.Success {
		t.Fatal("expected failure when no args for checkout")
	}
}

func TestGitTool_AddNoArgs(t *testing.T) {
	t.Parallel()

	tool := NewGitTool(t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"subcommand": "add"})
	if out.Success {
		t.Fatal("expected failure when no args for add")
	}
}

func TestGitTool_Name(t *testing.T) {
	t.Parallel()

	tool := NewGitTool(".")
	if got := tool.Name(); got != "git" {
		t.Errorf("Name() = %q, want %q", got, "git")
	}
}
