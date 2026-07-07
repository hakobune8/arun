package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	agentpkg "github.com/hakobune8/arun/internal/agent"
	"github.com/hakobune8/arun/internal/llm"
	"github.com/hakobune8/arun/internal/runtime"
	"github.com/hakobune8/arun/internal/sandbox"
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
			Error:     "Cookie: arun_session=signed-session-value",
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

func TestFallbackPlan_RoutesSpecializedDomains(t *testing.T) {
	t.Parallel()

	agents := map[string]runtime.Agent{}
	for _, name := range []string{"go-backend", "frontend", "docs", "ci-fixer", "security", "release-manager", "dependency-updater", "qa", "docker", "helm", "kubernetes", "devops", "analyst", "reporter", "reviewer"} {
		agents[name] = agentpkg.NewBaseAgent(name, llm.NewMockLLMClient(nil))
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(t.TempDir()),
		agents,
		&runtime.Config{},
	)

	tests := []struct {
		name       string
		task       string
		wantAgents []string
	}{
		{"frontend", "Update React Tailwind responsive UI", []string{"frontend", "qa", "docs", "reviewer"}},
		{"docker", "Fix Dockerfile container deployment", []string{"devops", "docker", "helm", "kubernetes", "security", "qa", "docs", "reviewer"}},
		{"helm", "Fix Helm chart values for Kubernetes ingress", []string{"devops", "docker", "helm", "kubernetes", "security", "qa", "docs", "reviewer"}},
		{"investigation report", "Investigate the last 24 hours of run logs and create a repository health report", []string{"analyst", "reporter", "reviewer"}},
		{"backend", "Add Go API endpoint handler", []string{"go-backend", "docs", "ci-fixer", "reviewer"}},
		{"docs", "Update README documentation guide", []string{"docs", "reviewer"}},
		{"security", "Fix CVE authz permission issue", []string{"security", "qa", "docs", "reviewer"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := o.fallbackPlan(tt.task)
			var got []string
			for _, subtask := range plan.Subtasks {
				got = append(got, subtask.AgentName)
			}
			for _, want := range tt.wantAgents {
				if !containsAgent(got, want) {
					t.Fatalf("agents = %+v, want %q", got, want)
				}
			}
			if len(plan.Subtasks) > 0 && plan.Subtasks[len(plan.Subtasks)-1].AgentName != "reviewer" {
				t.Fatalf("last agent = %q, want reviewer", plan.Subtasks[len(plan.Subtasks)-1].AgentName)
			}
		})
	}
}

func containsAgent(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
		map[string]runtime.Agent{
			"go-backend":         &recordingAgent{name: "go-backend"},
			"security":           &recordingAgent{name: "security"},
			"release-manager":    &recordingAgent{name: "release-manager"},
			"dependency-updater": &recordingAgent{name: "dependency-updater"},
			"qa":                 &recordingAgent{name: "qa"},
			"analyst":            &recordingAgent{name: "analyst"},
			"reporter":           &recordingAgent{name: "reporter"},
		},
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
	for _, want := range []string{
		"Architecture/conventions",
		"cmd/",
		"internal/",
		"Output expectations",
		"go vet ./...",
		"security",
		"frontend",
		"package.json",
		"responsive",
		"release-manager",
		"dependency-updater",
		"qa",
		"analyst",
		"run records",
		"reporter",
		"requested output language",
	} {
		if !strings.Contains(userPrompt, want) {
			t.Fatalf("planner prompt missing %q:\n%s", want, userPrompt)
		}
	}
}

func TestApplyDefaultQualityGate_SpecializedBuiltIns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		agent string
		file  string
	}{
		{"security", "SECURITY.md"},
		{"frontend", ""},
		{"release-manager", ""},
		{"dependency-updater", filepath.Join("server", "go.mod")},
		{"qa", ""},
		{"docker", "Dockerfile"},
		{"helm", ""},
		{"kubernetes", ""},
		{"devops", ""},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			subtask := &Subtask{AgentName: tt.agent, Description: "exercise specialized agent"}
			applyDefaultQualityGate(subtask)
			if subtask.QualityGate == nil {
				t.Fatalf("%s missing quality gate", tt.agent)
			}
			if tt.file == "" && len(subtask.QualityGate.ValidationCommands) == 0 {
				t.Fatalf("%s validation commands are empty", tt.agent)
			}
			found := tt.file == ""
			for _, file := range subtask.QualityGate.RequiredFiles {
				if file == tt.file {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s required files = %+v, want %q", tt.agent, subtask.QualityGate.RequiredFiles, tt.file)
			}
		})
	}
}

func TestSubtaskProfile_ReportOnlyAgentsDoNotRequireGoValidation(t *testing.T) {
	t.Parallel()

	for _, agent := range []string{"analyst", "reporter", "reviewer", "release-manager"} {
		t.Run(agent, func(t *testing.T) {
			prof := subtaskProfile(agent)
			if prof.Commands.Test != "" {
				t.Fatalf("%s test command = %q, want empty", agent, prof.Commands.Test)
			}
			for _, tool := range prof.Tools.Allow {
				if tool == "test" {
					t.Fatalf("%s tools = %+v, want no test tool", agent, prof.Tools.Allow)
				}
			}
			if (agent == "analyst" || agent == "reporter") && !containsTestString(prof.Tools.Allow, "write_file") {
				t.Fatalf("%s tools = %+v, want write_file for planning/report artifacts", agent, prof.Tools.Allow)
			}
		})
	}
}

func TestSubtaskProfile_GoAgentsUseRepoAwareValidation(t *testing.T) {
	t.Parallel()

	for _, agent := range []string{"go-backend", "ci-fixer", "security", "dependency-updater"} {
		t.Run(agent, func(t *testing.T) {
			prof := subtaskProfile(agent)
			if prof.Commands.Test != goTestValidationCommand {
				t.Fatalf("%s test command = %q, want repo-aware go test command", agent, prof.Commands.Test)
			}
			if prof.Commands.Lint != goVetValidationCommand {
				t.Fatalf("%s lint command = %q, want repo-aware go vet command", agent, prof.Commands.Lint)
			}
		})
	}
	if prof := subtaskProfile("go-backend"); prof.Commands.Build != goBuildValidationCommand {
		t.Fatalf("go-backend build command = %q, want repo-aware go build command", prof.Commands.Build)
	}
}

func TestIsCanonicalGoServiceTask_AllowsHealthEndpointWording(t *testing.T) {
	t.Parallel()

	description := "Create a minimal Go net/http server with a health endpoint and tests."
	if !isCanonicalGoServiceTask(description) {
		t.Fatalf("isCanonicalGoServiceTask(%q) = false, want true", description)
	}
}

func containsTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestQAQualityGate_AllowsStaticFrontendSmokeEvidence(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "client", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"scripts":{"test":"node --check client/src/main.js","build":"node --check client/src/main.js"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "client", "src", "main.js"), []byte(`console.log("ok");`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "smoke-test.md"), []byte("# Smoke Test\n\nRun npm test.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	subtask := &Subtask{AgentName: "qa", Description: "Validate frontend smoke checks"}
	applyDefaultQualityGate(subtask)
	status := validateQualityGate(context.Background(), repo, subtask.QualityGate)
	if !status.Passed {
		t.Fatalf("quality gate failed: %+v", status)
	}

	prof := subtaskProfile("qa")
	if prof.Commands.Test != qaValidationCommand {
		t.Fatalf("qa test command = %q, want repo-aware command", prof.Commands.Test)
	}
}

func TestFrontendQualityGate_AllowsStaticEvidenceWithoutNodeRuntime(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("PATH-limited POSIX shell validation is covered on Unix runners")
	}
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"package.json":                         `{"scripts":{"test":"node --check src/main.js","build":"node --check src/main.js"}}`,
		"index.html":                           `<script type="module" src="./src/main.js"></script>`,
		filepath.Join("src", "main.js"):        `console.log("ok");`,
		filepath.Join("docs", "smoke-test.md"): "# Smoke Test\n\nRun npm test when Node.js is available.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	bin := t.TempDir()
	if err := os.Symlink("/bin/sh", filepath.Join(bin, "sh")); err != nil {
		t.Fatalf("symlink sh: %v", err)
	}
	t.Setenv("PATH", bin)

	for _, agent := range []string{"frontend", "qa"} {
		subtask := &Subtask{AgentName: agent, Description: "Validate static frontend assets"}
		applyDefaultQualityGate(subtask)
		status := validateQualityGate(context.Background(), repo, subtask.QualityGate)
		if !status.Passed {
			t.Fatalf("%s quality gate failed without node runtime: %+v", agent, status)
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
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))

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
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))

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
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))

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
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))

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
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))

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
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))

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
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))

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

func TestExecuteSubtask_FrontendFailsNoOp(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"scripts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)

	subtask := &Subtask{
		ID:          "step-1",
		Description: "Update responsive UI",
		AgentName:   "frontend",
	}
	applyDefaultQualityGate(subtask)

	result := o.executeSubtask(context.Background(), subtask, "")
	if result.Success {
		t.Fatalf("executeSubtask() succeeded, want frontend no-op failure: %+v", result)
	}
	if !strings.Contains(result.Error, "produced no diff") {
		t.Fatalf("error = %q, want no diff detail", result.Error)
	}
}

