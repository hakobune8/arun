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

package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/safety"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/state"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/kazyamaz200/agentos/internal/tools"
)

func TestBaseAgent_New(t *testing.T) {
	t.Parallel()
	mock := llm.NewMockLLMClient(nil)
	a := NewBaseAgent("test-agent", mock)
	if a == nil {
		t.Fatal("expected non-nil agent")
	}
	if a.name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", a.name)
	}
}

func TestBaseAgent_Name(t *testing.T) {
	t.Parallel()
	mock := llm.NewMockLLMClient(nil)
	a := NewBaseAgent("my-agent", mock)
	if got := a.Name(); got != "my-agent" {
		t.Errorf("expected 'my-agent', got %q", got)
	}
}

func TestBaseAgent_ImplementsRuntimeAgent(t *testing.T) {
	t.Parallel()
	mock := llm.NewMockLLMClient(nil)
	a := NewBaseAgent("test", mock)
	// Compile-time check: assert that *BaseAgent satisfies the interface.
	var _ interface {
		Name() string
	} = a
	_ = a.Name()
}

func TestBaseAgent_ExecuteDoesNotRetryPassingTestsWithOutput(t *testing.T) {
	t.Parallel()

	rctx := newExecuteTestContext(t, "echo tests passed", "")
	a := NewBaseAgent("test", llm.NewMockLLMClient(nil))

	result, err := a.Execute(rctx, &runtime.Plan{})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("result.Success = false, error=%s", result.Error)
	}
	if result.Retries != 0 {
		t.Fatalf("Retries = %d, want 0", result.Retries)
	}
	if result.TestLog == "" {
		t.Fatal("expected test log to preserve stdout")
	}
}

func TestBaseAgent_ExecuteFailsAfterTestRetries(t *testing.T) {
	t.Parallel()

	rctx := newExecuteTestContext(t, "echo boom && exit 1", "")
	a := NewBaseAgent("test", llm.NewMockLLMClient(nil))

	result, err := a.Execute(rctx, &runtime.Plan{})
	if err == nil {
		t.Fatal("expected execute error")
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.Success {
		t.Fatal("result.Success = true, want false")
	}
	if result.Retries != 1 {
		t.Fatalf("Retries = %d, want 1", result.Retries)
	}
}

func TestBaseAgent_ExecuteFailsAfterLintRetries(t *testing.T) {
	t.Parallel()

	rctx := newExecuteTestContext(t, "echo tests passed", "echo lint failed && exit 1")
	a := NewBaseAgent("test", llm.NewMockLLMClient(nil))

	result, err := a.Execute(rctx, &runtime.Plan{})
	if err == nil {
		t.Fatal("expected execute error")
	}
	if result == nil {
		t.Fatal("expected result")
	}
	if result.Success {
		t.Fatal("result.Success = true, want false")
	}
	if result.Retries != 1 {
		t.Fatalf("Retries = %d, want 1", result.Retries)
	}
}

func TestBaseAgent_ExecuteEditStepWritesFile(t *testing.T) {
	t.Parallel()

	rctx := newExecuteTestContext(t, "test -f generated.txt", "")
	a := NewBaseAgent("test", llm.NewMockLLMClient([]llm.ChatResponse{
		{
			Choices: []llm.Choice{{
				Message: llm.Message{Content: `{"action":"edit","file":"generated.txt","content":"hello","reasoning":"create requested file"}`},
			}},
		},
	}))

	result, err := a.Execute(rctx, &runtime.Plan{Steps: []runtime.Step{{
		StepNumber:  1,
		Action:      "edit",
		Description: "Create generated.txt",
		TargetFiles: []string{"generated.txt"},
	}}})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("result.Success = false, error=%s", result.Error)
	}
	if data, err := os.ReadFile(filepath.Join(rctx.Workspace.RootDir(), "generated.txt")); err != nil || string(data) != "hello" {
		t.Fatalf("generated.txt = %q, err=%v", data, err)
	}
}

func TestBaseAgent_ExecuteShellStepUsesStructuredCommand(t *testing.T) {
	t.Parallel()

	rctx := newExecuteTestContext(t, "test -f shell-generated.txt", "")
	a := NewBaseAgent("test", llm.NewMockLLMClient([]llm.ChatResponse{
		{
			Choices: []llm.Choice{{
				Message: llm.Message{Content: `{"action":"shell","command":"printf ok > shell-generated.txt","reasoning":"create file"}`},
			}},
		},
	}))

	result, err := a.Execute(rctx, &runtime.Plan{Steps: []runtime.Step{{
		StepNumber:  1,
		Action:      "shell",
		Description: "Create shell-generated.txt using a shell command",
		TargetFiles: []string{"shell-generated.txt"},
	}}})
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("result.Success = false, error=%s", result.Error)
	}
	if _, err := os.Stat(filepath.Join(rctx.Workspace.RootDir(), "shell-generated.txt")); err != nil {
		t.Fatalf("shell-generated.txt missing: %v", err)
	}
}

func newExecuteTestContext(t *testing.T, testCmd, lintCmd string) *runtime.RunContext {
	t.Helper()

	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "-c", "user.name=AgentOS", "-c", "user.email=agentos@example.com", "commit", "-m", "init")

	registry := tools.NewRegistry()
	policy := safety.NewCommandPolicy(nil)
	registry.MustRegister(tools.NewSearchTool(repo))
	registry.MustRegister(tools.NewReadFileTool(repo))
	registry.MustRegister(tools.NewWriteFileTool(repo))
	registry.MustRegister(tools.NewShellTool(policy, repo))
	registry.MustRegister(tools.NewTestTool(repo))

	prof := profile.DefaultProfile()
	prof.Commands.Test = testCmd
	prof.Commands.Lint = lintCmd
	prof.Limits.MaxRetries = 1

	sb := sandbox.NewLocalSandbox(repo)
	if err := sb.PrepareRun("test-task"); err != nil {
		t.Fatal(err)
	}

	return &runtime.RunContext{
		Context:    context.Background(),
		Task:       &task.Task{ID: "test-task", Repo: repo, BaseBranch: "main", Branch: "agent/test"},
		Profile:    &prof,
		Workspace:  sb,
		Registry:   registry,
		Logger:     state.NewLogger(t.TempDir()),
		MaxRetries: prof.Limits.MaxRetries,
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
