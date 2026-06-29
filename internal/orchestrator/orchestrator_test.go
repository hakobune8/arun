package orchestrator

import (
	"context"
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