func TestExecuteSubtask_FrontendFixAllowsNoOpWhenGatePasses(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))
	if err := os.MkdirAll(filepath.Join(repo, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("client", "index.html"): `<!doctype html><html><head><title>App</title><link rel="stylesheet" href="style.css"></head><body><script src="app.js"></script></body></html>`,
		filepath.Join("client", "style.css"):  `body { margin: 0; }`,
		filepath.Join("client", "app.js"):     `console.log("ready");`,
		filepath.Join("server", "main.go"): `package main

import "net/http"

func main() {
	http.Handle("/", http.FileServer(http.Dir("client")))
}
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)

	subtask := &Subtask{
		ID:          "sprint-1-frontend-fix",
		Description: "Sprint 1 remediation: address QA findings that require frontend or static asset changes.",
		AgentName:   "frontend",
	}
	applyDefaultQualityGate(subtask)

	result := o.executeSubtask(context.Background(), subtask, "")
	if !result.Success {
		t.Fatalf("executeSubtask() failed for no-op frontend fix: %+v", result)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
	if !strings.Contains(result.Output, "No frontend changes were required") {
		t.Fatalf("output = %q, want no-op success note", result.Output)
	}
}

func TestExecuteSubtask_FrontendRecoversEmptyRepositoryNoOp(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))
	if err := runCmd(context.Background(), repo, "git", "init"); err != nil {
		t.Fatalf("git init: %v", err)
	}

	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)

	subtask := &Subtask{
		ID:          "step-1",
		Description: "Run an implementation-heavy agile scrum workflow for hakobune8/invaders on main.",
		AgentName:   "frontend",
	}
	applyDefaultQualityGate(subtask)

	result := o.executeSubtask(context.Background(), subtask, "")
	if !result.Success {
		t.Fatalf("executeSubtask() failed: %s", result.Error)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
	if !strings.Contains(result.Output, "static frontend scaffold") {
		t.Fatalf("output = %q, want frontend fallback detail", result.Output)
	}
	for _, file := range []string{filepath.Join("client", "package.json"), filepath.Join("client", "index.html"), filepath.Join("client", "styles.css"), filepath.Join("client", "src", "main.js"), "README.md", filepath.Join("docs", "smoke-test.md"), filepath.Join("docs", "testing.md"), "CHANGELOG.md"} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	if !strings.Contains(result.Diff, "index.html") {
		t.Fatalf("diff missing scaffold files:\n%s", result.Diff)
	}
}

func TestRecoverFrontendStaticAppUsesInvaderProductConceptTitle(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverFrontendStaticApp(repo, "新規性のあるインベーダーゲームを作成する"); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	index, err := os.ReadFile(filepath.Join(repo, "client", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "<title>One-Button Invaders</title>") ||
		!strings.Contains(string(index), `<h1 id="app-title">One-Button Invaders</h1>`) {
		t.Fatalf("index.html does not use invader product concept title:\n%s", index)
	}
	pkg, err := os.ReadFile(filepath.Join(repo, "client", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pkg), `"name": "one-button-invaders"`) {
		t.Fatalf("client/package.json does not use invader product package name:\n%s", pkg)
	}
	if strings.Contains(string(pkg), "client/src/main.js") {
		t.Fatalf("client/package.json should use client-relative script paths:\n%s", pkg)
	}
	if _, err := os.Stat(filepath.Join(repo, "package.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("root package.json exists after frontend recovery: %v", err)
	}
	mainJS, err := os.ReadFile(filepath.Join(repo, "client", "src", "main.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainJS), `gravity: "floor"`) ||
		!strings.Contains(string(mainJS), "flipGravity") ||
		!strings.Contains(string(mainJS), "laneMatched") {
		t.Fatalf("main.js does not implement gravity-lane mechanic:\n%s", mainJS)
	}
	brief, err := os.ReadFile(filepath.Join(repo, "docs", "product-brief.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(brief), "# Product Brief: One-Button Invaders") ||
		!strings.Contains(string(brief), "gravity-lane flip mechanic") {
		t.Fatalf("product brief does not describe generated product concept:\n%s", brief)
	}
}

func TestRepairGeneratedFrontendPackageLayoutMovesRootPackage(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "client", "index.html"), []byte("<!doctype html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"scripts":{"test":"node --check client/src/main.js"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repairGeneratedFrontendPackageLayout(repo); err != nil {
		t.Fatalf("repairGeneratedFrontendPackageLayout() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "package.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("root package.json still exists: %v", err)
	}
	pkg, err := os.ReadFile(filepath.Join(repo, "client", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pkg), "node --check src/main.js") || strings.Contains(string(pkg), "client/src/main.js") {
		t.Fatalf("client/package.json scripts not rewritten:\n%s", pkg)
	}
}

func TestRecoverFrontendDocsKeepsExistingStaticAppTitle(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverFrontendStaticApp(repo, "新規性のあるインベーダーゲームを作成する"); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	description := "Sprint 3 documentation: update README and docs with Kubernetes deploy notes. README H1 must be the product name or repository name, not a deployment topic such as Kubernetes."
	if _, err := recoverFrontendDocs(repo, description); err != nil {
		t.Fatalf("recoverFrontendDocs() error = %v", err)
	}
	readme, err := os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(readme), "# One-Button Invaders\n") {
		t.Fatalf("README title drifted from existing app title:\n%s", readme)
	}
	brief, err := os.ReadFile(filepath.Join(repo, "docs", "product-brief.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(brief), "# Product Brief: One-Button Invaders") {
		t.Fatalf("product brief title drifted from existing app title:\n%s", brief)
	}
	if strings.Contains(string(brief), "Sprint 3 documentation") || strings.Contains(string(brief), "## Source Request") {
		t.Fatalf("product brief includes copied task prompt text:\n%s", brief)
	}
}

func TestRecoverBuiltInSubtask_StaticFrontendFallbackGatePasses(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-step-1"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)
	subtask := &Subtask{
		ID:          "step-1",
		AgentName:   "frontend",
		Description: "For a completely empty repository, build Empty Invaders.",
	}
	applyDefaultQualityGate(subtask)

	result, ok := o.recoverBuiltInSubtask(context.Background(), subtask, runSandbox, errors.New("subtask timed out after 5m"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want fallback pass", result.QualityGate)
	}
}

func TestFrontendQualityGateFailsForUnservedFrontendIndex(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "frontend"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("frontend", "index.html"): "<!doctype html><title>Alternate</title>",
		"main.go": `package main

import "net/http"

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("placeholder"))
	})
}
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with unserved frontend/index.html: %+v", status)
	}
}

func TestFrontendQualityGateFailsForUnservedWebIndex(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("web", "index.html"): "<!doctype html><title>Alternate</title>",
		"main.go": `package main

import "net/http"

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("placeholder"))
	})
}
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with unserved web/index.html: %+v", status)
	}
}

func TestFrontendQualityGateFailsForUnservedAlternateIndexWithStaticAssetPath(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("web", "index.html"): "<!doctype html><title>Gravity Invaders</title>",
		"main.go": `package main

import "net/http"

func staticAssetPath(urlPath string) (string, bool) {
	return "styles.css", true
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if assetPath, ok := staticAssetPath(r.URL.Path); ok {
				http.ServeFile(w, r, assetPath)
				return
			}
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})
}
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with unserved web/index.html and staticAssetPath: %+v", status)
	}
}

func TestFrontendQualityGateFailsForUnservedRootAssets(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":                    `<link rel="stylesheet" href="./styles.css"><script type="module" src="./src/main.js"></script>`,
		"styles.css":                    `body { color: white; }`,
		filepath.Join("src", "main.js"): `console.log("ok");`,
		"main.go": `package main

import "net/http"

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})
}
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with unserved root CSS/JS assets: %+v", status)
	}
}

func TestFrontendQualityGateFailsForMissingClientReferencedAssets(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("client", "index.html"): `<link rel="stylesheet" href="style.css"><script src="game.js"></script>`,
		filepath.Join("server", "main.go"): `package main

import (
	"net/http"
	"path"
)

func staticAssetPath(urlPath string) (string, bool) {
	clean := path.Clean("/" + urlPath)
	switch clean {
	case "/styles.css":
		return "client/styles.css", true
	default:
		return "", false
	}
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if assetPath, ok := staticAssetPath(r.URL.Path); ok {
				http.ServeFile(w, r, assetPath)
				return
			}
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "client/index.html")
	})
}
`,
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with missing client CSS/JS assets: %+v", status)
	}
}

func TestFrontendQualityGateFailsForMissingAbsoluteClientReferencedAssets(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		filepath.Join("client", "index.html"): `<!doctype html><link rel="stylesheet" href="/style.css"><script src="/app.js"></script>`,
		filepath.Join("server", "main.go"): `package main

import (
	"net/http"
	"path"
)

func staticAssetPath(urlPath string) (string, bool) {
	clean := path.Clean("/" + urlPath)
	switch clean {
	case "/styles.css":
		return "client/styles.css", true
	case "/src/main.js":
		return "client/src/main.js", true
	default:
		return "", false
	}
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if assetPath, ok := staticAssetPath(r.URL.Path); ok {
				http.ServeFile(w, r, assetPath)
				return
			}
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "client/index.html")
	})
}
`,
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with missing absolute client CSS/JS assets: %+v", status)
	}
}

func TestFrontendQualityGateFailsWhenStaticAssetPathDoesNotServeClientRefs(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		filepath.Join("client", "index.html"): `<link rel="stylesheet" href="style.css"><script src="game.js"></script>`,
		filepath.Join("client", "style.css"):  `body { color: white; }`,
		filepath.Join("client", "game.js"):    `console.log("ok");`,
		filepath.Join("server", "main.go"): `package main

import (
	"net/http"
	"path"
)

