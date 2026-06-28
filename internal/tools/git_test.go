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
		cmd := exec.Command(args[0], args[1:]...)
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

	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

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

	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hello"), 0644)

	tool := NewGitTool(dir)
	out := tool.Run(context.Background(), ToolInput{"subcommand": "add", "args": "f.txt"})
	if !out.Success {
		t.Fatal(out.Error)
	}
	out = tool.Run(context.Background(), ToolInput{"subcommand": "commit", "message": "msg"})
	if !out.Success {
		t.Fatal(out.Error)
	}

	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("world"), 0644)

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
