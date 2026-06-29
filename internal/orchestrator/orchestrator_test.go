package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestPlan_EmptyLLMContentUsesFallbackPlan(t *testing.T) {
	t.Parallel()

	o := NewOrchestrator(
		llm.NewMockLLMClient([]llm.ChatResponse{{
			Choices: []llm.Choice{{Message: llm.Message{Role: llm.RoleAssistant}}},
		}}),
		sandbox.NewLocalSandbox(t.TempDir()),
		map[string]runtime.Agent{
			"go-backend": &recordingAgent{name: "go-backend"},
			"docs":       &recordingAgent{name: "docs"},
			"ci-fixer":   &recordingAgent{name: "ci-fixer"},
			"reviewer":   &recordingAgent{name: "reviewer"},
		},
		&runtime.Config{},
	)

	parentTask := "create a Go HTTP service with /healthz and /"
	plan, err := o.Plan(context.Background(), parentTask)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Subtasks) != 4 {
		t.Fatalf("got %d subtasks, want 4", len(plan.Subtasks))
	}
	if plan.Subtasks[2].AgentName != "ci-fixer" || len(plan.Subtasks[2].Deps) != 1 || plan.Subtasks[2].Deps[0] != "step-1" {
		t.Fatalf("ci-fixer fallback subtask = %+v, want dependency on step-1", plan.Subtasks[2])
	}
	if plan.Subtasks[3].AgentName != "reviewer" || len(plan.Subtasks[3].Deps) != 3 {
		t.Fatalf("reviewer fallback subtask = %+v, want dependencies on implementation, docs, and CI", plan.Subtasks[3])
	}
	if !strings.Contains(plan.Subtasks[0].Description, parentTask) || !strings.Contains(plan.Subtasks[0].Description, "go.mod") {
		t.Fatalf("go-backend fallback description = %q, want parent task and concrete Go files", plan.Subtasks[0].Description)
	}
}

func TestExecuteWithObserver_EmitsSubtaskEvents(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("AGENTOS_HOME", filepath.Join(t.TempDir(), "agentos-home"))

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"test-agent": &recordingAgent{}},
		&runtime.Config{},
	)
	o.SetSubtaskTimeout(time.Minute)

	var events []SubtaskEvent
	results, err := o.ExecuteWithObserver(context.Background(), &TaskPlan{
		Subtasks: []Subtask{{
			ID:          "step-1",
			Description: "exercise repo",
			AgentName:   "test-agent",
		}},
	}, func(event SubtaskEvent) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatalf("ExecuteWithObserver() error = %v", err)
	}
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("results = %+v, want one success", results)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want started and completed", events)
	}
	if events[0].Type != SubtaskStarted || events[1].Type != SubtaskCompleted {
		t.Fatalf("event types = %s, %s", events[0].Type, events[1].Type)
	}
	if events[1].Result == nil || !events[1].Result.Success {
		t.Fatalf("completed event result = %+v, want success", events[1].Result)
	}
}

func TestExecuteParallel_RespectsDependencies(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("AGENTOS_HOME", filepath.Join(t.TempDir(), "agentos-home"))

	agent := &recordingAgent{delay: 10 * time.Millisecond}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"test-agent": agent},
		&runtime.Config{},
	)
	o.SetStrategy(StrategyParallel)
	o.SetSubtaskTimeout(time.Minute)

	var mu sync.Mutex
	started := make(map[string]time.Time)
	finished := make(map[string]time.Time)
	results, err := o.ExecuteWithObserver(context.Background(), &TaskPlan{
		Subtasks: []Subtask{
			{ID: "step-1", Description: "first", AgentName: "test-agent"},
			{ID: "step-2", Description: "second", AgentName: "test-agent", Deps: []string{"step-1"}},
			{ID: "step-3", Description: "independent", AgentName: "test-agent"},
		},
	}, func(event SubtaskEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch event.Type {
		case SubtaskStarted:
			started[event.Subtask.ID] = event.Started
		case SubtaskCompleted:
			finished[event.Subtask.ID] = event.Finished
		}
	})
	if err != nil {
		t.Fatalf("ExecuteWithObserver() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	if started["step-2"].Before(finished["step-1"]) {
		t.Fatalf("dependent subtask started before dependency finished: step-2=%s step-1-finished=%s", started["step-2"], finished["step-1"])
	}
}