func staticAssetPath(urlPath string) (string, bool) {
	clean := path.Clean("/" + urlPath)
	switch clean {
	case "/styles.css":
		return "client/styles.css", true
	default:
		return "", false
	}
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if assetPath, ok := staticAssetPath(r.URL.Path); ok {
				http.ServeFile(w, r, assetPath)
				return
			}
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "client/index.html")
	})
}
`,
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed when staticAssetPath did not serve referenced client assets: %+v", status)
	}
}

func TestFrontendQualityGateFailsWhenServerWorkingDirCannotServeClientAssets(t *testing.T) {
	t.Parallel()
	if goruntime.GOOS == "windows" {
		t.Skip("runtime server smoke is skipped on Windows to avoid leaked process directory locks")
	}

	repo := t.TempDir()
	files := map[string]string{
		filepath.Join("server", "go.mod"):         "module github.com/hakobune8/arun-test/server\n\ngo 1.22\n",
		filepath.Join("client", "index.html"):     `<!doctype html><link rel="stylesheet" href="./styles.css"><script src="./src/main.js"></script>`,
		filepath.Join("client", "styles.css"):     "body { color: white; }",
		filepath.Join("client", "src", "main.js"): "console.log('ok');",
		filepath.Join("server", "main.go"): `package main

import (
	"log"
	"net/http"
	"os"
	"path"
	"strings"
)

const staticDir = "client"

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		if assetPath, ok := staticAssetPath(r.URL.Path); ok {
			http.ServeFile(w, r, assetPath)
			return
		}
		http.NotFound(w, r)
		return
	}
	if _, err := os.Stat(path.Join(staticDir, "index.html")); err == nil {
		http.ServeFile(w, r, path.Join(staticDir, "index.html"))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func staticAssetPath(urlPath string) (string, bool) {
	clean := path.Clean("/" + urlPath)
	switch {
	case clean == "/styles.css":
		return path.Join(staticDir, "styles.css"), true
	case strings.HasPrefix(clean, "/src/") && strings.HasSuffix(clean, ".js"):
		return path.Join(staticDir, strings.TrimPrefix(clean, "/")), true
	default:
		return "", false
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", rootHandler)
	log.Fatal(http.ListenAndServe(":8080", mux))
}
`,
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed when cd server runtime cannot serve client assets: %+v", status)
	}
}

func TestFrontendQualityGateFailsForRootPackageInSeparatedGeneratedLayout(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		"package.json":                            `{"scripts":{"test":"node --check client/src/main.js"}}`,
		filepath.Join("server", "go.mod"):         "module github.com/hakobune8/arun-test/server\n\ngo 1.22\n",
		filepath.Join("server", "main.go"):        frontendServingGoMain(),
		filepath.Join("client", "index.html"):     `<!doctype html><link rel="stylesheet" href="./styles.css"><script src="./src/main.js"></script>`,
		filepath.Join("client", "styles.css"):     "body { color: white; }",
		filepath.Join("client", "src", "main.js"): "console.log('ok');",
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with root package.json in separated layout: %+v", status)
	}
}

func TestFrontendQualityGateFailsForProductConceptDrift(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		"README.md": "# Chrono Invaders\n\nTime reversal arcade game.\n",
		filepath.Join("docs", "product-brief.md"): "# Gravity Invaders - Product Brief\n\nGravity mechanic.\n",
		filepath.Join("client", "index.html"):     `<!doctype html><title>Gravity Invaders - 重力インベーダー</title><link rel="stylesheet" href="styles.css"><script src="src/main.js"></script>`,
		filepath.Join("client", "styles.css"):     `body { color: white; }`,
		filepath.Join("client", "src", "main.js"): `console.log("gravity invaders");`,
		filepath.Join("server", "main.go"): `package main

import "net/http"

func main() {
	http.Handle("/", http.FileServer(http.Dir("client")))
}
`,
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with product concept drift: %+v", status)
	}
}

func TestFrontendQualityGateFailsForUnreferencedGeneratedClientAsset(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		"README.md": "# One-Button Invaders\n",
		filepath.Join("docs", "product-brief.md"):     "# One-Button Invaders\n",
		filepath.Join("docs", "artifact-contract.md"): "# Contract\n\n## Primary route\n/\n\n## Frontend\n- Entrypoint: `client/index.html`.\n- Required local assets: `client/styles.css` and `client/src/main.js`.\n\n## Backend\n- Go module: `github.com/hakobune8/arun-test/server`.\n\n## Validation\n- `npm --prefix client test`.\n",
		filepath.Join("client", "index.html"):         `<!doctype html><title>One-Button Invaders</title><link rel="stylesheet" href="./styles.css"><script src="./src/main.js"></script>`,
		filepath.Join("client", "styles.css"):         `body { color: white; }`,
		filepath.Join("client", "style.css"):          `.unused { color: red; }`,
		filepath.Join("client", "src", "main.js"):     `console.log("ok");`,
		filepath.Join("client", "package.json"):       `{"scripts":{"test":"node --check src/main.js","build":"node --check src/main.js"}}`,
		filepath.Join("server", "main.go"): `package main

import "net/http"

func main() {
	http.Handle("/", http.FileServer(http.Dir("../client")))
}
`,
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with unreferenced generated asset: %+v", status)
	}
}

func TestUnservedRootFrontendAssetsExist(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":                    `<link rel="stylesheet" href="./styles.css"><script type="module" src="./src/main.js"></script>`,
		"styles.css":                    `body { color: white; }`,
		filepath.Join("src", "main.js"): `console.log("ok");`,
		"main.go": `package main

import "net/http"

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})
}
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if !unservedRootFrontendAssetsExist(repo) {
		t.Fatalf("unservedRootFrontendAssetsExist() = false, want true")
	}
}

