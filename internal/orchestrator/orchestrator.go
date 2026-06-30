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

// Package orchestrator provides multi-agent coordination and task execution.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/safety"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/task"
)

// Strategy defines the execution strategy for multi-agent coordination.
type Strategy string

const (
	// StrategySequential executes subtasks one after another.
	StrategySequential Strategy = "sequential"
	// StrategyParallel executes subtasks concurrently.
	StrategyParallel Strategy = "parallel"
)

// Orchestrator coordinates multiple agents to execute a task.
type Orchestrator struct {
	llm            llm.LLMClient
	sandbox        sandbox.Sandbox
	agents         map[string]runtime.Agent
	agentDefs      []AgentMetadata
	agentProfiles  map[string]profile.Profile
	strategy       Strategy
	cfg            *runtime.Config
	baseBranch     string
	subtaskTimeout time.Duration
	runID          string
}

// AgentMetadata describes an agent to the planner.
type AgentMetadata struct {
	Name                 string
	Description          string
	ArchitectureGuidance []string
	OutputExpectations   []string
}

// NewOrchestrator creates a new Orchestrator with the given llm client, sandbox, and agents.
func NewOrchestrator(llmClient llm.LLMClient, sb sandbox.Sandbox, agents map[string]runtime.Agent, cfg *runtime.Config) *Orchestrator {
	var infos []AgentMetadata
	profiles := make(map[string]profile.Profile)
	for name, a := range agents {
		infos = append(infos, builtInAgentInfo(name, a.Name()))
		profiles[name] = subtaskProfile(a.Name())
	}
	return &Orchestrator{
		llm:           llmClient,
		sandbox:       sb,
		agents:        agents,
		agentDefs:     infos,
		agentProfiles: profiles,
		strategy:      StrategySequential,
		cfg:           cfg,
		baseBranch:    "main",
	}
}

// DefaultAgent returns the first registered agent, used as fallback.
func (o *Orchestrator) DefaultAgent() runtime.Agent {
	for _, a := range o.agents {
		return a
	}
	return nil
}

// SubtaskEventType identifies a subtask execution lifecycle event.
type SubtaskEventType string

const (
	// SubtaskStarted indicates that a subtask has started.
	SubtaskStarted SubtaskEventType = "started"
	// SubtaskCompleted indicates that a subtask has completed.
	SubtaskCompleted SubtaskEventType = "completed"
)

// SubtaskEvent reports incremental subtask execution progress.
type SubtaskEvent struct {
	Type     SubtaskEventType `json:"type"`
	Subtask  Subtask          `json:"subtask"`
	Result   *SubtaskResult   `json:"result,omitempty"`
	Started  time.Time        `json:"startedAt,omitempty"`
	Finished time.Time        `json:"finishedAt,omitempty"`
}

// SubtaskObserver receives incremental subtask execution events.
type SubtaskObserver func(SubtaskEvent)

// SetStrategy sets the execution strategy for the orchestrator.
func (o *Orchestrator) SetStrategy(s Strategy) {
	o.strategy = s
}

// SetBaseBranch sets the base branch used for subtask task metadata.
func (o *Orchestrator) SetBaseBranch(branch string) {
	if branch != "" {
		o.baseBranch = branch
	}
}

// SetRunID sets the parent orchestration ID used to scope runtime artifacts.
func (o *Orchestrator) SetRunID(id string) {
	o.runID = id
}

// SetSubtaskTimeout sets the maximum runtime for a single subtask.
func (o *Orchestrator) SetSubtaskTimeout(timeout time.Duration) {
	o.subtaskTimeout = timeout
}

// SetAgentMetadata overrides planner metadata and runtime profiles for selected
// agents. It is primarily used for repository-defined custom agents.
func (o *Orchestrator) SetAgentMetadata(infos []AgentMetadata, profiles map[string]profile.Profile) {
	if len(infos) > 0 {
		o.agentDefs = infos
	}
	if len(profiles) > 0 {
		if o.agentProfiles == nil {
			o.agentProfiles = make(map[string]profile.Profile, len(profiles))
		}
		for name := range profiles {
			o.agentProfiles[name] = profiles[name]
		}
	}
}

