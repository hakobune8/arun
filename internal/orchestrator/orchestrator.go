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
	maxRetries     int
	runID          string
}

// AgentMetadata describes an agent to the planner.
type AgentMetadata struct {
	Name                 string
	Description          string
	Domains              []string
	TriggerKeywords      []string
	TriggerFiles         []string
	RecommendedAfter     []string
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

// SetMaxRetries overrides the retry count for each subtask runtime when positive.
func (o *Orchestrator) SetMaxRetries(maxRetries int) {
	o.maxRetries = maxRetries
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
	for i := range o.agentDefs {
		info := &o.agentDefs[i]
		agentsInfo += fmt.Sprintf("- %s: %s\n", info.Name, info.Description)
		if len(info.Domains) > 0 {
			agentsInfo += fmt.Sprintf("  Capabilities/domains: %s\n", strings.Join(info.Domains, ", "))
		}
		if len(info.TriggerKeywords) > 0 {
			agentsInfo += fmt.Sprintf("  Route when task mentions: %s\n", strings.Join(info.TriggerKeywords, ", "))
		}
		if len(info.TriggerFiles) > 0 {
			agentsInfo += fmt.Sprintf("  Route when repository contains: %s\n", strings.Join(info.TriggerFiles, ", "))
		}
		if len(info.RecommendedAfter) > 0 {
			agentsInfo += fmt.Sprintf("  Usually runs after: %s\n", strings.Join(info.RecommendedAfter, ", "))
		}
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
		case "analyst":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete analyst requirements: inspect available run records, artifacts, logs, GitHub context, repository files, memory, and guidelines; cite source provenance for each finding; separate observed facts from inferences; redact obvious secrets and keep log excerpts short.")
		case "reporter":
			plan.Subtasks[i].Description = appendContext(plan.Subtasks[i].Description, parentTask,
				"Concrete reporter requirements: turn findings into a structured Markdown report using the requested output language and templates when provided; include summary, scope, evidence, findings, recommendations, risks, and open questions; avoid overstating uncertain evidence.")
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
	for i := range o.agentDefs {
		info := &o.agentDefs[i]
		available[info.Name] = true
	}

	var subtasks []Subtask
	step := 1
	addedAgents := map[string]bool{}
	idByAgent := map[string]string{}
	add := func(id, agentName, description string, deps []string) {
		if !available[agentName] || addedAgents[agentName] {
			return
		}
		subtasks = append(subtasks, Subtask{
			ID:          id,
			AgentName:   agentName,
			Description: description,
			Deps:        deps,
		})
		addedAgents[agentName] = true
		idByAgent[agentName] = id
		applyDefaultQualityGate(&subtasks[len(subtasks)-1])
	}
	addNext := func(agentName, description string, deps []string) {
		add(fmt.Sprintf("step-%d", step), agentName, description, deps)
		step++
	}
	hasAny := func(words ...string) bool {
		text := strings.ToLower(taskDesc)
		for _, word := range words {
			if strings.Contains(text, word) {
				return true
			}
		}
		return false
	}
	depsFor := func(agents ...string) []string {
		var deps []string
		for _, agentName := range agents {
			if id, ok := idByAgent[agentName]; ok {
				deps = append(deps, id)
			}
		}
		return deps
	}

	switch {
	case hasAny("frontend", "react", "tailwind", "css", "browser", "responsive", "vite", "next.js", "nextjs", "vue", "nuxt", "svelte", "component", "page", "layout", "accessibility"):
		addNext("frontend", fmt.Sprintf("Implement the frontend or UI change requested by the parent task using the repository's existing frontend conventions. Inspect package.json, framework config, routing, components, styling, and state management first; keep controls responsive and accessible. Detect and run available package scripts such as lint, typecheck, test, and build when present. Parent task:\n\n%s", taskDesc), nil)
		addNext("qa", fmt.Sprintf("Add or run focused frontend validation for the requested UI change. Prefer existing npm, pnpm, yarn, or bun scripts, browser smoke checks, responsive checks, and manual verification notes when automation is incomplete. Parent task:\n\n%s", taskDesc), depsFor("frontend"))
	case hasAny("analyze", "investigate", "investigation", "root cause", "rca", "logs", "artifacts", "run history", "failure pattern", "trend", "report", "summary", "release readiness", "repository health", "incident"):
		addNext("analyst", fmt.Sprintf("Investigate the requested repository or operations question. Inspect available run records, artifacts, logs, GitHub issues or PRs, checks, repository files, memory, and guidelines; cite provenance and distinguish observed facts from inferences. Parent task:\n\n%s", taskDesc), nil)
		addNext("reporter", fmt.Sprintf("Create a structured Markdown report from the investigation findings. Include summary, scope, evidence, findings, root cause or likely causes, impact, recommendations, risks, and open questions; use requested language or templates when available. Parent task:\n\n%s", taskDesc), depsFor("analyst"))
	case hasAny("docker", "helm", "kubernetes", "k8s", "ingress", "container", "cluster", "deployment", "chart", "manifest", "rollout"):
		addNext("devops", fmt.Sprintf("Coordinate the deployment or ops-oriented change requested by the parent task. Inspect the full path from container image to Helm chart, Kubernetes manifests, rollout, and rollback before choosing the smallest safe change. Parent task:\n\n%s", taskDesc), nil)
		addNext("docker", fmt.Sprintf("Handle Dockerfile, image build, .dockerignore, compose, and container runtime changes requested by the parent task. Preserve existing build conventions, avoid leaking secrets into layers, and validate with docker build when available or static Dockerfile checks otherwise. Parent task:\n\n%s", taskDesc), depsFor("devops"))
		addNext("helm", fmt.Sprintf("Handle Helm chart, values, schema, template, and chart release changes requested by the parent task. Preserve chart layout and values compatibility, and validate with helm lint/template when available. Parent task:\n\n%s", taskDesc), depsFor("devops", "docker"))
		addNext("kubernetes", fmt.Sprintf("Handle Kubernetes manifests, ingress, service, deployment, probes, resources, securityContext, and rollout verification requested by the parent task. Keep secrets out of manifests and validate YAML plus kubectl dry-run when available. Parent task:\n\n%s", taskDesc), depsFor("devops", "docker", "helm"))
		addNext("security", fmt.Sprintf("Review container, Helm, Kubernetes, secret, permission, and ingress security implications for the requested ops change. Add tests or manual verification notes where useful. Parent task:\n\n%s", taskDesc), depsFor("docker", "helm", "kubernetes"))
		addNext("qa", fmt.Sprintf("Add or document deployment smoke checks for the requested ops change, including concrete commands or cluster verification steps when automation is incomplete. Parent task:\n\n%s", taskDesc), depsFor("docker", "helm", "kubernetes"))
	case hasAny("security", "vulnerability", "cve", "secret", "xss", "csrf", "sql injection", "permission", "authz", "codeql"):
		addNext("security", fmt.Sprintf("Review and fix the security-sensitive work requested by the parent task. Inspect dependencies, auth/session handling, secrets, permissions, and security-relevant configuration before changing code. Parent task:\n\n%s", taskDesc), nil)
		addNext("qa", fmt.Sprintf("Add focused regression or manual verification for the security-sensitive change. Parent task:\n\n%s", taskDesc), depsFor("security"))
	case hasAny("docs", "documentation", "readme", "guide", "manual"):
		addNext("docs", fmt.Sprintf("Update README.md or docs/ for the requested documentation task. Inspect existing documentation style first and keep examples copy-pasteable. Parent task:\n\n%s", taskDesc), nil)
	default:
		addNext("go-backend", fmt.Sprintf("Implement the Go backend requested by the parent task. Inspect the existing repository layout first and preserve established cmd/, internal/, pkg/, api/, router, middleware, and package conventions. Prefer idiomatic standard-library Go for small services and avoid unnecessary layout churn. Create go.mod only if missing. Parent task:\n\n%s", taskDesc), nil)
		addNext("docs", fmt.Sprintf("Update README.md or docs/ for the requested changes. Inspect existing documentation style first. Include practical startup, configuration, endpoint, testing, deployment, and troubleshooting details where relevant. Parent task:\n\n%s", taskDesc), nil)
		addNext("ci-fixer", fmt.Sprintf("Add or fix Go tests and GitHub Actions workflow so go test ./... succeeds for the implementation requested by the parent task. Inspect existing workflow conventions first and prefer checkout/setup-go with cache-aware Go setup plus explicit go test and go vet steps. Parent task:\n\n%s", taskDesc), depsFor("go-backend"))
	}

	if hasAny("dependency", "dependencies", "upgrade", "bump", "go.sum", "package-lock", "pnpm-lock", "yarn.lock") {
		addNext("dependency-updater", fmt.Sprintf("Update the dependencies requested by the parent task. Inspect manifests, lockfiles, toolchain versions, and CI compatibility first; keep manifests and lockfiles synchronized. Parent task:\n\n%s", taskDesc), depsFor("security"))
	}
	if hasAny("ci", "github actions", "workflow", "check failed", "lint", "build failure", "test failure") {
		addNext("ci-fixer", fmt.Sprintf("Fix CI or workflow validation requested by the parent task. Preserve existing workflow intent and mirror CI commands locally where practical. Parent task:\n\n%s", taskDesc), depsFor("go-backend", "dependency-updater"))
	}
	if hasAny("release", "changelog", "version", "release notes", "rollback") {
		addNext("release-manager", fmt.Sprintf("Prepare release artifacts requested by the parent task. Inspect changelog, versioning, Helm chart, deployment, and rollback conventions before editing release files. Parent task:\n\n%s", taskDesc), depsFor("go-backend", "ci-fixer", "qa"))
	}
	if hasAny("report", "summary", "stakeholder", "incident report", "release readiness", "repository health") {
		addNext("reporter", fmt.Sprintf("Prepare or refine the requested report or stakeholder summary. Follow requested output language and template controls when available, and keep facts, inferences, risks, recommendations, and open questions distinct. Parent task:\n\n%s", taskDesc), depsFor("analyst", "security", "qa", "release-manager"))
	}
	if !hasAny("docs", "documentation", "readme", "guide", "manual", "report", "summary") {
		addNext("docs", fmt.Sprintf("Update relevant documentation for the requested changes when user-visible behavior, commands, deployment, or configuration changed. Parent task:\n\n%s", taskDesc), depsFor("go-backend", "release-manager"))
	}
	addNext("reviewer", fmt.Sprintf("Review the final diff for correctness, tests, security, maintainability, release readiness, routing fit, and convention preservation. Flag over-engineered or convention-breaking changes with severity and file references. Parent task:\n\n%s", taskDesc), depsFor("go-backend", "frontend", "docs", "ci-fixer", "security", "release-manager", "dependency-updater", "qa", "analyst", "reporter", "devops", "docker", "helm", "kubernetes"))

	if len(subtasks) == 0 {
		for i := range o.agentDefs {
			info := &o.agentDefs[i]
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
		info.Domains = []string{"backend", "go", "api", "service"}
		info.TriggerKeywords = []string{"go", "backend", "api", "server", "handler", "endpoint", "database", "service"}
		info.TriggerFiles = []string{"go.mod", "go.sum", "cmd/", "internal/", "pkg/", "api/"}
		info.ArchitectureGuidance = []string{
			"Inspect existing layout before editing and follow established package, cmd/, internal/, pkg/, api/, router, and middleware conventions when present.",
			"Prefer idiomatic standard-library Go for small services; introduce frameworks or new top-level layout only when task complexity warrants it.",
			"Separate handlers, configuration, and tests when the repository already uses that structure; avoid over-engineering small repositories.",
		}
		info.OutputExpectations = []string{"gofmt, go test ./..., and go vet ./... pass.", "Architecture choices are summarized when new structure is introduced."}
	case "frontend":
		info.Description = "Frontend application agent for UI implementation, responsive layout, accessibility, and frontend validation"
		info.Domains = []string{"frontend", "ui", "web-app", "responsive", "accessibility", "visual-validation"}
		info.TriggerKeywords = []string{"frontend", "ui", "react", "vite", "next.js", "nextjs", "vue", "nuxt", "svelte", "sveltekit", "tailwind", "css", "layout", "responsive", "accessibility", "browser", "component", "page"}
		info.TriggerFiles = []string{"package.json", "vite.config.ts", "vite.config.js", "next.config.js", "next.config.mjs", "nuxt.config.ts", "svelte.config.js", "tailwind.config.js", "src/", "app/", "pages/", "components/", "public/", "index.html"}
		info.ArchitectureGuidance = []string{
			"Inspect package.json, framework config, routing, component structure, styling conventions, and state management before editing UI code.",
			"Preserve existing framework and design-system patterns; prefer existing components, utilities, tokens, and CSS conventions over new dependencies.",
			"Keep layouts responsive and accessible with semantic markup, keyboard-friendly controls, sensible labels, and mobile plus desktop verification where practical.",
		}
		info.OutputExpectations = []string{"UI implementation changes touch relevant component, page, style, or asset files instead of reporting no-op success.", "Available package scripts such as lint, typecheck, test, and build are detected and run when present.", "Browser, screenshot, responsive, or manual verification notes are included when visual behavior cannot be fully automated."}
	case "ci-fixer":
		info.Description = "CI fix agent for conventional GitHub Actions and validation repairs"
		info.Domains = []string{"ci", "github-actions", "validation"}
		info.TriggerKeywords = []string{"ci", "github actions", "workflow", "check failed", "lint", "build failure", "test failure"}
		info.TriggerFiles = []string{".github/workflows/"}
		info.RecommendedAfter = []string{"go-backend", "dependency-updater"}
		info.ArchitectureGuidance = []string{
			"Inspect existing workflow names, jobs, matrices, and branch-protection expectations before replacing CI structure.",
			"Prefer actions/checkout, actions/setup-go, cache-aware Go setup, go test ./..., and go vet ./...",
			"Keep lint, test, and optional security steps explicit and compatible with the repository's existing Go version and module layout.",
		}
		info.OutputExpectations = []string{"Workflow YAML preserves existing job intent.", "Local validation mirrors the workflow where practical."}
	case "docs":
		info.Description = "Documentation agent that updates practical docs while matching existing repository style"
		info.Domains = []string{"documentation", "developer-experience", "release-notes"}
		info.TriggerKeywords = []string{"docs", "documentation", "readme", "guide", "manual", "quickstart", "changelog"}
		info.TriggerFiles = []string{"README.md", "docs/", "CHANGELOG.md", ".agentos/config.yaml"}
		info.RecommendedAfter = []string{"go-backend", "release-manager"}
		info.ArchitectureGuidance = []string{
			"Inspect README.md and docs/ structure before adding sections or files.",
			"Prefer overview, quickstart, configuration, endpoints, testing, deployment, and troubleshooting sections where relevant.",
			"Preserve existing tone, headings, examples, and link conventions.",
		}
		info.OutputExpectations = []string{"Docs cover changed user-visible behavior.", "Commands and examples are runnable from the repository root."}
	case "reviewer":
		info.Description = "Code review agent for correctness, tests, security, maintainability, and release readiness"
		info.Domains = []string{"review", "quality", "release-readiness"}
		info.TriggerKeywords = []string{"review", "diff", "approval", "risk", "maintainability", "release readiness"}
		info.TriggerFiles = []string{".github/", "go.mod", "package.json", "Dockerfile", "charts/", "k8s/", "deploy/"}
		info.RecommendedAfter = []string{"go-backend", "ci-fixer", "docs", "security", "release-manager", "dependency-updater", "qa"}
		info.ArchitectureGuidance = []string{
			"Evaluate whether changes preserve existing repository conventions before judging style preferences.",
			"Flag over-engineered layouts, unnecessary dependencies, and convention-breaking rewrites.",
			"Review tests, security-sensitive behavior, maintainability, and release readiness with severity and file references.",
		}
		info.OutputExpectations = []string{"Findings include severity and file references where applicable.", "Review states validation and release-readiness risk."}
	case "security":
		info.Description = "Security agent for dependencies, auth/session handling, secrets, and security-sensitive diffs"
		info.Domains = []string{"security", "auth", "secrets", "dependencies"}
		info.TriggerKeywords = []string{"security", "vulnerability", "cve", "secret", "xss", "csrf", "sql injection", "permission", "authz", "codeql"}
		info.TriggerFiles = []string{"SECURITY.md", ".github/workflows/codeql.yml", ".github/dependabot.yml", "go.sum", "package-lock.json"}
		info.RecommendedAfter = []string{"go-backend", "dependency-updater"}
		info.ArchitectureGuidance = []string{
			"Inspect authentication, authorization, session, secret-handling, dependency, and CI security conventions before proposing changes.",
			"Prefer small defensive fixes, safer defaults, and standard library or existing dependency patterns over broad rewrites.",
			"Document residual risk and validation scope when a finding cannot be fully fixed in the current task.",
		}
		info.OutputExpectations = []string{"Security-sensitive changes include tests or manual verification notes.", "Dependency or configuration findings identify the affected package, file, workflow, or setting.", "go test ./... and go vet ./... pass when code is changed."}
	case "release-manager":
		info.Description = "Release manager agent for changelogs, release notes, release checklists, and readiness validation"
		info.Domains = []string{"release", "deployment", "helm", "kubernetes", "docker"}
		info.TriggerKeywords = []string{"release", "changelog", "version", "rollback", "helm", "kubernetes", "k8s", "docker", "deployment", "ingress"}
		info.TriggerFiles = []string{"CHANGELOG.md", "charts/", "Chart.yaml", "values.yaml", "Dockerfile", "k8s/", "deploy/", "deployment.yaml", "ingress.yaml"}
		info.RecommendedAfter = []string{"go-backend", "ci-fixer", "qa", "security"}
		info.ArchitectureGuidance = []string{
			"Inspect existing changelog, release note, versioning, and Helm chart conventions before editing release artifacts.",
			"Keep version changes explicit and avoid publishing or tagging releases unless the task asks for it.",
			"Summarize release readiness, known gaps, and deployment or rollback considerations.",
		}
		info.OutputExpectations = []string{"CHANGELOG.md or release documentation is updated when release notes are requested.", "Version and chart changes are consistent when release packaging is in scope.", "Release checklist items are concrete and traceable to validation commands or manual checks."}
	case "docker":
		info.Description = "Docker ops agent for Dockerfiles, image builds, .dockerignore, compose, and container safety"
		info.Domains = []string{"docker", "containers", "images", "compose", "container-security"}
		info.TriggerKeywords = []string{"docker", "dockerfile", "container", "image", "buildkit", "compose", ".dockerignore", "multi-stage", "healthcheck"}
		info.TriggerFiles = []string{"Dockerfile", "Dockerfile.*", ".dockerignore", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}
		info.RecommendedAfter = []string{"security", "dependency-updater"}
		info.ArchitectureGuidance = []string{
			"Inspect the existing Dockerfile, compose files, build context, entrypoint, exposed ports, and CI image-build flow before editing container files.",
			"Prefer multi-stage builds, non-root runtime users, minimal copied context, deterministic package installation, and explicit health checks when the application supports them.",
			"Keep secrets out of image layers, build args, labels, logs, and compose files.",
		}
		info.OutputExpectations = []string{"Container changes touch Dockerfile, .dockerignore, compose, or related build configuration instead of reporting no-op success.", "docker build is run when available; otherwise static Dockerfile checks and unavailable-tool notes are reported.", "Security and runtime notes cover user, ports, health checks, secret handling, image size, and rollback considerations."}
	case "helm":
		info.Description = "Helm ops agent for charts, templates, values, schema, chart linting, and release-safe packaging"
		info.Domains = []string{"helm", "charts", "templates", "values", "kubernetes-packaging"}
		info.TriggerKeywords = []string{"helm", "chart", "charts", "values.yaml", "values.schema.json", "template", "helm lint", "helm template", "appversion"}
		info.TriggerFiles = []string{"charts/", "Chart.yaml", "values.yaml", "values.schema.json", "templates/", "Chart.lock"}
		info.RecommendedAfter = []string{"docker", "kubernetes", "security"}
		info.ArchitectureGuidance = []string{
			"Inspect chart layout, helper templates, values defaults, schema, labels, annotations, and release versioning before changing templates.",
			"Preserve existing values structure and helper conventions; avoid hard-coded environment-specific data in templates.",
			"Prefer conservative Kubernetes defaults, schema-backed values, stable labels/selectors, and explicit upgrade or rollback notes for chart changes.",
		}
		info.OutputExpectations = []string{"Helm changes touch Chart.yaml, values, templates, schema, or chart tests instead of reporting no-op success.", "helm lint and helm template are run when available; otherwise structure checks and unavailable-tool notes are reported.", "Chart version, appVersion, values compatibility, and upgrade impact are summarized when packaging or release behavior changes."}
	case "kubernetes":
		info.Description = "Kubernetes ops agent for manifests, deployments, services, ingress, probes, resources, rollout checks, and safe defaults"
		info.Domains = []string{"kubernetes", "k8s", "manifests", "deployments", "services", "ingress", "rollouts"}
		info.TriggerKeywords = []string{"kubernetes", "k8s", "manifest", "deployment", "service", "ingress", "configmap", "secret", "probe", "resources", "rollout", "kubectl"}
		info.TriggerFiles = []string{"k8s/", "kubernetes/", "manifests/", "deploy/", "deployment.yaml", "service.yaml", "ingress.yaml", "configmap.yaml", "secret.yaml"}
		info.RecommendedAfter = []string{"docker", "helm", "security"}
		info.ArchitectureGuidance = []string{
			"Inspect existing manifest layout, namespace assumptions, labels/selectors, service ports, ingress, probes, resources, and rollout conventions before editing.",
			"Prefer standard Kubernetes objects, conservative resource/security defaults, stable selectors, readiness/liveness probes, and non-privileged containers unless explicitly required.",
			"Keep secrets out of manifests and logs; use Secret references, sealed/external secret systems, or documented runtime configuration instead.",
		}
		info.OutputExpectations = []string{"Kubernetes changes touch manifests, kustomize, Helm-rendered resources, or deployment docs instead of reporting no-op success.", "YAML parses successfully and kubectl dry-run is run when kubectl and context are available.", "Rollout, rollback, probe, resource, securityContext, and secret-handling verification notes are included for deployment changes."}
	case "devops":
		info.Description = "DevOps umbrella agent for Docker, Helm, Kubernetes, deployment debugging, and release hardening"
		info.Domains = []string{"devops", "ops", "deployment", "docker", "helm", "kubernetes", "release-hardening"}
		info.TriggerKeywords = []string{"devops", "ops", "deployment", "release hardening", "rollout", "rollback", "docker", "helm", "kubernetes", "k8s", "cluster"}
		info.TriggerFiles = []string{"Dockerfile", "charts/", "k8s/", "kubernetes/", "deploy/", ".github/workflows/"}
		info.RecommendedAfter = []string{"docker", "helm", "kubernetes", "security", "qa"}
		info.ArchitectureGuidance = []string{
			"Inspect the full delivery path from image build to chart/manifests and rollout before choosing the smallest safe operational change.",
			"Coordinate specialist findings from Docker, Helm, Kubernetes, security, and QA work without duplicating low-level edits unnecessarily.",
			"Prefer conservative release-hardening changes with explicit validation, rollback, and residual-risk notes.",
		}
		info.OutputExpectations = []string{"Ops work is decomposed into relevant Docker, Helm, Kubernetes, security, QA, and review steps when the task spans multiple layers.", "Validation results cite image build, chart render/lint, manifest dry-run, smoke checks, or the reason a tool was unavailable.", "Final notes include deployment impact, rollback path, manual verification, and follow-up risks."}
	case "dependency-updater":
		info.Description = "Dependency updater agent for Go modules, package locks, and GitHub Actions versions"
		info.Domains = []string{"dependencies", "go-modules", "package-locks", "github-actions"}
		info.TriggerKeywords = []string{"dependency", "dependencies", "upgrade", "bump", "go mod", "go.sum", "package-lock", "pnpm-lock", "yarn.lock"}
		info.TriggerFiles = []string{"go.mod", "go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", ".github/dependabot.yml"}
		info.RecommendedAfter = []string{"security"}
		info.ArchitectureGuidance = []string{
			"Inspect existing dependency managers, lockfiles, toolchain versions, and CI compatibility before updating versions.",
			"Prefer narrow updates requested by the task; avoid broad upgrades unless the task calls for them.",
			"Keep generated files such as go.sum or lockfiles consistent with the manifest that changed.",
		}
		info.OutputExpectations = []string{"Manifests and lockfiles remain synchronized after updates.", "go mod tidy and go test ./... pass for Go dependency work.", "Compatibility or breaking-change notes are included for major or security-sensitive upgrades."}
	case "qa":
		info.Description = "QA agent for scenario tests, smoke checks, regression coverage, and manual verification notes"
		info.Domains = []string{"qa", "tests", "smoke", "regression", "frontend-validation"}
		info.TriggerKeywords = []string{"qa", "quality assurance", "smoke", "scenario test", "regression", "manual verification", "browser", "responsive"}
		info.TriggerFiles = []string{"*_test.go", "test/", "tests/", "package.json", "playwright.config.ts", "cypress.config.ts"}
		info.RecommendedAfter = []string{"go-backend", "ci-fixer", "security", "release-manager", "dependency-updater"}
		info.ArchitectureGuidance = []string{
			"Inspect existing test layout, fixtures, and documented verification workflows before adding new checks.",
			"Prefer focused regression and smoke coverage that exercises user-visible behavior changed by the task.",
			"Record manual verification steps when behavior cannot be fully automated.",
		}
		info.OutputExpectations = []string{"New or updated tests fail without the intended behavior and pass with it.", "go test ./... passes when Go code or tests are in scope.", "Manual verification notes include concrete commands, URLs, or scenarios."}
	case "analyst":
		info.Description = "Analyst agent for log, run, artifact, GitHub, and repository-context investigations"
		info.Domains = []string{"analysis", "investigation", "logs", "artifacts", "github", "observability", "root-cause"}
		info.TriggerKeywords = []string{"analyze", "investigate", "investigation", "root cause", "rca", "logs", "artifacts", "run history", "failure pattern", "trend", "evidence", "findings"}
		info.TriggerFiles = []string{"logs/", "artifacts/", ".github/workflows/", "README.md", "docs/", "CHANGELOG.md", ".agentos/"}
		info.ArchitectureGuidance = []string{
			"Gather evidence from run records, artifacts, logs, GitHub issues or PRs, repository files, memory, and guidelines before drawing conclusions.",
			"Separate observed facts from inferences, include confidence level, and call out missing or unavailable sources explicitly.",
			"Redact obvious secrets and keep log excerpts short while preserving enough provenance to verify each finding.",
		}
		info.OutputExpectations = []string{"Investigation output includes summary, scope, evidence, findings, root cause or likely causes, impact, recommendations, and open questions.", "Each finding cites source provenance such as run IDs, file paths, issue or PR numbers, timestamps, or short log excerpts.", "No-data cases state which sources were checked and what could not be determined."}
	case "reporter":
		info.Description = "Reporter agent for Markdown reports, stakeholder summaries, and GitHub-ready updates"
		info.Domains = []string{"reporting", "markdown", "incident-report", "release-readiness", "repository-health", "stakeholder-summary"}
		info.TriggerKeywords = []string{"report", "summary", "summarize", "stakeholder", "incident report", "release readiness", "repository health", "findings", "recommendations", "write-up"}
		info.TriggerFiles = []string{"README.md", "docs/", "CHANGELOG.md", "reports/", ".agentos/"}
		info.RecommendedAfter = []string{"analyst", "security", "qa", "release-manager"}
		info.ArchitectureGuidance = []string{
			"Use the requested output language and repository templates when provided, preserving existing Markdown and documentation conventions.",
			"Convert analyst findings into audience-appropriate reports without overstating uncertain evidence.",
			"Prepare GitHub issue or PR comment text only when enabled by the orchestration settings.",
		}
		info.OutputExpectations = []string{"Reports include summary, scope, evidence, findings, recommendations, risks, and open questions.", "Incident reports include timeline, impact, detection, root cause, mitigation, and prevention when those facts are available.", "Repository health and release readiness reports distinguish blockers, risks, validation status, and recommended next actions."}
	}
	return info
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
	if o.maxRetries > 0 {
		prof.Limits.MaxRetries = o.maxRetries
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
	_ = runCmd(ctx, runSandbox.RootDir(), "git", "add", "-N", ".") //nolint:errcheck // best-effort diff visibility for new files
	diff := gitDiff(ctx, runSandbox.RootDir())
	if subtask.AgentName == "frontend" && strings.TrimSpace(diff) == "" {
		return SubtaskResult{
			SubtaskID: subtask.ID,
			Success:   false,
			Error:     "frontend subtask produced no diff; UI implementation changes are required",
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
		Diff:        firstNonEmpty(diff, sharedCtx),
		QualityGate: &gateStatus,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
	case "frontend":
		prof.Role = "Frontend application agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = frontendValidationCommand
		prof.Commands.Lint = ""
		prof.Commands.Build = ""
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
	case "docker":
		prof.Role = "Docker operations agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = dockerValidationCommand
		prof.Commands.Lint = ""
		prof.Limits.MaxRetries = 1
	case "helm":
		prof.Role = "Helm chart operations agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = helmValidationCommand
		prof.Commands.Lint = ""
		prof.Limits.MaxRetries = 1
	case "kubernetes":
		prof.Role = "Kubernetes operations agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = kubernetesValidationCommand
		prof.Commands.Lint = ""
		prof.Limits.MaxRetries = 1
	case "devops":
		prof.Role = "DevOps coordination agent"
		prof.Tools.Allow = []string{"read_file", "write_file", "search", "shell", "git", "test"}
		prof.Commands.Test = opsValidationCommand
		prof.Commands.Lint = ""
		prof.Limits.MaxRetries = 1
		prof.Limits.MaxIterations = 4
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