func TestCleanupGeneratedArtifactHygieneRemovesUnservedAlternateUIAndIncompleteCharts(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "charts", "invader-game"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "charts", "orphan-chart", "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":                       "<!doctype html><title>One-Button Invaders</title>",
		"styles.css":                       "body { color: white; }",
		filepath.Join("src", "main.js"):    `console.log("ok");`,
		filepath.Join("web", "index.html"): "<!doctype html><title>Gravity Invaders</title>",
		filepath.Join("charts", "invader-game", "Chart.yaml"):                "apiVersion: v2\nname: invader-game\ntype: application\nversion: 0.1.0\n",
		filepath.Join("charts", "orphan-chart", "templates", "service.yaml"): "apiVersion: v1\nkind: Service\n",
		"main.go": `package main

import "net/http"

func staticAssetPath(urlPath string) (string, bool) {
	return "styles.css", true
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			if assetPath, ok := staticAssetPath(r.URL.Path); ok {
				http.ServeFile(w, r, assetPath)
				return
			}
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})
}
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	for _, removed := range []string{
		filepath.Join("web", "index.html"),
		filepath.Join("charts", "invader-game", "Chart.yaml"),
		filepath.Join("charts", "orphan-chart", "templates", "service.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(repo, removed)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still exists or unexpected error: %v", removed, err)
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "client", "index.html")); err != nil {
		t.Fatalf("client index missing after migration: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "server", "main.go")); err != nil {
		t.Fatalf("server main missing after migration: %v", err)
	}
}

func TestGeneratedArtifactHygieneFailsForEmptyGeneratedFiles(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	emptyChart := filepath.Join(repo, "charts", "invader-game")
	if err := os.MkdirAll(filepath.Join(emptyChart, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{
		filepath.Join(emptyChart, "Chart.yaml"),
		filepath.Join(emptyChart, "values.yaml"),
		filepath.Join(emptyChart, "templates", "deployment.yaml"),
		filepath.Join(emptyChart, "templates", "service.yaml"),
	} {
		if err := os.WriteFile(file, nil, 0o600); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}

	status := validateQualityGate(context.Background(), repo, &QualityGate{
		ValidationCommands: []string{generatedArtifactHygieneValidationCommand},
	})
	if status.Passed {
		t.Fatalf("quality gate passed with empty generated chart files: %+v", status)
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	if _, err := os.Stat(emptyChart); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty chart directory still exists or unexpected error: %v", err)
	}
}

func TestGeneratedArtifactHygieneRepairsPlaceholderHelmChartNames(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if commandAvailable("git") {
		if err := runCmd(context.Background(), repo, "git", "init"); err != nil {
			t.Fatalf("git init: %v", err)
		}
		if err := runCmd(context.Background(), repo, "git", "remote", "add", "origin", "git@github.com:hakobune8/arun-test.git"); err != nil {
			t.Fatalf("git remote add: %v", err)
		}
	}
	chartDir := filepath.Join(repo, "charts", "name")
	if err := os.MkdirAll(filepath.Join(chartDir, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(chartDir, "Chart.yaml"):  "apiVersion: v2\nname: name\ntype: application\nversion: 0.1.0\n",
		filepath.Join(chartDir, "values.yaml"): "image:\n  repository: ghcr.io/example/app\n  tag: latest\n",
		filepath.Join(chartDir, "templates", "deployment.yaml"): `apiVersion: apps/v1
kind: Deployment
metadata:
  name: placeholder
`,
		filepath.Join(chartDir, "templates", "service.yaml"): `apiVersion: v1
kind: Service
metadata:
  name: placeholder
`,
		filepath.Join(repo, "k8s", "name", "README.md"):     "Render with Helm.\n",
		filepath.Join(repo, "docs", "kubernetes-deploy.md"): "# Kubernetes Deploy\n\nUse `helm upgrade --install name charts/name` after setting an image tag.\n",
	}
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, &QualityGate{
		ValidationCommands: []string{generatedArtifactHygieneValidationCommand},
	})
	if status.Passed {
		t.Fatalf("quality gate passed with placeholder Helm chart name: %+v", status)
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	for _, file := range []string{
		filepath.Join("charts", "arun-test", "Chart.yaml"),
		filepath.Join("charts", "arun-test", "values.yaml"),
		filepath.Join("k8s", "arun-test", "README.md"),
	} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not repaired: %v", file, err)
		}
	}
	chart, err := os.ReadFile(filepath.Join(repo, "charts", "arun-test", "Chart.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chart), "name: arun-test") {
		t.Fatalf("chart name not repaired:\n%s", chart)
	}
	values, err := os.ReadFile(filepath.Join(repo, "charts", "arun-test", "values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(values), "ghcr.io/hakobune8/arun-test") {
		t.Fatalf("image repository not repaired:\n%s", values)
	}
	doc, err := os.ReadFile(filepath.Join(repo, "docs", "kubernetes-deploy.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(doc), "charts/name") || !strings.Contains(string(doc), "charts/arun-test") {
		t.Fatalf("deploy doc not repaired:\n%s", doc)
	}
	status = validateQualityGate(context.Background(), repo, &QualityGate{
		ValidationCommands: []string{generatedArtifactHygieneValidationCommand},
	})
	if !status.Passed {
		t.Fatalf("quality gate failed after placeholder repair: %+v", status)
	}
}

func TestGeneratedArtifactHygieneRemovesDuplicateCIWorkflowName(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	workflowDir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "client", "package.json"), []byte(`{"scripts":{"test":"node --check src/main.js","build":"node --check src/main.js"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte("name: CI\non: [push]\njobs: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	goWorkflow := `name: CI
on: [push]
jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - run: go test ./...
      - run: go vet ./...
      - run: npm --prefix client test
      - run: npm --prefix client run build
`
	if err := os.WriteFile(filepath.Join(workflowDir, "go.yml"), []byte(goWorkflow), 0o600); err != nil {
		t.Fatal(err)
	}

	status := validateQualityGate(context.Background(), repo, &QualityGate{
		ValidationCommands: []string{generatedArtifactHygieneValidationCommand},
	})
	if status.Passed {
		t.Fatalf("quality gate passed with duplicate CI workflow name: %+v", status)
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workflowDir, "ci.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("duplicate ci.yml still exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workflowDir, "go.yml")); err != nil {
		t.Fatalf("go.yml was removed unexpectedly: %v", err)
	}
}

func TestGeneratedArtifactHygieneRemovesStrayRootHelmChartMetadata(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		filepath.Join("charts", "Chart.yaml"):               "apiVersion: v2\nname: arun-test\ntype: application\nversion: 0.1.0\n",
		filepath.Join("charts", "values.yaml"):              "replicaCount: 1\nimage:\n  repository: arun-test\n  tag: latest\n",
		filepath.Join("charts", "arun-test", "Chart.yaml"):  "apiVersion: v2\nname: arun-test\ntype: application\nversion: 0.1.0\n",
		filepath.Join("charts", "arun-test", "values.yaml"): "replicaCount: 1\n",
		filepath.Join("charts", "arun-test", "templates", "deployment.yaml"): `apiVersion: apps/v1
kind: Deployment
metadata:
  name: arun-test
spec:
  selector:
    matchLabels:
      app: arun-test
  template:
    metadata:
      labels:
        app: arun-test
    spec:
      containers:
        - name: arun-test
          image: nginx
`,
		filepath.Join("charts", "arun-test", "templates", "service.yaml"): `apiVersion: v1
kind: Service
metadata:
  name: arun-test
spec:
  selector:
    app: arun-test
  ports:
    - port: 80
`,
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, &QualityGate{
		ValidationCommands: []string{generatedArtifactHygieneValidationCommand},
	})
	if status.Passed {
		t.Fatalf("quality gate passed with stray root chart metadata: %+v", status)
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "charts", "Chart.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stray charts/Chart.yaml still exists or unexpected error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "charts", "values.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stray charts/values.yaml still exists or unexpected error: %v", err)
	}
}

func TestDocsQualityGateFailsForDeferredRemediationNotes(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "remediation_notes.md"), []byte("実装は次の human-led sprint で実施します。\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "docs",
		Description: "Sprint 3 documentation for generated product.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with deferred remediation notes: %+v", status)
	}
}

func TestDocsQualityGateFailsForSprintRemediationPlanOnlyNotes(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Sprint 2 調整計画: 修正ノート\n\n本ドキュメントは計画段階のものであり、コード実装は含まれません。\n\n`.github/workflows/ci.yml` を更新します。\n"
	if err := os.WriteFile(filepath.Join(repo, "docs", "sprint-2-remediation-notes.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "docs",
		Description: "Sprint 3 documentation for generated product.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with plan-only sprint remediation notes: %+v", status)
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "sprint-2-remediation-notes.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plan-only remediation notes still exist or unexpected error: %v", err)
	}
}

func TestDocsQualityGateFailsForMissingEndpointAndLayoutClaims(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"main.go": `package main

import "net/http"

func main() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {})
}
`,
		filepath.Join("docs", "sprint-report.md"): "Backend: `cmd/`, Frontend: `web/`, and the /health endpoint are ready.\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "docs",
		Description: "Sprint 3 documentation for generated product.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with missing endpoint/layout claims: %+v", status)
	}
}

func TestDocsQualityGateAllowsExistingEndpointAndLayoutClaims(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	for _, dir := range []string{"cmd", "web", "docs"} {
		if err := os.Mkdir(filepath.Join(repo, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"main.go": `package main

import "net/http"

func main() {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {})
}
`,
		filepath.Join("docs", "sprint-report.md"): "Backend: `cmd/`, Frontend: `web/`, and the /health endpoint are ready.\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "docs",
		Description: "Sprint 3 documentation for generated product.",
	}))
	if !status.Passed {
		t.Fatalf("quality gate failed with matching endpoint/layout claims: %+v", status)
	}
}

func TestCleanupGeneratedArtifactHygieneRemovesGeneratedDocsWithInvalidClaims(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"main.go": `package main

import "net/http"

func main() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {})
}
`,
		filepath.Join("docs", "sprint-3-report.md"): "Backend: `cmd/`, Frontend: `web/`, and the /health endpoint are ready.\n",
		filepath.Join("docs", "product-brief.md"):   "The product brief is still useful and should remain.\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "sprint-3-report.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sprint report stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "product-brief.md")); err != nil {
		t.Fatalf("product brief removed unexpectedly: %v", err)
	}
}

func TestCleanupGeneratedArtifactHygieneRemovesDuplicateRootProductBrief(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("docs", "product-brief.md"): "# Product Brief: One-Button Invaders\n\nGravity-lane flip mechanic.\n",
		"product-brief.md":                        "# Star Hopper - 新規性インベーダーゲーム\n\nPhase shift mechanic.\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "product-brief.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("root product-brief.md stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "product-brief.md")); err != nil {
		t.Fatalf("canonical docs/product-brief.md removed unexpectedly: %v", err)
	}
}

func TestCleanupGeneratedArtifactHygieneRemovesCaseVariantDocsProductBrief(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("docs", "product-brief.md"): "# Product Brief: One-Button Invaders\n\nGravity-lane flip mechanic.\n",
		filepath.Join("docs", "PRODUCT_BRIEF.md"): "# Gravity Invaders\n\nGravity field chain-collision mechanic.\n",
		filepath.Join("docs", "testing.md"):       "# Testing\n\nRelevant testing documentation.\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if err := cleanupGeneratedArtifactHygiene(repo); err != nil {
		t.Fatalf("cleanupGeneratedArtifactHygiene() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "PRODUCT_BRIEF.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("docs/PRODUCT_BRIEF.md stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "product-brief.md")); err != nil {
		t.Fatalf("canonical docs/product-brief.md removed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "testing.md")); err != nil {
		t.Fatalf("unrelated docs/testing.md removed unexpectedly: %v", err)
	}
}

func TestRecoverBuiltInSubtask_DocsQualityGateRecoveryRemovesInvalidReport(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverFrontendStaticApp(repo, "新規性のあるインベーダーゲームを作成する"); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "sprint-3-report.md"), []byte("Backend: `cmd/`, Frontend: `web/`, and the /health endpoint are ready.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-docs-recovery"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"docs": &recordingAgent{name: "docs"}},
		&runtime.Config{},
	)
	subtask := &Subtask{
		ID:          "sprint-3-docs",
		AgentName:   "docs",
		Description: "Sprint 3 documentation for generated product.",
	}
	applyDefaultQualityGate(subtask)

	failedStatus := validateQualityGate(context.Background(), repo, subtask.QualityGate)
	if failedStatus.Passed {
		t.Fatalf("pre-recovery quality gate passed unexpectedly: %+v", failedStatus)
	}
	result, ok := o.recoverNoOpBuiltInSubtaskWithStatus(context.Background(), subtask, runSandbox, failedStatus)
	if !ok || !result.Success {
		t.Fatalf("recoverNoOpBuiltInSubtaskWithStatus() = (%+v, %v), want success", result, ok)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed after cleanup recovery", result.QualityGate)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "sprint-3-report.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid sprint report stat err = %v, want not exist", err)
	}
}

func TestRecoverBuiltInSubtask_FrontendTimeoutRecoversEmptyRepository(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "run.log"), []byte("runtime started\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-step-1"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)

	result, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:          "step-1",
		AgentName:   "frontend",
		Description: "For an empty repository, create an initial minimal app scaffold.",
	}, runSandbox, errors.New("subtask timed out after 5m"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	for _, file := range []string{filepath.Join("client", "package.json"), filepath.Join("client", "index.html"), filepath.Join("client", "src", "main.js"), filepath.Join("docs", "smoke-test.md")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
}

func TestRecoverBuiltInSubtask_FrontendValidationRecoversGoService(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		"go.mod": `module github.com/hakobune8/arun-test

