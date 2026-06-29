package orchestrator

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/sandbox"
)

func TestNewOrchestrator(t *testing.T) {
	t.Parallel()

	llmClient := llm.NewMockLLMClient(nil)
	sb := sandbox.NewLocalSandbox(t.TempDir())
	agents := map[string]runtime.Agent{}
	cfg := &runtime.Config{}

	o := NewOrchestrator(llmClient, sb, agents, cfg)
	if o == nil {
		t.Fatal("NewOrchestrator returned nil")
	}
}

func TestSetStrategy(t *testing.T) {
	t.Parallel()

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(t.TempDir()),
		map[string]runtime.Agent{},
		&runtime.Config{},
	)

	o.SetStrategy(StrategyParallel)
}

func TestMergeResults(t *testing.T) {
	t.Parallel()

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(t.TempDir()),
		map[string]runtime.Agent{},
		&runtime.Config{},
	)

	results := []SubtaskResult{
		{SubtaskID: "step-1", Output: "done", Success: true},
	}
	merged := o.MergeResults(results)
	if merged == "" {
		t.Error("MergeResults returned empty string")
	}
}

func TestDefaultAgent_Empty(t *testing.T) {
	t.Parallel()

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(t.TempDir()),
		map[string]runtime.Agent{},
		&runtime.Config{},
	)

	if a := o.DefaultAgent(); a != nil {
		t.Error("DefaultAgent should be nil when no agents registered")
	}
}

func TestExecute_EmptyPlan(t *testing.T) {
	t.Parallel()

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(t.TempDir()),
		map[string]runtime.Agent{},
		&runtime.Config{},
	)

	results, err := o.Execute(context.Background(), &TaskPlan{})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}

func TestExecuteSubtask_UsesDefaultProfileAndRepo(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("AGENTOS_HOME", filepath.Join(t.TempDir(), "agentos-home"))

	agent := &recordingAgent{}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"test-agent": agent},
		&runtime.Config{},
	)

	result := o.executeSubtask(context.Background(), Subtask{
		ID:          "step-1",
		Description: "exercise repo",
		AgentName:   "test-agent",
	}, "")
	if !result.Success {
		t.Fatalf("executeSubtask() failed: %s", result.Error)
	}

	if agent.taskRepo != repo {
		t.Fatalf("task repo = %q, want %q", agent.taskRepo, repo)
	}
	if agent.baseBranch != "main" {
		t.Fatalf("base branch = %q, want main", agent.baseBranch)
	}
	if agent.profileName != "test-agent" {
		t.Fatalf("profile name = %q, want test-agent", agent.profileName)
	}
	if agent.workspaceRoot != repo {
		t.Fatalf("workspace root = %q, want %q", agent.workspaceRoot, repo)
	}
}

func TestStrategy_Constants(t *testing.T) {
	t.Parallel()

	if StrategySequential != Strategy("sequential") {
		t.Errorf("StrategySequential = %q, want %q", StrategySequential, "sequential")
	}
	if StrategyParallel != Strategy("parallel") {
		t.Errorf("StrategyParallel = %q, want %q", StrategyParallel, "parallel")
	}
}

func TestSubtask_Defaults(t *testing.T) {
	t.Parallel()

	st := Subtask{}
	if st.ID != "" {
		t.Errorf("ID = %q, want empty", st.ID)
	}
	if st.Description != "" {
		t.Errorf("Description = %q, want empty", st.Description)
	}
}

func TestSubtaskResult_Defaults(t *testing.T) {
	t.Parallel()

	sr := SubtaskResult{}
	if sr.SubtaskID != "" {
		t.Errorf("SubtaskID = %q, want empty", sr.SubtaskID)
	}
	if sr.Success {
		t.Error("Success should be false")
	}
}

type recordingAgent struct {
	taskRepo      string
	baseBranch    string
	profileName   string
	workspaceRoot string
}

func (a *recordingAgent) Name() string {
	return "test-agent"
}

func (a *recordingAgent) Plan(ctx *runtime.RunContext) (*runtime.Plan, error) {
	a.taskRepo = ctx.Task.Repo
	a.baseBranch = ctx.Task.BaseBranch
	a.profileName = ctx.Profile.Name
	a.workspaceRoot = ctx.Workspace.RootDir()
	return &runtime.Plan{Summary: "ok"}, nil
}

func (a *recordingAgent) Execute(_ *runtime.RunContext, _ *runtime.Plan) (*runtime.ExecutionResult, error) {
	return &runtime.ExecutionResult{Success: true}, nil
}

func (a *recordingAgent) Review(_ *runtime.RunContext, _ *runtime.ExecutionResult) (*runtime.ReviewResult, error) {
	return &runtime.ReviewResult{Approved: true, Summary: "ok"}, nil
}