// TaskPlan represents a breakdown of a task into subtasks.
type TaskPlan struct {
	Description string    `json:"description"`
	Subtasks    []Subtask `json:"subtasks"`
}

// Subtask represents a single unit of work within a task plan.
type Subtask struct {
	ID          string       `json:"id"`
	Description string       `json:"description"`
	AgentName   string       `json:"agent_type"`
	Deps        []string     `json:"dependencies"`
	QualityGate *QualityGate `json:"quality_gate,omitempty"`
}

// SubtaskResult contains the result of executing a subtask.
type SubtaskResult struct {
	SubtaskID   string             `json:"subtask_id"`
	Output      string             `json:"output"`
	Diff        string             `json:"diff,omitempty"`
	Error       string             `json:"error,omitempty"`
	Success     bool               `json:"success"`
	QualityGate *QualityGateStatus `json:"quality_gate,omitempty"`
}

// Plan uses an LLM to break a task description into a plan of subtasks.
func (o *Orchestrator) Plan(ctx context.Context, taskDesc string) (*TaskPlan, error) {
	systemMsg := llm.Message{
		Role: llm.RoleSystem,
		Content: `You are a task planner for multi-agent coordination. Break down the given task into subtasks that multiple agents can work on.

Output ONLY valid JSON with this structure:
{
  "description": "task overview",
  "subtasks": [
    {
      "id": "step-1",
      "description": "what to do",
      "agent_type": "agent name from the list",
      "dependencies": [],
      "quality_gate": {
        "required_files": ["relative/path.ext"],
        "validation_commands": ["command to run from repository root"],
        "content_checks": [{"file":"relative/path.ext","contains":["required text"]}]
      }
    }
  ]
}

Use quality_gate when a subtask has required files, validation commands, or required content. Omit empty quality gates.
Do not include markdown, explanations, or reasoning. The assistant message content must be the JSON object only.`,
	}

	agentsInfo := ""
	for _, info := range o.agentDefs {
		agentsInfo += fmt.Sprintf("- %s: %s\n", info.Name, info.Description)
		if len(info.ArchitectureGuidance) > 0 {
			agentsInfo += "  Architecture/conventions:\n"
			for _, item := range info.ArchitectureGuidance {
				agentsInfo += fmt.Sprintf("  - %s\n", item)
			}
		}
		if len(info.OutputExpectations) > 0 {
			agentsInfo += "  Output expectations:\n"
			for _, item := range info.OutputExpectations {
				agentsInfo += fmt.Sprintf("  - %s\n", item)
			}
		}
	}

	userMsg := llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf(`Task: %s

Available agents:
%s

Break this task into subtasks and assign each to the most suitable agent.`, taskDesc, agentsInfo),
	}

	resp, err := o.llm.Chat(ctx, llm.ChatRequest{
		Model:       o.llm.ModelName(),
		Messages:    []llm.Message{systemMsg, userMsg},
		Temperature: 0.2,
		MaxTokens:   4096,
	})
	if err != nil {
		return o.fallbackPlan(taskDesc), nil
	}

	content := resp.Choices[0].Message.Content
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if content == "" {
		return o.fallbackPlan(taskDesc), nil
	}

	var plan TaskPlan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return o.fallbackPlan(taskDesc), nil
	}

	plan.Description = taskDesc
	enrichSubtasks(&plan, taskDesc)
	return &plan, nil
}