go 1.22
`,
		"main.go": `package main

import (
	"encoding/json"
	"net/http"
)

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	http.HandleFunc("/healthz", healthzHandler)
	_ = http.ListenAndServe(":8080", nil)
}
`,
		"main_test.go": `package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	healthzHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}
`,
		"README.md": "# ARUN Test\n\nGenerated Go service.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-frontend-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)

	result, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:        "sprint-1-frontend",
		AgentName: "frontend",
		Description: `Sprint 1 coding: create or connect a minimal user-facing frontend/static experience.

新規 repository の target baseline:
- Health endpoint と小さな product API または static asset handler を持つ minimal Go HTTP server。
- Repository goal に合う場合の minimal Web UI または static frontend slice。`,
	}, runSandbox, errors.New("validation failed after 3 retries: tests"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	for _, file := range []string{filepath.Join("client", "package.json"), filepath.Join("client", "index.html"), filepath.Join("client", "styles.css"), filepath.Join("client", "src", "main.js"), filepath.Join("docs", "smoke-test.md")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
}

func TestRecoverBuiltInSubtask_FrontendValidationRecoversMissingReferencedAssets(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module github.com/hakobune8/arun-test\n\ngo 1.22\n",
		filepath.Join("client", "index.html"): `<!doctype html>
<title>Gravity Invader</title>
<link rel="stylesheet" href="style.css">
<script src="app.js"></script>`,
		filepath.Join("server", "main.go"): `package main

import (
	"net/http"
	"path"
)

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		if assetPath, ok := staticAssetPath(r.URL.Path); ok {
			http.ServeFile(w, r, assetPath)
			return
		}
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, path.Join("client", "index.html"))
}

func staticAssetPath(urlPath string) (string, bool) {
	switch urlPath {
	case "/styles.css":
		return path.Join("client", "styles.css"), true
	default:
		return "", false
	}
}

func main() {}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-missing-assets-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)

	result, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:          "sprint-1-frontend",
		AgentName:   "frontend",
		Description: "Sprint 1 coding: create or connect a minimal user-facing frontend/static experience.",
		QualityGate: qualityGateForSubtask(&Subtask{
			AgentName:   "frontend",
			Description: "Add a browser game UI.",
		}),
	}, runSandbox, errors.New("validation failed after 3 retries: tests"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	for _, file := range []string{filepath.Join("client", "styles.css"), filepath.Join("client", "src", "main.js"), filepath.Join("client", "package.json")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not recovered: %v", file, err)
		}
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
}

func TestRecoverBuiltInSubtask_FrontendValidationRecoversUnservedRootAssets(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":                        "module github.com/hakobune8/arun-test\n\ngo 1.22\n",
		"index.html":                    `<link rel="stylesheet" href="./styles.css"><script type="module" src="./src/main.js"></script>`,
		"styles.css":                    `body { color: white; }`,
		filepath.Join("src", "main.js"): `console.log("ok");`,
		"main.go": `package main

import "net/http"

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-root-assets-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)
	subtask := &Subtask{
		ID:          "sprint-1-frontend",
		AgentName:   "frontend",
		Description: "Sprint 1 coding: create or connect a minimal user-facing frontend/static experience.",
	}
	applyDefaultQualityGate(subtask)

	result, ok := o.recoverBuiltInSubtask(context.Background(), subtask, runSandbox, errors.New("validation failed after 3 retries: index.html references styles.css but server/main.go does not serve static assets"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
	mainGo, err := os.ReadFile(filepath.Join(repo, "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainGo), "staticAssetPath") {
		t.Fatalf("server/main.go did not gain static asset serving:\n%s", mainGo)
	}
}

func TestRecoverBuiltInSubtask_FrontendValidationRecoversUnservedClientAssets(t *testing.T) {
	t.Parallel()
	if !commandAvailable("go") {
		t.Skip("go toolchain unavailable")
	}

	repo := t.TempDir()
	files := map[string]string{
		filepath.Join("docs", "artifact-contract.md"): "# Contract\n\n## Primary route\n/\n\n## Frontend\nclient/\n\n## Backend\nserver/\n\n## Validation\ngo test\n",
		filepath.Join("client", "index.html"):         `<!doctype html><title>Phase Invaders</title><link rel="stylesheet" href="style.css"><script src="app.js"></script>`,
		filepath.Join("client", "style.css"):          `body { color: black; }`,
		filepath.Join("client", "app.js"):             `console.log("phase");`,
		filepath.Join("server", "go.mod"):             "module github.com/hakobune8/arun-test/server\n\ngo 1.22\n",
		filepath.Join("server", "main.go"):            frontendServingGoMain(),
	}
	for name, content := range files {
		full := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if !unservedClientFrontendAssetsExist(repo) {
		t.Fatalf("unservedClientFrontendAssetsExist() = false, want true")
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-client-assets-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)
	subtask := &Subtask{
		ID:          "sprint-1-frontend",
		AgentName:   "frontend",
		Description: "Sprint 1 coding: create or connect a minimal user-facing frontend/static experience.",
	}
	applyDefaultQualityGate(subtask)

	result, ok := o.recoverBuiltInSubtask(context.Background(), subtask, runSandbox, errors.New("validation failed after 3 retries: tests"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
	for _, removed := range []string{filepath.Join("client", "style.css"), filepath.Join("client", "app.js")} {
		if _, err := os.Stat(filepath.Join(repo, removed)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s still exists or unexpected error: %v", removed, err)
		}
	}
}

func TestRecoverGoBackendResetsBrokenGeneratedGoInEmptyRepo(t *testing.T) {
	t.Parallel()
	if !commandAvailable("go") {
		t.Skip("go toolchain unavailable")
	}

	repo := t.TempDir()
	if err := runCmd(context.Background(), repo, "git", "init", "-b", "main"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "cmd", "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module arun-local-repo\n\ngo 1.22\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := `package main

func main() {
	_ = context.Background()
}
`
	if err := os.WriteFile(filepath.Join(repo, "cmd", "server", "main.go"), []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := recoverGoBackend(context.Background(), repo, "For a completely empty repository, create a minimal Go net/http service with /healthz and go test ./...")
	if err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if !strings.Contains(out, "Created minimal Go net/http service") {
		t.Fatalf("output = %q, want recovery summary", out)
	}
	if fileExists(filepath.Join(repo, "cmd", "server", "main.go")) {
		t.Fatalf("broken generated Go file still exists")
	}
	if err := runShell(context.Background(), filepath.Join(repo, "server"), "go test ./..."); err != nil {
		t.Fatalf("go test after recovery: %v", err)
	}
	if err := runShell(context.Background(), filepath.Join(repo, "server"), "go vet ./..."); err != nil {
		t.Fatalf("go vet after recovery: %v", err)
	}
}

func TestRecoverGoBackendServesExistingStaticIndex(t *testing.T) {
	t.Parallel()
	if !commandAvailable("go") {
		t.Skip("go toolchain unavailable")
	}

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "client", "index.html"), []byte("<!doctype html><title>One-Button Invaders</title>"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := recoverGoBackend(context.Background(), repo, "create /healthz with net/http and serve the static frontend"); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if err := runShell(context.Background(), filepath.Join(repo, "server"), "go test ./..."); err != nil {
		t.Fatalf("go test after recovery: %v", err)
	}
	mainGo, err := os.ReadFile(filepath.Join(repo, "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainGo), `filepath.Join(dir, "index.html")`) {
		t.Fatalf("server/main.go does not serve existing client/index.html:\n%s", mainGo)
	}
}

func TestRecoverHelmChartCreatesLintableChart(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	out, err := recoverHelmChart(context.Background(), repo, "Add Helm chart for local empty invaders repository with Deployment and Service.")
	if err != nil {
		t.Fatalf("recoverHelmChart() error = %v", err)
	}
	if !strings.Contains(out, "charts/invaders") {
		t.Fatalf("output = %q, want chart path", out)
	}
	for _, file := range []string{
		filepath.Join("charts", "invaders", "Chart.yaml"),
		filepath.Join("charts", "invaders", "values.yaml"),
		filepath.Join("charts", "invaders", "templates", "deployment.yaml"),
		filepath.Join("charts", "invaders", "templates", "service.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "helm",
		Description: "Add Helm chart for Kubernetes deployment.",
	}))
	if !status.Passed {
		t.Fatalf("quality gate failed: %+v", status)
	}
}

func TestRecoverDockerfileCreatesValidDockerArtifacts(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverGoBackend(context.Background(), repo, "For a completely empty repository, create a minimal Go net/http service with /healthz and go test ./..."); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	out, err := recoverDockerfile(context.Background(), repo, "Add Dockerfile and container run instructions for the Go application.")
	if err != nil {
		t.Fatalf("recoverDockerfile() error = %v", err)
	}
	if !strings.Contains(out, "Dockerfile") {
		t.Fatalf("output = %q, want Dockerfile summary", out)
	}
	for _, file := range []string{"Dockerfile", ".dockerignore", filepath.Join("docs", "container-run.md")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "docker",
		Description: "Add Dockerfile and container run instructions.",
	}))
	if !status.Passed {
		t.Fatalf("quality gate failed: %+v", status)
	}
}

func TestRecoverDockerfileCopiesStaticFrontendAssetsWhenPresent(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverFrontendStaticApp(repo, "新規性のあるインベーダーゲームを作成する"); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	if _, err := recoverGoBackend(context.Background(), repo, "create /healthz with net/http and serve the static frontend"); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if _, err := recoverDockerfile(context.Background(), repo, "Add Dockerfile for a Go server that serves the static frontend from /"); err != nil {
		t.Fatalf("recoverDockerfile() error = %v", err)
	}
	dockerfile, err := os.ReadFile(filepath.Join(repo, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"WORKDIR /src/server",
		"COPY server/go.mod ./",
		"RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags=\"-s -w\" -o /out/app .",
		"COPY client /app/client",
	} {
		if !strings.Contains(string(dockerfile), want) {
			t.Fatalf("Dockerfile missing %q:\n%s", want, dockerfile)
		}
	}
	docs, err := os.ReadFile(filepath.Join(repo, "docs", "container-run.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(docs), "serves the same primary UI from `/`") {
		t.Fatalf("container docs do not describe static UI parity:\n%s", docs)
	}
}

func TestRecoverDockerfileCopiesStaticFrontendAssetsWithoutPackageJSON(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "client", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("client", "package.json"):   `{"scripts":{"test":"node --check src/main.js","build":"node --check src/main.js"}}`,
		filepath.Join("server", "go.mod"):         "module github.com/hakobune8/arun-test/server\n\ngo 1.22\n",
		filepath.Join("server", "main.go"):        frontendServingGoMain(),
		filepath.Join("client", "index.html"):     `<!doctype html><link rel="stylesheet" href="styles.css"><script src="src/main.js"></script>`,
		filepath.Join("client", "styles.css"):     "body { margin: 0; }\n",
		filepath.Join("client", "src", "main.js"): "console.log('game')\n",
	}
	if err := os.MkdirAll(filepath.Join(repo, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if _, err := recoverDockerfile(context.Background(), repo, "Add Dockerfile for a Go server that serves the static frontend from /"); err != nil {
		t.Fatalf("recoverDockerfile() error = %v", err)
	}
	dockerfile, err := os.ReadFile(filepath.Join(repo, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerfile), "COPY client /app/client") {
		t.Fatalf("Dockerfile does not copy package-less static frontend assets:\n%s", dockerfile)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "docker",
		Description: "Add Dockerfile for a Go server that serves static frontend assets.",
	}))
	if !status.Passed {
		t.Fatalf("quality gate failed: %+v", status)
	}
}

func TestRecoverDockerfileRepairsMissingAbsoluteFrontendAssets(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		filepath.Join("server", "go.mod"):     "module github.com/hakobune8/arun-test/server\n\ngo 1.22\n",
		filepath.Join("server", "main.go"):    "package main\n\nimport \"net/http\"\n\nfunc main() { http.HandleFunc(\"/healthz\", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(\"ok\")) }); http.ListenAndServe(\":8080\", nil) }\n",
		filepath.Join("client", "index.html"): `<!doctype html><link rel="stylesheet" href="/style.css"><script src="/app.js"></script>`,
	}
	for path, content := range files {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if _, err := recoverDockerfile(context.Background(), repo, "Add Dockerfile for a Go server that serves the static frontend from /"); err != nil {
		t.Fatalf("recoverDockerfile() error = %v", err)
	}
	for _, file := range []string{
		filepath.Join("client", "styles.css"),
		filepath.Join("client", "src", "main.js"),
		"Dockerfile",
	} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	dockerfile, err := os.ReadFile(filepath.Join(repo, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(dockerfile), "COPY client /app/client") {
		t.Fatalf("Dockerfile does not copy repaired client assets:\n%s", dockerfile)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "docker",
		Description: "Add Dockerfile for a Go server that serves static frontend assets.",
	}))
	if !status.Passed {
		t.Fatalf("quality gate failed after recovery: %+v", status)
	}
}

func TestDockerQualityGateFailsWhenStaticAssetsAreMissingFromRuntimeImage(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"index.html":                    "<!doctype html><title>Game</title>",
		filepath.Join("src", "main.js"): "console.log('game')\n",
		"Dockerfile": `FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o /app/server .
FROM alpine:3.21
WORKDIR /app
COPY --from=builder /app/server .
CMD ["./server"]
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "docker",
		Description: "Add Dockerfile for a Go server that serves static frontend assets.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed without runtime static asset copies: %+v", status)
	}
}

