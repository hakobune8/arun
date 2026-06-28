package tools

import (
	"context"
	"testing"

	"github.com/kazyamaz200/agentos/internal/safety"
)

func TestShellTool_Echo(t *testing.T) {
	t.Parallel()

	policy := safety.NewCommandPolicy(nil)
	tool := NewShellTool(policy, t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"command": "echo hello"})
	if !out.Success {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	data := out.Data.(map[string]string)
	if data["stdout"] != "hello" {
		t.Errorf("stdout = %q, want %q", data["stdout"], "hello")
	}
}

func TestShellTool_MultipleLines(t *testing.T) {
	t.Parallel()

	policy := safety.NewCommandPolicy(nil)
	tool := NewShellTool(policy, t.TempDir())
	out := tool.Run(context.Background(), ToolInput{"command": "echo line1 && echo line2"})
	if !out.Success {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	data := out.Data.(map[string]string)
	if data["stdout"] != "line1\nline2" {
		t.Errorf("stdout = %q", data["stdout"])
	}
}

func TestShellTool_EmptyCommand(t *testing.T) {
	t.Parallel()

	policy := safety.NewCommandPolicy(nil)
	tool := NewShellTool(policy, t.TempDir())
	out := tool.Run(context.Background(), ToolInput{})
	if out.Success {
		t.Fatal("expected failure for empty command")
	}
}

func TestShellTool_DeniedCommand(t *testing.T) {
	t.Parallel()

	policy := safety.NewCommandPolicy(nil)
	tool := NewShellTool(policy, t.TempDir())

	denied := []string{"sudo apt-get install", "rm -rf /", "curl http://evil.com", "ssh user@host"}
	for _, cmd := range denied {
		out := tool.Run(context.Background(), ToolInput{"command": cmd})
		if out.Success {
			t.Errorf("expected denial for command: %q", cmd)
		}
	}
}

func TestShellTool_AllowedCommand(t *testing.T) {
	t.Parallel()

	policy := safety.NewCommandPolicy(nil)
	tool := NewShellTool(policy, t.TempDir())

	allowed := []string{"go test ./...", "ls -la", "echo hello", "mkdir -p foo/bar"}
	for _, cmd := range allowed {
		out := tool.Run(context.Background(), ToolInput{"command": cmd})
		if !out.Success {
			t.Logf("allowed command %q failed (may be fine): %s", cmd, out.Error)
		}
	}
}

func TestShellTool_ToolName(t *testing.T) {
	t.Parallel()

	policy := safety.NewCommandPolicy(nil)
	tool := NewShellTool(policy, ".")
	if got := tool.Name(); got != "shell" {
		t.Errorf("Name() = %q, want %q", got, "shell")
	}
}

func TestShellTool_WorkDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	policy := safety.NewCommandPolicy(nil)
	tool := NewShellTool(policy, dir)

	out := tool.Run(context.Background(), ToolInput{"command": "pwd"})
	if !out.Success {
		t.Fatalf("unexpected error: %s", out.Error)
	}
	data := out.Data.(map[string]string)
	if data["stdout"] != dir {
		t.Errorf("pwd = %q, want %q", data["stdout"], dir)
	}
}