func enrichSubtasks(plan *TaskPlan, parentTask string) {
	if plan == nil {
		return
	}
	for i := range plan.Subtasks {
		switch plan.Subtasks[i].AgentName {
		case "go-backend":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete go-backend requirements: inspect the existing repository layout before choosing files; preserve the exact parent task requirements; follow existing cmd/, internal/, pkg/, api/, router, middleware, and package conventions when present; use idiomatic standard-library Go for small services; create go.mod only if missing; create or update main.go when the repository has no clearer entrypoint; ensure go test ./... and go vet ./... can run.")
		case "ci-fixer":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete ci-fixer requirements: inspect existing GitHub Actions workflows before replacing them; preserve current job intent; prefer actions/checkout, actions/setup-go, cache-aware Go setup, go test ./..., and go vet ./...; keep validation passing.")
		case "docs":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete docs requirements: inspect README.md and docs/ before adding content; preserve existing style; cover overview, quickstart or startup instructions, configuration, endpoints, testing, deployment, and troubleshooting where relevant.")
		case "reviewer":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete reviewer requirements: review the final diff against the parent task; flag correctness, test coverage, security, maintainability, release-readiness, over-engineering, and convention-breaking findings with severity and file references.")
		case "security":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete security requirements: inspect dependency, auth/session, secret-handling, permission, and security-sensitive code paths; prefer focused defensive fixes; include tests or manual verification notes; keep go test ./... and go vet ./... passing when code changes.")
		case "release-manager":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete release-manager requirements: inspect existing CHANGELOG.md, release notes, versioning, and chart conventions; update release artifacts only when in scope; record release readiness, validation, deployment, and rollback notes.")
		case "dependency-updater":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete dependency-updater requirements: inspect manifests, lockfiles, Go toolchain, and workflow compatibility first; prefer narrow requested updates; keep go.mod/go.sum synchronized; run go mod tidy and go test ./... when Go dependencies change.")
		case "qa":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete qa requirements: inspect existing test layout and verification docs; add focused regression, scenario, or smoke coverage for changed behavior; document manual verification steps when automation is incomplete; keep go test ./... passing.")
		default:
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask, "")
		}
		applyDefaultQualityGate(&plan.Subtasks[i])
	}
}

func appendContext(description, parentTask, extra string) string {
	var b strings.Builder
	b.WriteString(description)
	if extra != "" {
		b.WriteString("\n\n")
		b.WriteString(extra)
	}
	b.WriteString("\n\nParent orchestration task:\n")
	b.WriteString(parentTask)
	return b.String()
}

func (o *Orchestrator) fallbackPlan(taskDesc string) *TaskPlan {
	available := make(map[string]bool, len(o.agentDefs))
	for _, info := range o.agentDefs {
		available[info.Name] = true
	}

	var subtasks []Subtask
	add := func(id, agentName, description string, deps []string) {
		if available[agentName] {
			subtasks = append(subtasks, Subtask{
				ID:          id,
				AgentName:   agentName,
				Description: description,
				Deps:        deps,
			})
			applyDefaultQualityGate(&subtasks[len(subtasks)-1])
		}
	}

	add("step-1", "go-backend", fmt.Sprintf("Implement the Go backend requested by the parent task. Inspect the existing repository layout first and preserve established cmd/, internal/, pkg/, api/, router, middleware, and package conventions. Prefer idiomatic standard-library Go for small services and avoid unnecessary layout churn. Create go.mod only if missing. Parent task:\n\n%s", taskDesc), nil)
	add("step-2", "docs", fmt.Sprintf("Update README.md or docs/ for the requested changes. Inspect existing documentation style first. Include practical startup, configuration, endpoint, testing, deployment, and troubleshooting details where relevant. Parent task:\n\n%s", taskDesc), nil)
	ciDeps := dependenciesForAvailable(available, "go-backend")
	add("step-3", "ci-fixer", fmt.Sprintf("Add or fix Go tests and GitHub Actions workflow so go test ./... succeeds for the implementation requested by the parent task. Inspect existing workflow conventions first and prefer checkout/setup-go with cache-aware Go setup plus explicit go test and go vet steps. Parent task:\n\n%s", taskDesc), ciDeps)
	add("step-4", "security", fmt.Sprintf("Review and fix security-sensitive aspects of the requested change. Inspect dependencies, auth/session handling, secrets, permissions, and security-relevant configuration. Add tests or manual verification notes where useful. Parent task:\n\n%s", taskDesc), dependenciesForAvailable(available, "go-backend"))
	add("step-5", "qa", fmt.Sprintf("Add focused regression, scenario, or smoke coverage for the requested change. Preserve existing test conventions and document manual verification steps when automation is incomplete. Parent task:\n\n%s", taskDesc), dependenciesForAvailable(available, "go-backend"))
	reviewerDeps := dependenciesForAvailable(available, "go-backend", "docs", "ci-fixer", "security", "qa")
	add("step-6", "reviewer", fmt.Sprintf("Review the final diff for correctness, tests, security, maintainability, release readiness, and convention preservation. Flag over-engineered or convention-breaking changes with severity and file references. Parent task:\n\n%s", taskDesc), reviewerDeps)

	if len(subtasks) == 0 {
		for i, info := range o.agentDefs {
			subtasks = append(subtasks, Subtask{
				ID:          fmt.Sprintf("step-%d", i+1),
				AgentName:   info.Name,
				Description: taskDesc,
			})
			applyDefaultQualityGate(&subtasks[len(subtasks)-1])
		}
	}

	return &TaskPlan{Description: taskDesc, Subtasks: subtasks}
}