func TestRecoverDockerfileSkipsStaticAssetCopiesWithoutFrontend(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverGoBackend(context.Background(), repo, "create /healthz with net/http"); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if _, err := recoverDockerfile(context.Background(), repo, "Add Dockerfile for Go service only"); err != nil {
		t.Fatalf("recoverDockerfile() error = %v", err)
	}
	dockerfile, err := os.ReadFile(filepath.Join(repo, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dockerfile), "/src/index.html") || strings.Contains(string(dockerfile), "/src/styles.css") {
		t.Fatalf("Dockerfile copies static assets without frontend:\n%s", dockerfile)
	}
}

func TestRecoverBuiltInSubtask_DockerRuntimeErrorRecovers(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-step-1"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"docker": &recordingAgent{name: "docker"}},
		&runtime.Config{},
	)
	subtask := &Subtask{
		ID:          "step-1",
		AgentName:   "docker",
		Description: "Add or improve a Dockerfile and container-focused run instructions for a minimal Go HTTP server with /healthz.",
	}
	applyDefaultQualityGate(subtask)

	result, ok := o.recoverBuiltInSubtask(context.Background(), subtask, runSandbox, errors.New("validation failed after 1 retries: tests"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if _, err := os.Stat(filepath.Join(repo, "Dockerfile")); err != nil {
		t.Fatalf("Dockerfile not created: %v", err)
	}
}

func TestRecoverBuiltInSubtask_HelmRuntimeErrorRecovers(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-step-1"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"helm": &recordingAgent{name: "helm"}},
		&runtime.Config{},
	)
	subtask := &Subtask{
		ID:          "step-1",
		AgentName:   "helm",
		Description: "Add Helm chart for local empty invaders repository with Deployment and Service.",
	}
	applyDefaultQualityGate(subtask)

	result, ok := o.recoverBuiltInSubtask(context.Background(), subtask, runSandbox, errors.New("validation failed after 1 retries: tests"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if _, err := os.Stat(filepath.Join(repo, "charts", "invaders", "Chart.yaml")); err != nil {
		t.Fatalf("Chart.yaml not created: %v", err)
	}
}

func TestHelmQualityGateFailsWithoutChart(t *testing.T) {
	t.Parallel()

	status := validateQualityGate(context.Background(), t.TempDir(), qualityGateForSubtask(&Subtask{
		AgentName:   "helm",
		Description: "Add Helm chart for Kubernetes deployment.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed without chart: %+v", status)
	}
}

func TestArtifactContractQualityGateFailsWhenGeneratedAppLacksContract(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "client", "index.html"), []byte("<!doctype html><title>App</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "product-brief.md"), []byte("# Product Brief: App\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := validateQualityGate(context.Background(), repo, &QualityGate{
		ValidationCommands: []string{artifactContractValidationCommand},
	})
	if status.Passed {
		t.Fatalf("quality gate passed without artifact contract: %+v", status)
	}
	if !strings.Contains(qualityGateError(status), "docs/artifact-contract.md") {
		t.Fatalf("quality gate error = %q, want artifact contract", qualityGateError(status))
	}
}

func TestArtifactContractQualityGateAcceptsJapaneseContractSections(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	contract := `# 実装契約

## 提供ルート
- GET / は client/index.html を返す。

## バックエンド
- server/main.go に Go HTTP エントリポイントを置く。

## フロントエンド
- client/index.html, client/style.css, client/app.js を使う。

## バリデーション
- go test ./...
`
	if err := os.WriteFile(filepath.Join(repo, "docs", "product-brief.md"), []byte("# Product Brief: App\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "artifact-contract.md"), []byte(contract), 0o600); err != nil {
		t.Fatal(err)
	}
	status := validateQualityGate(context.Background(), repo, &QualityGate{
		ValidationCommands: []string{artifactContractValidationCommand},
	})
	if !status.Passed {
		t.Fatalf("quality gate failed for Japanese artifact contract: %s", qualityGateError(status))
	}
}

func TestRecoverBuiltInSubtask_AnalystQualityGateFailureCreatesPlanningArtifacts(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "artifact-contract.md"), []byte("# 実装契約\n\n## バリデーション\n- go test ./...\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-planning-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"analyst": &recordingAgent{name: "analyst"}},
		&runtime.Config{},
	)
	subtask := &Subtask{
		ID:          "sprint-1-plan",
		AgentName:   "analyst",
		Description: "Sprint 1 product planning and design: create docs/product-brief.md and docs/artifact-contract.md for a Go net/http app with /healthz.",
	}
	applyDefaultQualityGate(subtask)
	status := validateQualityGate(context.Background(), repo, subtask.QualityGate)
	if status.Passed {
		t.Fatalf("quality gate unexpectedly passed before recovery")
	}

	result, ok := o.recoverNoOpBuiltInSubtaskWithStatus(context.Background(), subtask, runSandbox, status)
	if !ok || !result.Success {
		t.Fatalf("recoverNoOpBuiltInSubtaskWithStatus() = (%+v, %v), want success", result, ok)
	}
	for _, file := range []string{filepath.Join("docs", "product-brief.md"), filepath.Join("docs", "artifact-contract.md")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed after recovery", result.QualityGate)
	}
}

func TestHelmQualityGateFailsForEmptyChart(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	chartDir := filepath.Join(repo, "charts", "inverter")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(chartDir, "Chart.yaml"), []byte("apiVersion: v2\nname: inverter\ntype: application\nversion: 0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "helm",
		Description: "Add Helm chart for Kubernetes deployment.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed for empty chart: %+v", status)
	}
}

