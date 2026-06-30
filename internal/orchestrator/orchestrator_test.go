package orchestrator

import (
	"context"
	"fmt"
	"os"
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

func TestMergeResults_RedactsSecrets(t *testing.T) {
	t.Parallel()

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(t.TempDir()),
		map[string]runtime.Agent{},
		&runtime.Config{},
	)

	merged := o.MergeResults([]SubtaskResult{
		{
			SubtaskID: "step-1",
			Output:    "created token ghp_123456789012345678901234567890123456",
			Error:     "Cookie: agentos_session=signed-session-value",
		},
	})
	for _, leaked := range []string{
		"ghp_123456789012345678901234567890123456",
		"signed-session-value",
	} {
		if strings.Contains(merged, leaked) {
			t.Fatalf("merged output leaked %q: %s", leaked, merged)
		}
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
	if !strings.Contains(plan.Subtasks[0].Description, "preserve established") || !strings.Contains(plan.Subtasks[0].Description, "internal/") {
		t.Fatalf("go-backend fallback description = %q, want convention-aware architecture guidance", plan.Subtasks[0].Description)
	}
	if plan.Subtasks[0].QualityGate.empty() {
		t.Fatalf("go-backend fallback subtask missing quality gate: %+v", plan.Subtasks[0])
	}
}

func TestPlan_IncludesAgentConventionGuidanceInPlannerPrompt(t *testing.T) {
	t.Parallel()

	mock := llm.NewMockLLMClient([]llm.ChatResponse{{
		Choices: []llm.Choice{{Message: llm.Message{
			Role:    llm.RoleAssistant,
			Content: `{"description":"test","subtasks":[{"id":"step-1","description":"implement server","agent_type":"go-backend","dependencies":[]}]}`,
		}}},
	}})
	o := NewOrchestrator(
		mock,
		sandbox.NewLocalSandbox(t.TempDir()),
		map[string]runtime.Agent{"go-backend": &recordingAgent{name: "go-backend"}},
		&runtime.Config{},
	)

	_, err := o.Plan(context.Background(), "add endpoint")
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(mock.Requests) != 1 {
		t.Fatalf("got %d LLM requests, want 1", len(mock.Requests))
	}
	userPrompt := mock.Requests[0].Messages[1].Content
	for _, want := range []string{"Architecture/conventions", "cmd/", "internal/", "Output expectations", "go vet ./..."} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("planner prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestPlan_EnrichesGeneratedSubtasksWithParentRequirements(t *testing.T) {
	t.Parallel()

	o := NewOrchestrator(
		llm.NewMockLLMClient([]llm.ChatResponse{{
			Choices: []llm.Choice{{Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: `{"description":"test","subtasks":[{"id":"step-1","description":"implement server","agent_type":"go-backend","dependencies":[]}]}`,
			}}},
		}}),
		sandbox.NewLocalSandbox(t.TempDir()),
		map[string]runtime.Agent{"go-backend": &recordingAgent{name: "go-backend"}},
		&runtime.Config{},
	)

	parentTask := `Create /healthz returning {"status":"ok"} and / using net/http.`
	plan, err := o.Plan(context.Background(), parentTask)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Subtasks) != 1 {
		t.Fatalf("got %d subtasks, want 1", len(plan.Subtasks))
	}
	description := plan.Subtasks[0].Description
	for _, want := range []string{"go.mod", "main.go", `{"status":"ok"}`, parentTask} {
		if !strings.Contains(description, want) {
			t.Fatalf("enriched description missing %q: %s", want, description)
		}
	}
	for _, want := range []string{"existing repository layout", "cmd/", "internal/"} {
		if !strings.Contains(description, want) {
			t.Fatalf("enriched description missing architecture guidance %q: %s", want, description)
		}
	}
	if plan.Subtasks[0].QualityGate == nil || len(plan.Subtasks[0].QualityGate.RequiredFiles) == 0 {
		t.Fatalf("generated subtask missing quality gate: %+v", plan.Subtasks[0])
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

func TestExecuteSequential_SkipsSubtaskWhenDependencyFails(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("AGENTOS_HOME", filepath.Join(t.TempDir(), "agentos-home"))

	agent := &recordingAgent{failTasks: map[string]bool{"step-1": true}}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"test-agent": agent},
		&runtime.Config{},
	)
	o.SetSubtaskTimeout(time.Minute)

	results, err := o.ExecuteWithObserver(context.Background(), &TaskPlan{
		Subtasks: []Subtask{
			{ID: "step-1", Description: "first", AgentName: "test-agent"},
			{ID: "step-2", Description: "second", AgentName: "test-agent", Deps: []string{"step-1"}},
		},
	}, nil)
	if err == nil {
		t.Fatal("ExecuteWithObserver() error = nil, want failure")
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[1].Success || !strings.Contains(results[1].Error, `dependency "step-1" failed`) {
		t.Fatalf("dependent result = %+v, want dependency failure", results[1])
	}
	agent.mu.Lock()
	defer agent.mu.Unlock()
	for _, id := range agent.executedTaskIDs {
		if id == "step-2" {
			t.Fatal("dependent subtask executed despite failed dependency")
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

	result := o.executeSubtask(context.Background(), &Subtask{
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

	result := o.executeSubtask(context.Background(), &Subtask{
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

func TestExecuteSubtask_FailsMissingRequiredFile(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("AGENTOS_HOME", filepath.Join(t.TempDir(), "agentos-home"))

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"test-agent": &recordingAgent{}},
		&runtime.Config{},
	)

	result := o.executeSubtask(context.Background(), &Subtask{
		ID:          "step-1",
		Description: "create required artifact",
		AgentName:   "test-agent",
		QualityGate: &QualityGate{RequiredFiles: []string{"required.txt"}},
	}, "")
	if result.Success {
		t.Fatalf("executeSubtask() succeeded, want quality gate failure: %+v", result)
	}
	if result.QualityGate == nil || result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want failed status", result.QualityGate)
	}
	if !strings.Contains(result.Error, "required.txt") {
		t.Fatalf("error = %q, want required file detail", result.Error)
	}
}

func TestValidateQualityGate_RequiredCommandAndContent(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("run go test ./...\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	status := validateQualityGate(context.Background(), repo, &QualityGate{
		RequiredFiles:      []string{"README.md"},
		ValidationCommands: []string{"test -f README.md"},
		ContentChecks: []QualityContentCheck{{
			File:     "README.md",
			Contains: []string{"go test ./..."},
		}},
	})
	if !status.Passed {
		t.Fatalf("quality gate failed: %+v", status)
	}
}

func TestValidateQualityGate_BlocksEscapingPath(t *testing.T) {
	status := validateQualityGate(context.Background(), t.TempDir(), &QualityGate{
		RequiredFiles: []string{"../secret.txt"},
	})
	if status.Passed {
		t.Fatalf("quality gate passed, want path escape failure: %+v", status)
	}
}

func TestRecoverGoBackend_CreatesValidService(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	out, err := recoverGoBackend(context.Background(), repo, "https://github.com/kazyamaz200/agentos-test.git create /healthz with net/http")
	if err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if !strings.Contains(out, "Go net/http service") {
		t.Fatalf("output = %q", out)
	}
	for _, file := range []string{"go.mod", "main.go"} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	mainData, err := os.ReadFile(filepath.Join(repo, "main.go"))
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	for _, want := range []string{"net/http", "healthzHandler", `"status": "ok"`} {
		if !strings.Contains(string(mainData), want) {
			t.Fatalf("main.go missing %q:\n%s", want, mainData)
		}
	}
}

func TestRecoverGoCI_CreatesWorkflowAndTests(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverGoBackend(context.Background(), repo, "create /healthz with net/http"); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	out, err := recoverGoCI(context.Background(), repo)
	if err != nil {
		t.Fatalf("recoverGoCI() error = %v", err)
	}
	if !strings.Contains(out, "GitHub Actions") {
		t.Fatalf("output = %q", out)
	}
	for _, file := range []string{"main_test.go", filepath.Join(".github", "workflows", "go.yml")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
}

func TestRecoverNoOpDocs_CreatesRequiredREADME(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("AGENTOS_HOME", filepath.Join(t.TempDir(), "agentos-home"))
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-step-2"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"docs": &recordingAgent{name: "docs"}},
		&runtime.Config{},
	)

	result, ok := o.recoverNoOpBuiltInSubtask(context.Background(), &Subtask{
		ID:          "step-2",
		AgentName:   "docs",
		Description: "Update README for /healthz using net/http and go test.",
	}, runSandbox)
	if !ok || !result.Success {
		t.Fatalf("recoverNoOpBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if !readmeCoversScenario(repo) {
		t.Fatalf("README.md does not cover scenario")
	}
}

func TestRecoverNoOpCIFixer_CreatesTestsAndWorkflow(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverGoBackend(context.Background(), repo, "create /healthz with net/http"); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-step-3"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"ci-fixer": &recordingAgent{name: "ci-fixer"}},
		&runtime.Config{},
	)

	result, ok := o.recoverNoOpBuiltInSubtask(context.Background(), &Subtask{
		ID:          "step-3",
		AgentName:   "ci-fixer",
		Description: "Add tests and GitHub Actions for /healthz using net/http and go test ./...",
	}, runSandbox)
	if !ok || !result.Success {
		t.Fatalf("recoverNoOpBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if !ciCoversScenario(repo) {
		t.Fatalf("CI files do not cover scenario")
	}
}

func TestRecoverBuiltInSubtask_UsesFreshContextAfterTimeout(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-step-1"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"go-backend": &recordingAgent{name: "go-backend"}},
		&runtime.Config{},
	)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	result, ok := o.recoverBuiltInSubtask(canceledCtx, &Subtask{
		ID:          "step-1",
		AgentName:   "go-backend",
		Description: "Create /healthz with net/http in https://github.com/kazyamaz200/agentos-test.git",
	}, runSandbox, context.DeadlineExceeded)
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success despite canceled subtask context", result, ok)
	}
	if _, err := os.Stat(filepath.Join(repo, "main.go")); err != nil {
		t.Fatalf("main.go not created: %v", err)
	}
}

func TestInferModulePath_ExtractsGitHubURLWithoutRegex(t *testing.T) {
	t.Parallel()

	got := inferModulePath("target repo is https://github.com/kazyamaz200/agentos-test.git and should expose /healthz", t.TempDir())
	if got != "github.com/kazyamaz200/agentos-test" {
		t.Fatalf("inferModulePath() = %q", got)
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
	name            string
	taskID          string
	taskRepo        string
	baseBranch      string
	profileName     string
	workspaceRoot   string
	failTasks       map[string]bool
	delay           time.Duration
	executedTaskIDs []string
	mu              sync.Mutex
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
	a.mu.Lock()
	a.executedTaskIDs = append(a.executedTaskIDs, ctx.Task.ID)
	a.mu.Unlock()
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