func builtInAgentInfo(name, fallbackDescription string) AgentMetadata {
	info := AgentMetadata{Name: name, Description: fallbackDescription}
	switch name {
	case "go-backend":
		info.Description = "Go backend coding agent that preserves existing architecture before adding idiomatic Go changes"
		info.ArchitectureGuidance = []string{
			"Inspect existing layout before editing and follow established package, cmd/, internal/, pkg/, api/, router, and middleware conventions when present.",
			"Prefer idiomatic standard-library Go for small services; introduce frameworks or new top-level layout only when task complexity warrants it.",
			"Separate handlers, configuration, and tests when the repository already uses that structure; avoid over-engineering small repositories.",
		}
		info.OutputExpectations = []string{"gofmt, go test ./..., and go vet ./... pass.", "Architecture choices are summarized when new structure is introduced."}
	case "ci-fixer":
		info.Description = "CI fix agent for conventional GitHub Actions and validation repairs"
		info.ArchitectureGuidance = []string{
			"Inspect existing workflow names, jobs, matrices, and branch-protection expectations before replacing CI structure.",
			"Prefer actions/checkout, actions/setup-go, cache-aware Go setup, go test ./..., and go vet ./...",
			"Keep lint, test, and optional security steps explicit and compatible with the repository's existing Go version and module layout.",
		}
		info.OutputExpectations = []string{"Workflow YAML preserves existing job intent.", "Local validation mirrors the workflow where practical."}
	case "docs":
		info.Description = "Documentation agent that updates practical docs while matching existing repository style"
		info.ArchitectureGuidance = []string{
			"Inspect README.md and docs/ structure before adding sections or files.",
			"Prefer overview, quickstart, configuration, endpoints, testing, deployment, and troubleshooting sections where relevant.",
			"Preserve existing tone, headings, examples, and link conventions.",
		}
		info.OutputExpectations = []string{"Docs cover changed user-visible behavior.", "Commands and examples are runnable from the repository root."}
	case "reviewer":
		info.Description = "Code review agent for correctness, tests, security, maintainability, and release readiness"
		info.ArchitectureGuidance = []string{
			"Evaluate whether changes preserve existing repository conventions before judging style preferences.",
			"Flag over-engineered layouts, unnecessary dependencies, and convention-breaking rewrites.",
			"Review tests, security-sensitive behavior, maintainability, and release readiness with severity and file references.",
		}
		info.OutputExpectations = []string{"Findings include severity and file references where applicable.", "Review states validation and release-readiness risk."}
	case "security":
		info.Description = "Security agent for dependencies, auth/session handling, secrets, and security-sensitive diffs"
		info.ArchitectureGuidance = []string{
			"Inspect authentication, authorization, session, secret-handling, dependency, and CI security conventions before proposing changes.",
			"Prefer small defensive fixes, safer defaults, and standard library or existing dependency patterns over broad rewrites.",
			"Document residual risk and validation scope when a finding cannot be fully fixed in the current task.",
		}
		info.OutputExpectations = []string{"Security-sensitive changes include tests or manual verification notes.", "Dependency or configuration findings identify the affected package, file, workflow, or setting.", "go test ./... and go vet ./... pass when code is changed."}
	case "release-manager":
		info.Description = "Release manager agent for changelogs, release notes, release checklists, and readiness validation"
		info.ArchitectureGuidance = []string{
			"Inspect existing changelog, release note, versioning, and Helm chart conventions before editing release artifacts.",
			"Keep version changes explicit and avoid publishing or tagging releases unless the task asks for it.",
			"Summarize release readiness, known gaps, and deployment or rollback considerations.",
		}
		info.OutputExpectations = []string{"CHANGELOG.md or release documentation is updated when release notes are requested.", "Version and chart changes are consistent when release packaging is in scope.", "Release checklist items are concrete and traceable to validation commands or manual checks."}
	case "dependency-updater":
		info.Description = "Dependency updater agent for Go modules, package locks, and GitHub Actions versions"
		info.ArchitectureGuidance = []string{
			"Inspect existing dependency managers, lockfiles, toolchain versions, and CI compatibility before updating versions.",
			"Prefer narrow updates requested by the task; avoid broad upgrades unless the task calls for them.",
			"Keep generated files such as go.sum or lockfiles consistent with the manifest that changed.",
		}
		info.OutputExpectations = []string{"Manifests and lockfiles remain synchronized after updates.", "go mod tidy and go test ./... pass for Go dependency work.", "Compatibility or breaking-change notes are included for major or security-sensitive upgrades."}
	case "qa":
		info.Description = "QA agent for scenario tests, smoke checks, regression coverage, and manual verification notes"
		info.ArchitectureGuidance = []string{
			"Inspect existing test layout, fixtures, and documented verification workflows before adding new checks.",
			"Prefer focused regression and smoke coverage that exercises user-visible behavior changed by the task.",
			"Record manual verification steps when behavior cannot be fully automated.",
		}
		info.OutputExpectations = []string{"New or updated tests fail without the intended behavior and pass with it.", "go test ./... passes when Go code or tests are in scope.", "Manual verification notes include concrete commands, URLs, or scenarios."}
	}
	return info
}