func TestHelmQualityGateFailsForOrphanChartTemplates(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "charts", "orphan", "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "charts", "orphan", "templates", "service.yaml"), []byte("apiVersion: v1\nkind: Service\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "helm",
		Description: "Add Helm chart for Kubernetes deployment.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed for orphan chart templates: %+v", status)
	}
}

func TestHelmQualityGateFailsWhenAnyChartIsEmpty(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	validChart := filepath.Join(repo, "charts", "valid")
	if err := os.MkdirAll(filepath.Join(validChart, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	validFiles := map[string]string{
		filepath.Join(validChart, "Chart.yaml"):  "apiVersion: v2\nname: valid\ntype: application\nversion: 0.1.0\n",
		filepath.Join(validChart, "values.yaml"): "replicaCount: 1\n",
		filepath.Join(validChart, "templates", "deployment.yaml"): `apiVersion: apps/v1
kind: Deployment
metadata:
  name: valid
spec:
  selector:
    matchLabels:
      app: valid
  template:
    metadata:
      labels:
        app: valid
    spec:
      containers:
        - name: valid
          image: nginx
`,
		filepath.Join(validChart, "templates", "service.yaml"): `apiVersion: v1
kind: Service
metadata:
  name: valid
spec:
  selector:
    app: valid
  ports:
    - port: 80
`,
	}
	for path, content := range validFiles {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	emptyChart := filepath.Join(repo, "charts", "empty")
	if err := os.MkdirAll(emptyChart, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(emptyChart, "Chart.yaml"), []byte("apiVersion: v2\nname: empty\ntype: application\nversion: 0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "helm",
		Description: "Add Helm charts for Kubernetes deployment.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed when one chart is empty: %+v", status)
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

func TestDockerQualityGate_AllowsMissingDockerDaemon(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("Docker daemon behavior differs on Windows runners")
	}
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "Dockerfile"), []byte("FROM scratch\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName: "docker",
	}))
	if !status.Passed {
		t.Fatalf("docker quality gate failed without daemon: %+v", status)
	}
}

func TestRecoverGoBackend_CreatesValidService(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	out, err := recoverGoBackend(context.Background(), repo, "https://github.com/hakobune8/arun-test.git create /healthz with net/http")
	if err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if !strings.Contains(out, "Go net/http service") {
		t.Fatalf("output = %q", out)
	}
	for _, file := range []string{filepath.Join("server", "go.mod"), filepath.Join("server", "main.go")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	mainData, err := os.ReadFile(filepath.Join(repo, "server", "main.go"))
	if err != nil {
		t.Fatalf("read server/main.go: %v", err)
	}
	for _, want := range []string{"net/http", "healthzHandler", `"status": "ok"`} {
		if !strings.Contains(string(mainData), want) {
			t.Fatalf("server/main.go missing %q:\n%s", want, mainData)
		}
	}
}

func TestRecoverGoBackendServesClientFromServerWorkingDirectory(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "client", "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("client", "package.json"):   `{"scripts":{"test":"node --check src/main.js","build":"node --check src/main.js"}}`,
		filepath.Join("client", "index.html"):     `<!doctype html><link rel="stylesheet" href="./styles.css"><script src="./src/main.js"></script>`,
		filepath.Join("client", "styles.css"):     "body { color: white; }",
		filepath.Join("client", "src", "main.js"): "console.log('ok');",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if _, err := recoverGoBackend(context.Background(), repo, "https://github.com/hakobune8/arun-test.git create /healthz with net/http and serve client assets"); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	mainData, err := os.ReadFile(filepath.Join(repo, "server", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainData), `filepath.Join("..", "client")`) {
		t.Fatalf("server/main.go does not resolve ../client when run from server directory:\n%s", mainData)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "frontend",
		Description: "Add a browser game UI.",
	}))
	if !status.Passed {
		t.Fatalf("frontend quality gate failed after backend recovery: %+v", status)
	}
}

func TestGoQualityGateRejectsConflictingRootAndServerModules(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":                          "module github.com/hakobune8/arun-test\n\ngo 1.22\n",
		filepath.Join("server", "go.mod"): "module github.com/hakobune8/arun-test/server\n\ngo 1.22\n",
		filepath.Join("server", "main.go"): `package main

import "net/http"

func main() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(` + "`" + `{"status":"ok"}` + "`" + `))
	})
	_ = http.ListenAndServe(":8080", nil)
}
`,
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "go-backend",
		Description: "Create a minimal Go net/http server with /healthz.",
	}))
	if status.Passed {
		t.Fatalf("quality gate passed with nested server/go.mod: %+v", status)
	}
	if !strings.Contains(qualityGateError(status), "root go.mod conflicts") {
		t.Fatalf("quality gate error = %q, want root go.mod conflict", qualityGateError(status))
	}
}

func TestRecoverGoBackendRemovesConflictingRootModule(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod":                          "module github.com/hakobune8/arun-test\n\ngo 1.22\n",
		filepath.Join("server", "go.mod"): "module github.com/hakobune8/arun-test/server\n\ngo 1.22\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	if _, err := recoverGoBackend(context.Background(), repo, "Create a minimal Go net/http server with /healthz."); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "go.mod")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("root go.mod still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "server", "go.mod")); err != nil {
		t.Fatalf("server/go.mod missing after recovery: %v", err)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "go-backend",
		Description: "Create a minimal Go net/http server with /healthz.",
	}))
	if !status.Passed {
		t.Fatalf("quality gate failed after recovery: %+v", status)
	}
}

func TestRecoverGoBackend_AllowsMissingGoToolchain(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("PATH-limited POSIX shell validation is covered on Unix runners")
	}
	repo := t.TempDir()
	bin := t.TempDir()
	if err := os.Symlink("/bin/sh", filepath.Join(bin, "sh")); err != nil {
		t.Fatalf("symlink sh: %v", err)
	}
	if err := os.Symlink("/usr/bin/grep", filepath.Join(bin, "grep")); err != nil {
		t.Fatalf("symlink grep: %v", err)
	}
	t.Setenv("PATH", bin)

	out, err := recoverGoBackend(context.Background(), repo, "create /healthz with net/http")
	if err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if !strings.Contains(out, "Go toolchain is unavailable") {
		t.Fatalf("output = %q, want unavailable toolchain note", out)
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "go-backend",
		Description: "create /healthz with net/http",
	}))
	if !status.Passed {
		t.Fatalf("quality gate failed without go toolchain: %+v", status)
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
	for _, file := range []string{filepath.Join("server", "main_test.go"), filepath.Join(".github", "workflows", "go.yml")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
}

func TestRecoverGoCI_IncludesClientValidationWhenClientPackageExists(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverFrontendStaticApp(repo, "新規性のあるインベーダーゲームを作成する"); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	if _, err := recoverGoBackend(context.Background(), repo, "create /healthz with Go HTTP server"); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	if _, err := recoverGoCI(context.Background(), repo); err != nil {
		t.Fatalf("recoverGoCI() error = %v", err)
	}
	workflow, err := os.ReadFile(filepath.Join(repo, ".github", "workflows", "go.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"actions/setup-node@v4", "npm --prefix client test", "npm --prefix client run build"} {
		if !strings.Contains(string(workflow), want) {
			t.Fatalf("workflow missing %q:\n%s", want, workflow)
		}
	}
	status := validateQualityGate(context.Background(), repo, qualityGateForSubtask(&Subtask{
		AgentName:   "devops",
		Description: "Sprint 3 coding: add or improve GitHub Actions CI so future pull requests can continuously run checks.",
	}))
	if !status.Passed {
		t.Fatalf("devops quality gate failed after CI recovery: %+v", status)
	}
}

func TestRecoverNoOpDevopsCreatesMissingCIWorkflow(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))
	if _, err := recoverFrontendStaticApp(repo, "新規性のあるインベーダーゲームを作成する"); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	if _, err := recoverGoBackend(context.Background(), repo, "create /healthz with Go HTTP server"); err != nil {
		t.Fatalf("recoverGoBackend() error = %v", err)
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-devops-ci"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"devops": &recordingAgent{name: "devops"}},
		&runtime.Config{},
	)

	result, ok := o.recoverNoOpBuiltInSubtask(context.Background(), &Subtask{
		ID:          "sprint-3-devops",
		AgentName:   "devops",
		Description: "Sprint 3 coding: add or improve GitHub Actions CI so future pull requests can continuously run the available backend/frontend/container checks.",
	}, runSandbox)
	if !ok || !result.Success {
		t.Fatalf("recoverNoOpBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if _, err := os.Stat(filepath.Join(repo, ".github", "workflows", "go.yml")); err != nil {
		t.Fatalf("workflow not created: %v", err)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
}

func TestRecoverNoOpDocs_CreatesRequiredREADME(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("ARUN_HOME", filepath.Join(t.TempDir(), "arun-home"))
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

func TestRecoverBuiltInSubtask_StaticFrontendQAAndRelease(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverFrontendStaticApp(repo, "Run an implementation-heavy agile scrum workflow for hakobune8/invaders on main."); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-static-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{
			"qa":              &recordingAgent{name: "qa"},
			"release-manager": &recordingAgent{name: "release-manager"},
		},
		&runtime.Config{},
	)

	qaResult, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:          "step-2",
		AgentName:   "qa",
		Description: "Add focused frontend validation and smoke notes.",
	}, runSandbox, errors.New("validation failed after 3 retries: tests"))
	if !ok || !qaResult.Success {
		t.Fatalf("qa recoverBuiltInSubtask() = (%+v, %v), want success", qaResult, ok)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "testing.md")); err != nil {
		t.Fatalf("docs/testing.md not created: %v", err)
	}

	releaseResult, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:          "step-4",
		AgentName:   "release-manager",
		Description: "Prepare release readiness notes.",
	}, runSandbox, errors.New("quality gate failed"))
	if !ok || !releaseResult.Success {
		t.Fatalf("release recoverBuiltInSubtask() = (%+v, %v), want success", releaseResult, ok)
	}
	changelog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("CHANGELOG.md not created: %v", err)
	}
	if !strings.Contains(string(changelog), "v0.1.0") {
		t.Fatalf("CHANGELOG.md missing version entry:\n%s", changelog)
	}
}