func TestExecuteParallel_SkipsSubtaskWhenDependencyFails(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("AGENTOS_HOME", filepath.Join(t.TempDir(), "agentos-home"))

	agent := &recordingAgent{failTasks: map[string]bool{"step-1": true}}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"test-agent": agent},
		&runtime.Config{},
	)
	o.SetStrategy(StrategyParallel)
	o.SetSubtaskTimeout(time.Minute)

	var mu sync.Mutex
	var started []string
	results, err := o.ExecuteWithObserver(context.Background(), &TaskPlan{
		Subtasks: []Subtask{
			{ID: "step-1", Description: "first", AgentName: "test-agent"},
			{ID: "step-2", Description: "second", AgentName: "test-agent", Deps: []string{"step-1"}},
		},
	}, func(event SubtaskEvent) {
		if event.Type == SubtaskStarted {
			mu.Lock()
			defer mu.Unlock()
			started = append(started, event.Subtask.ID)
		}
	})
	if err == nil {
		t.Fatal("ExecuteWithObserver() error = nil, want failure")
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[1].Success || !strings.Contains(results[1].Error, `dependency "step-1" failed`) {
		t.Fatalf("dependent result = %+v, want dependency failure", results[1])
	}
	mu.Lock()
	defer mu.Unlock()
	for _, id := range started {
		if id == "step-2" {
			t.Fatal("dependent subtask started despite failed dependency")
		}
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

func TestExecuteSubtask_ScopesRuntimeTaskIDWithRunID(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("AGENTOS_HOME", filepath.Join(t.TempDir(), "agentos-home"))

	agent := &recordingAgent{}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"test-agent": agent},
		&runtime.Config{},
	)
	o.SetRunID("run-abc")

	result := o.executeSubtask(context.Background(), Subtask{
		ID:          "step-1",
		Description: "exercise repo",
		AgentName:   "test-agent",
	}, "")
	if !result.Success {
		t.Fatalf("executeSubtask() failed: %s", result.Error)
	}
	if agent.taskID != "run-abc-step-1" {
		t.Fatalf("task ID = %q, want run-scoped ID", agent.taskID)
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
	name          string
	taskID        string
	taskRepo      string
	baseBranch    string
	profileName   string
	workspaceRoot string
	failTasks     map[string]bool
	delay         time.Duration
	mu            sync.Mutex
}

func (a *recordingAgent) Name() string {
	if a.name != "" {
		return a.name
	}
	return "test-agent"
}

func (a *recordingAgent) Plan(ctx *runtime.RunContext) (*runtime.Plan, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.taskID = ctx.Task.ID
	a.taskRepo = ctx.Task.Repo
	a.baseBranch = ctx.Task.BaseBranch
	a.profileName = ctx.Profile.Name
	a.workspaceRoot = ctx.Workspace.RootDir()
	return &runtime.Plan{Summary: "ok"}, nil
}

func (a *recordingAgent) Execute(ctx *runtime.RunContext, _ *runtime.Plan) (*runtime.ExecutionResult, error) {
	if a.delay > 0 {
		time.Sleep(a.delay)
	}
	if a.failTasks[ctx.Task.ID] {
		return &runtime.ExecutionResult{Success: false}, fmt.Errorf("forced failure")
	}
	return &runtime.ExecutionResult{Success: true}, nil
}

func (a *recordingAgent) Review(_ *runtime.RunContext, _ *runtime.ExecutionResult) (*runtime.ReviewResult, error) {
	return &runtime.ReviewResult{Approved: true, Summary: "ok"}, nil
}