func dependenciesForAvailable(available map[string]bool, agents ...string) []string {
	ids := map[string]string{
		"go-backend": "step-1",
		"docs":       "step-2",
		"ci-fixer":   "step-3",
		"security":   "step-4",
		"qa":         "step-5",
		"reviewer":   "step-6",
	}
	var deps []string
	for _, agentName := range agents {
		if available[agentName] {
			deps = append(deps, ids[agentName])
		}
	}
	return deps
}

// Execute runs all subtasks in the plan according to the configured strategy.
func (o *Orchestrator) Execute(ctx context.Context, plan *TaskPlan) ([]SubtaskResult, error) {
	return o.ExecuteWithObserver(ctx, plan, nil)
}

// ExecuteWithObserver runs all subtasks and emits progress events as each subtask changes state.
func (o *Orchestrator) ExecuteWithObserver(ctx context.Context, plan *TaskPlan, observer SubtaskObserver) ([]SubtaskResult, error) {
	var results []SubtaskResult

	switch o.strategy {
	case StrategySequential:
		results = o.executeSequential(ctx, plan, observer)
	case StrategyParallel:
		results = o.executeParallel(ctx, plan, observer)
	}

	if err := executionError(results); err != nil {
		return results, err
	}
	return results, nil
}

func (o *Orchestrator) executeSequential(ctx context.Context, plan *TaskPlan, observer SubtaskObserver) []SubtaskResult {
	results := make([]SubtaskResult, 0, len(plan.Subtasks))
	subtasksByID := make(map[string]Subtask, len(plan.Subtasks))
	completed := make(map[string]bool, len(plan.Subtasks))
	successful := make(map[string]bool, len(plan.Subtasks))
	sharedCtx := ""
	for _, subtask := range plan.Subtasks {
		subtasksByID[subtask.ID] = subtask
	}

	for i := range plan.Subtasks {
		subtask := &plan.Subtasks[i]
		if failed, reason := failedDependency(subtask, subtasksByID, completed, successful); failed {
			result := SubtaskResult{SubtaskID: subtask.ID, Success: false, Error: reason}
			results = append(results, result)
			completed[subtask.ID] = true
			successful[subtask.ID] = false
			emitSyntheticCompletion(subtask, &result, observer)
			continue
		}
		if !dependenciesSatisfied(subtask, completed, successful) {
			result := SubtaskResult{SubtaskID: subtask.ID, Success: false, Error: "dependencies could not be satisfied"}
			results = append(results, result)
			completed[subtask.ID] = true
			successful[subtask.ID] = false
			emitSyntheticCompletion(subtask, &result, observer)
			continue
		}

		result := o.executeObservedSubtask(ctx, subtask, sharedCtx, observer)
		results = append(results, result)
		completed[subtask.ID] = true
		successful[subtask.ID] = result.Success
		if result.Diff != "" {
			sharedCtx = result.Diff
		}
	}
	return results
}