func TestRecoverFrontendReleaseDiscardsImplementationChanges(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverFrontendStaticApp(repo, "新規性のあるインベーダーゲームを作成する"); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	if err := runCmd(context.Background(), repo, "git", "init", "-b", "main"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := runCmd(context.Background(), repo, "git", "config", "user.email", "arun@example.com"); err != nil {
		t.Fatalf("git config email: %v", err)
	}
	if err := runCmd(context.Background(), repo, "git", "config", "user.name", "ARUN"); err != nil {
		t.Fatalf("git config name: %v", err)
	}
	if err := runCmd(context.Background(), repo, "git", "add", "."); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := runCmd(context.Background(), repo, "git", "commit", "-m", "baseline"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	original, err := os.ReadFile(filepath.Join(repo, "client", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "client", "index.html"), []byte("<!doctype html><title>Corrupted</title>"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := recoverFrontendRelease(repo, "Sprint 1 reporting: summarize validation evidence."); err != nil {
		t.Fatalf("recoverFrontendRelease() error = %v", err)
	}
	restored, err := os.ReadFile(filepath.Join(repo, "client", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	restored = bytes.ReplaceAll(restored, []byte("\r\n"), []byte("\n"))
	original = bytes.ReplaceAll(original, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(restored, original) {
		t.Fatalf("client/index.html was not restored:\n%s", restored)
	}
	changelog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changelog), "v0.1.0") {
		t.Fatalf("CHANGELOG.md missing release entry:\n%s", changelog)
	}
}

func TestRecoverBuiltInSubtask_FrontendRemovesUnservedAlternateUIAndArtifacts(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if _, err := recoverFrontendStaticApp(repo, "新規性のあるインベーダーゲームを作成する"); err != nil {
		t.Fatalf("recoverFrontendStaticApp() error = %v", err)
	}
	if err := os.Mkdir(filepath.Join(repo, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join("web", "index.html"): "<!doctype html><title>Orbit Invaders</title><script src=\"script.js\"></script>",
		"CHANGELOG.md":                     "# Changelog\n\n## v0.1.0\n\n- Initial app.\n\n## Scenario\n\nParent task:\nmake a game\n\nOperating mode: build-first\n\nQuality bar:\n- pass\n\nExpected output:\n- files\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	binaryName := "20260704T235557-run-b7443127479f1e91-hakobune8-arun-test"
	if err := os.WriteFile(filepath.Join(repo, binaryName), []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01}, 0o600); err != nil {
		t.Fatal(err)
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-step-frontend-fix"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"frontend": &recordingAgent{name: "frontend"}},
		&runtime.Config{},
	)

	result, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:          "step-frontend-fix",
		AgentName:   "frontend",
		Description: "Address frontend quality gate failure for an unserved alternate UI while preserving the Go backend.",
		QualityGate: qualityGateForSubtask(&Subtask{
			AgentName:   "frontend",
			Description: "Add a browser game UI.",
		}),
	}, runSandbox, errors.New("validation failed after 3 retries: tests"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
	if _, err := os.Stat(filepath.Join(repo, "web")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("web dir stat err = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(repo, binaryName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary artifact stat err = %v, want not exist", err)
	}
	changelog, err := os.ReadFile(filepath.Join(repo, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(changelog), "Parent task:") || strings.Contains(string(changelog), "Quality bar:") {
		t.Fatalf("CHANGELOG.md still contains prompt contamination:\n%s", changelog)
	}
}

func TestRecoverBuiltInSubtask_PartialStaticFrontendQA(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"package.json":                  `{"scripts":{"test":"node --check src/main.js"}}`,
		"index.html":                    `<script type="module" src="./src/main.js"></script>`,
		filepath.Join("src", "main.js"): `console.log("empty invaders");`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-partial-static-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"qa": &recordingAgent{name: "qa"}},
		&runtime.Config{},
	)

	result, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:          "step-2",
		AgentName:   "qa",
		Description: "Add deterministic Empty Invaders smoke tests and manual QA notes.",
	}, runSandbox, errors.New("validation failed after 3 retries: tests"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	for _, file := range []string{filepath.Join("docs", "smoke-test.md"), filepath.Join("docs", "testing.md")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
}

func TestRecoverBuiltInSubtask_AnalystEmptyPlanningOutput(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-planning-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"analyst": &recordingAgent{name: "analyst"}},
		&runtime.Config{},
	)

	result, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:        "sprint-1-plan",
		AgentName: "analyst",
		Description: `Sprint 1 planning: inspect the repository state.

Target baseline:
- Minimal Go net/http server.
- Health endpoint /healthz.
- Run go test ./... and go vet ./...`,
	}, runSandbox, errors.New("create plan: parse plan JSON: unexpected end of JSON input\ncontent: "))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	planPath := filepath.Join(repo, "docs", "sprint-planning", "sprint-1-plan.md")
	body, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("planning artifact not created: %v", err)
	}
	for _, want := range []string{"Recovery Plan", "net/http", "/healthz", "go test ./..."} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("planning artifact missing %q:\n%s", want, body)
		}
	}
	for _, file := range []string{filepath.Join("docs", "product-brief.md"), filepath.Join("docs", "artifact-contract.md")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	contract, err := os.ReadFile(filepath.Join(repo, "docs", "artifact-contract.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Primary route", "Frontend", "Backend", "Validation"} {
		if !strings.Contains(string(contract), want) {
			t.Fatalf("artifact contract missing %q:\n%s", want, contract)
		}
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
	}
}

func TestRecoverBuiltInSubtask_GoQAValidationFailure(t *testing.T) {
	t.Parallel()

	repo := t.TempDir()
	files := map[string]string{
		"go.mod": `module github.com/example/invaders

go 1.22
`,
		"main.go": `package main

import (
	"encoding/json"
	"net/http"
)

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	http.HandleFunc("/healthz", healthzHandler)
	_ = http.ListenAndServe(":8080", nil)
}
`,
		"main_test.go": `package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	healthzHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %+v", body)
	}
}
`,
		"Dockerfile": `FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /app/server .
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	runSandbox := sandbox.NewLocalSandbox(repo)
	if err := runSandbox.PrepareRun("run-go-qa-step"); err != nil {
		t.Fatalf("PrepareRun() error = %v", err)
	}
	o := NewOrchestrator(
		llm.NewMockLLMClient(nil),
		sandbox.NewLocalSandbox(repo),
		map[string]runtime.Agent{"qa": &recordingAgent{name: "qa"}},
		&runtime.Config{},
	)

	result, ok := o.recoverBuiltInSubtask(context.Background(), &Subtask{
		ID:        "sprint-1-qa",
		AgentName: "qa",
		Description: `Sprint 1 QA: run available tests or smoke checks, record gaps in repository artifacts.

新規 repository の target baseline:
- Health endpoint と小さな product API または static asset handler を持つ minimal Go HTTP server。
- Tests、lint または smoke checks、build validation を実行する GitHub Actions CI。`,
	}, runSandbox, errors.New("validation failed after 3 retries: tests"))
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success", result, ok)
	}
	for _, file := range []string{filepath.Join("docs", "testing.md"), filepath.Join("docs", "smoke-test.md")} {
		if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
			t.Fatalf("%s not created: %v", file, err)
		}
	}
	dockerfile, err := os.ReadFile(filepath.Join(repo, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dockerfile), "go.mod go.sum") {
		t.Fatalf("Dockerfile still requires missing go.sum:\n%s", dockerfile)
	}
	if result.QualityGate == nil || !result.QualityGate.Passed {
		t.Fatalf("quality gate = %+v, want passed", result.QualityGate)
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
		Description: "Create /healthz with net/http in https://github.com/hakobune8/arun-test.git",
	}, runSandbox, context.DeadlineExceeded)
	if !ok || !result.Success {
		t.Fatalf("recoverBuiltInSubtask() = (%+v, %v), want success despite canceled subtask context", result, ok)
	}
	if _, err := os.Stat(filepath.Join(repo, "server", "main.go")); err != nil {
		t.Fatalf("server/main.go not created: %v", err)
	}
}

func TestInferModulePath_ExtractsGitHubURLWithoutRegex(t *testing.T) {
	t.Parallel()

	got := inferModulePath("target repo is https://github.com/hakobune8/arun-test.git and should expose /healthz", t.TempDir())
	if got != "github.com/hakobune8/arun-test" {
		t.Fatalf("inferModulePath() = %q", got)
	}
}

func TestInferModulePath_UsesGitRemoteBeforeWorkspaceName(t *testing.T) {
	t.Parallel()
	if !commandAvailable("git") {
		t.Skip("git unavailable")
	}

	repo := t.TempDir()
	if err := runCmd(context.Background(), repo, "git", "init"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := runCmd(context.Background(), repo, "git", "remote", "add", "origin", "git@github.com:hakobune8/arun-test.git"); err != nil {
		t.Fatalf("git remote add: %v", err)
	}

	got := inferModulePath("create a Go service with /healthz", repo)
	if got != "github.com/hakobune8/arun-test" {
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