func emitSyntheticCompletion(subtask *Subtask, result *SubtaskResult, observer SubtaskObserver) {
	if observer == nil {
		return
	}
	now := time.Now().UTC()
	observer(SubtaskEvent{Type: SubtaskCompleted, Subtask: *subtask, Result: result, Finished: now})
}

type indexedSubtaskResult struct {
	result SubtaskResult
	index  int
}

func (o *Orchestrator) executeParallel(ctx context.Context, plan *TaskPlan, observer SubtaskObserver) []SubtaskResult {
	results := make([]SubtaskResult, len(plan.Subtasks))
	subtasksByID := make(map[string]Subtask, len(plan.Subtasks))
	for _, subtask := range plan.Subtasks {
		subtasksByID[subtask.ID] = subtask
	}

	started := make(map[string]bool, len(plan.Subtasks))
	completed := make(map[string]bool, len(plan.Subtasks))
	successful := make(map[string]bool, len(plan.Subtasks))
	ch := make(chan indexedSubtaskResult, len(plan.Subtasks))
	running := 0

	for len(completed) < len(plan.Subtasks) {
		progressed := false
		for i, subtask := range plan.Subtasks {
			if started[subtask.ID] || completed[subtask.ID] {
				continue
			}
			if failed, reason := failedDependency(&subtask, subtasksByID, completed, successful); failed {
				result := SubtaskResult{SubtaskID: subtask.ID, Success: false, Error: reason}
				results[i] = result
				completed[subtask.ID] = true
				if observer != nil {
					now := time.Now().UTC()
					observer(SubtaskEvent{Type: SubtaskCompleted, Subtask: subtask, Result: &result, Finished: now})
				}
				progressed = true
				continue
			}
			if !dependenciesSatisfied(&subtask, completed, successful) {
				continue
			}

			started[subtask.ID] = true
			running++
			progressed = true
			go func(index int) {
				ch <- indexedSubtaskResult{o.executeObservedSubtask(ctx, &plan.Subtasks[index], "", observer), index}
			}(i)
		}

		if len(completed) == len(plan.Subtasks) {
			break
		}
		if running == 0 {
			if !progressed {
				for i, subtask := range plan.Subtasks {
					if completed[subtask.ID] {
						continue
					}
					result := SubtaskResult{
						SubtaskID: subtask.ID,
						Success:   false,
						Error:     "dependencies could not be satisfied",
					}
					results[i] = result
					completed[subtask.ID] = true
					if observer != nil {
						now := time.Now().UTC()
						observer(SubtaskEvent{Type: SubtaskCompleted, Subtask: subtask, Result: &result, Finished: now})
					}
				}
			}
			continue
		}

		sr := <-ch
		running--
		results[sr.index] = sr.result
		completed[sr.result.SubtaskID] = true
		successful[sr.result.SubtaskID] = sr.result.Success
	}

	return results
}

func failedDependency(subtask *Subtask, subtasksByID map[string]Subtask, completed, successful map[string]bool) (failed bool, reason string) {
	for _, dep := range subtask.Deps {
		if _, ok := subtasksByID[dep]; !ok {
			return true, fmt.Sprintf("dependency %q was not found", dep)
		}
		if completed[dep] && !successful[dep] {
			return true, fmt.Sprintf("dependency %q failed", dep)
		}
	}
	return false, ""
}

func dependenciesSatisfied(subtask *Subtask, completed, successful map[string]bool) bool {
	for _, dep := range subtask.Deps {
		if !completed[dep] || !successful[dep] {
			return false
		}
	}
	return true
}

func executionError(results []SubtaskResult) error {
	var failed []string
	for _, result := range results {
		if !result.Success {
			if result.Error != "" {
				failed = append(failed, fmt.Sprintf("%s: %s", result.SubtaskID, result.Error))
			} else {
				failed = append(failed, result.SubtaskID)
			}
		}
	}
	if len(failed) == 0 {
		return nil
	}
	return errors.New("subtasks failed: " + strings.Join(failed, "; "))
}

func (o *Orchestrator) executeObservedSubtask(ctx context.Context, subtask *Subtask, sharedCtx string, observer SubtaskObserver) SubtaskResult {
	started := time.Now().UTC()
	if observer != nil {
		observer(SubtaskEvent{Type: SubtaskStarted, Subtask: *subtask, Started: started})
	}

	runCtx := ctx
	cancel := func() {}
	if o.subtaskTimeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, o.subtaskTimeout)
	}
	defer cancel()

	result := o.executeSubtask(runCtx, subtask, sharedCtx)
	if runCtx.Err() == context.DeadlineExceeded && !result.Success && result.Error == "" {
		result.Success = false
		result.Error = fmt.Sprintf("subtask timed out after %s", o.subtaskTimeout)
	}
	finished := time.Now().UTC()
	if observer != nil {
		observer(SubtaskEvent{Type: SubtaskCompleted, Subtask: *subtask, Result: &result, Started: started, Finished: finished})
	}
	return result
}

func (o *Orchestrator) executeSubtask(ctx context.Context, subtask *Subtask, sharedCtx string) SubtaskResult {
	agt, ok := o.agents[subtask.AgentName]
	if !ok {
		agt = o.DefaultAgent()
	}
	if agt == nil {
		return SubtaskResult{
			SubtaskID: subtask.ID,
			Success:   false,
			Error:     "no agent available",
		}
	}

	tk := &task.Task{
		ID:          o.runtimeTaskID(subtask.ID),
		Type:        "orchestrated_subtask",
		Repo:        o.sandbox.RootDir(),
		BaseBranch:  o.baseBranch,
		Title:       subtask.Description,
		Description: subtask.Description,
		Branch:      fmt.Sprintf("agentos/%s", o.runtimeTaskID(subtask.ID)),
	}

	prof, ok := o.agentProfiles[agt.Name()]
	if !ok {
		prof = subtaskProfile(agt.Name())
	}
	runSandbox := sandbox.NewLocalSandbox(o.sandbox.RootDir())
	rt := runtime.NewRuntime(o.llm, &prof, runSandbox, o.cfg, agt)
	if err := rt.Run(ctx, tk); err != nil {
		if result, ok := o.recoverBuiltInSubtask(ctx, subtask, runSandbox, err); ok {
			return result
		}
		return SubtaskResult{
			SubtaskID: subtask.ID,
			Success:   false,
			Error:     err.Error(),
		}
	}
	if result, ok := o.recoverNoOpBuiltInSubtask(ctx, subtask, runSandbox); ok {
		return result
	}
	gateStatus := validateQualityGate(ctx, runSandbox.RootDir(), subtask.QualityGate)
	if !gateStatus.Passed {
		if result, ok := o.recoverNoOpBuiltInSubtaskWithStatus(ctx, subtask, runSandbox, gateStatus); ok {
			return result
		}
		return SubtaskResult{
			SubtaskID:   subtask.ID,
			Success:     false,
			Error:       qualityGateError(gateStatus),
			QualityGate: &gateStatus,
		}
	}

	return SubtaskResult{
		SubtaskID:   subtask.ID,
		Success:     true,
		Output:      fmt.Sprintf("Executed by %s: %s", agt.Name(), subtask.Description),
		Diff:        sharedCtx,
		QualityGate: &gateStatus,
	}
}

func (o *Orchestrator) runtimeTaskID(subtaskID string) string {
	if o.runID == "" {
		return subtaskID
	}
	return o.runID + "-" + subtaskID
}

func subtaskProfile(agentName string) profile.Profile {
	prof := profile.DefaultProfile()
	prof.Name = agentName

	switch agentName {
	case "go-backend":
		prof.Role = "Go backend coding agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = "go test ./..."
		prof.Commands.Lint = "go vet ./..."
		prof.Commands.Build = "go build ./..."
	case "ci-fixer":
		prof.Role = "CI configuration fix agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = "go test ./..."
		prof.Commands.Lint = "go vet ./..."
	case "docs":
		prof.Role = "Documentation agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git"}
		prof.Commands.Test = ""
		prof.Commands.Lint = ""
	case "reviewer":
		prof.Role = "Code review agent"
		prof.Tools.Allow = []string{"read_file", "search", "shell", "git"}
		prof.Commands.Test = ""
		prof.Commands.Lint = ""
		prof.Limits.MaxRetries = 1
		prof.Limits.MaxIterations = 2
	case "security":
		prof.Role = "Security review and remediation agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = "go test ./..."
		prof.Commands.Lint = "go vet ./..."
	case "release-manager":
		prof.Role = "Release preparation agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git"}
		prof.Commands.Test = ""
		prof.Commands.Lint = ""
		prof.Limits.MaxRetries = 1
		prof.Limits.MaxIterations = 4
	case "dependency-updater":
		prof.Role = "Dependency update agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = "go test ./..."
		prof.Commands.Lint = "go vet ./..."
	case "qa":
		prof.Role = "QA and verification agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = "go test ./..."
		prof.Commands.Lint = ""
	}

	return prof
}

// MergeResults combines subtask results into a formatted report.
func (o *Orchestrator) MergeResults(results []SubtaskResult) string {
	redactor := safety.NewRedactor()
	var b strings.Builder
	b.WriteString("# Multi-Agent Execution Results\n\n")
	for _, r := range results {
		status := "PASS"
		if !r.Success {
			status = "FAIL"
		}
		b.WriteString(fmt.Sprintf("## [%s] %s\n", status, r.SubtaskID))
		if r.Output != "" {
			b.WriteString(fmt.Sprintf("%s\n", redactor.RedactString(r.Output)))
		}
		if r.Error != "" {
			b.WriteString(fmt.Sprintf("Error: %s\n", redactor.RedactString(r.Error)))
		}
		if r.QualityGate != nil {
			gate := "PASS"
			if !r.QualityGate.Passed {
				gate = "FAIL"
			}
			b.WriteString(fmt.Sprintf("Quality gate: %s\n", gate))
		}
		b.WriteString("\n")
	}
	return b.String()
}
