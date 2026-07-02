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

// Package evals runs deterministic orchestration regression scenarios.
package evals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/apphome"
	agentosgh "github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/orchestrator"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/tools"
)

// Mode controls how much of a scenario is exercised.
type Mode string

const (
	// ModePlan validates deterministic planning and routing only.
	ModePlan Mode = "plan"
	// ModeExecute validates planning plus deterministic fallback execution.
	ModeExecute Mode = "execute"
)

// Scenario describes a repeatable orchestration regression case.
type Scenario struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Mode           Mode     `json:"mode"`
	Task           string   `json:"task"`
	Agents         []string `json:"agents"`
	ExpectedAgents []string `json:"expectedAgents"`
	RequiredFiles  []string `json:"requiredFiles,omitempty"`
	FunctionalArea []string `json:"functionalArea,omitempty"`
	Live           bool     `json:"live,omitempty"`
}

// Options controls a suite run.
type Options struct {
	ScenarioIDs                 []string
	WorkDir                     string
	IncludeLive                 bool
	LiveURL                     string
	IncludeAuthE2E              bool
	IncludeStorageCleanupE2E    bool
	IncludeScheduleNotifyE2E    bool
	IncludeGitHubWorkflowE2E    bool
	IncludeKubernetesRolloutE2E bool
	IncludeRealLLMSmokeE2E      bool
}

// Report summarizes one eval suite run.
type Report struct {
	StartedAt    time.Time        `json:"startedAt"`
	FinishedAt   time.Time        `json:"finishedAt"`
	DurationMS   int64            `json:"durationMs"`
	Total        int              `json:"total"`
	Passed       int              `json:"passed"`
	Failed       int              `json:"failed"`
	SuccessRate  float64          `json:"successRate"`
	Coverage     []CoverageArea   `json:"coverage"`
	ScenarioRuns []ScenarioResult `json:"scenarios"`
}

// ScenarioResult summarizes one scenario run.
type ScenarioResult struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Category       string             `json:"category"`
	Mode           Mode               `json:"mode"`
	Passed         bool               `json:"passed"`
	DurationMS     int64              `json:"durationMs"`
	Agents         []string           `json:"agents"`
	ExpectedAgents []string           `json:"expectedAgents"`
	RequiredFiles  []FileCheck        `json:"requiredFiles,omitempty"`
	Subtasks       int                `json:"subtasks"`
	Successes      int                `json:"successes"`
	Failures       int                `json:"failures"`
	FailureReasons []string           `json:"failureReasons,omitempty"`
	Artifacts      map[string]string  `json:"artifacts,omitempty"`
	QualityGates   []QualityGateCheck `json:"qualityGates,omitempty"`
	Checks         []ScenarioCheck    `json:"checks,omitempty"`
}

// CoverageArea summarizes functional scenario coverage by area.
type CoverageArea struct {
	Name      string   `json:"name"`
	Covered   int      `json:"covered"`
	Total     int      `json:"total"`
	Scenarios []string `json:"scenarios"`
}

// FileCheck reports whether a required artifact exists.
type FileCheck struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// QualityGateCheck reports one subtask quality gate result.
type QualityGateCheck struct {
	SubtaskID string `json:"subtaskId"`
	Passed    bool   `json:"passed"`
}

// ScenarioCheck reports one non-file scenario assertion.
type ScenarioCheck struct {
	Page       string `json:"page,omitempty"`
	Action     string `json:"action"`
	Passed     bool   `json:"passed"`
	DurationMS int64  `json:"durationMs,omitempty"`
	Failure    string `json:"failure,omitempty"`
}

// DefaultScenarios returns the built-in deterministic v1.4 scenario suite.
func DefaultScenarios() []Scenario {
	commonAgents := []string{"go-backend", "docs", "ci-fixer", "reviewer", "frontend", "qa", "devops", "docker", "helm", "kubernetes", "security", "analyst", "reporter", "release-manager", "dependency-updater"}
	return []Scenario{
		{
			ID:             "empty-go-service-bootstrap",
			Name:           "Empty repository Go service bootstrap",
			Category:       "golden",
			Mode:           ModeExecute,
			Task:           `Create a minimal Go net/http service in github.com/acme/eval-service with GET / returning text and GET /healthz returning {"status":"ok"}.`,
			Agents:         []string{"go-backend", "docs", "ci-fixer", "reviewer"},
			ExpectedAgents: []string{"go-backend", "docs", "ci-fixer", "reviewer"},
			RequiredFiles:  []string{"go.mod", "main.go", "main_test.go", filepath.Join(".github", "workflows", "go.yml"), "README.md", "REVIEW.md"},
			FunctionalArea: []string{"planning", "fallback-execution", "quality-gates", "required-artifacts", "ci-workflow", "documentation"},
		},
		{
			ID:             "frontend-responsive-change",
			Name:           "Frontend responsive UI routing",
			Category:       "routing",
			Mode:           ModePlan,
			Task:           "Update the React Tailwind frontend layout so the dashboard is responsive on mobile and includes browser smoke validation.",
			Agents:         commonAgents,
			ExpectedAgents: []string{"frontend", "qa", "docs", "reviewer"},
			FunctionalArea: []string{"planning", "agent-routing", "frontend", "qa"},
		},
		{
			ID:             "ci-fix-workflow",
			Name:           "CI failure workflow routing",
			Category:       "routing",
			Mode:           ModePlan,
			Task:           "Fix the failing GitHub Actions CI check for a Go test failure and keep go test ./... passing.",
			Agents:         commonAgents,
			ExpectedAgents: []string{"go-backend", "docs", "ci-fixer", "reviewer"},
			FunctionalArea: []string{"planning", "agent-routing", "ci-workflow"},
		},
		{
			ID:             "ops-investigation",
			Name:           "Docker Helm Kubernetes ops investigation",
			Category:       "routing",
			Mode:           ModePlan,
			Task:           "Fix a Kubernetes rollout failure involving Docker image pull, Helm values, ingress, and rollback notes.",
			Agents:         commonAgents,
			ExpectedAgents: []string{"devops", "docker", "helm", "kubernetes", "security", "qa", "docs", "reviewer"},
			FunctionalArea: []string{"planning", "agent-routing", "docker", "helm", "kubernetes", "security", "qa"},
		},
		{
			ID:             "reporting-workflow",
			Name:           "Investigation and reporting workflow",
			Category:       "routing",
			Mode:           ModePlan,
			Task:           "Analyze the last 24 hours of orchestration logs, identify failure patterns, and write a stakeholder report in Markdown.",
			Agents:         commonAgents,
			ExpectedAgents: []string{"analyst", "reporter", "reviewer"},
			FunctionalArea: []string{"planning", "agent-routing", "analysis", "reporting"},
		},
		{
			ID:             "github-issue-pr-workflow",
			Name:           "Issue and PR workflow routing",
			Category:       "routing",
			Mode:           ModePlan,
			Task:           "Prepare a GitHub issue and pull request workflow update with release notes, CI validation, and reviewer sign-off.",
			Agents:         commonAgents,
			ExpectedAgents: []string{"go-backend", "docs", "ci-fixer", "release-manager", "reviewer"},
			FunctionalArea: []string{"planning", "agent-routing", "github-workflow", "release-readiness", "ci-workflow"},
		},
	}
}

// LiveScenarios returns opt-in checks that require a reachable deployment.
func LiveScenarios() []Scenario {
	return []Scenario{{
		ID:             "live-web-and-api-smoke",
		Name:           "Live web and API smoke",
		Category:       "live",
		Mode:           ModePlan,
		FunctionalArea: []string{"live-smoke", "web-assets", "api-health", "agent-registry", "auth-boundary"},
		Live:           true,
	}}
}

// AuthenticatedWebUIScenarios returns opt-in checks that require browser auth.
func AuthenticatedWebUIScenarios() []Scenario {
	return []Scenario{{
		ID:             "authenticated-webui-e2e",
		Name:           "Authenticated Web UI E2E",
		Category:       "live-auth",
		Mode:           ModePlan,
		FunctionalArea: []string{"live-smoke", "authenticated-webui", "web-navigation", "mobile-layout", "auth-boundary"},
		Live:           true,
	}}
}

// StorageCleanupScenarios returns opt-in checks that require disposable fixtures.
func StorageCleanupScenarios() []Scenario {
	return []Scenario{{
		ID:             "storage-cleanup-e2e",
		Name:           "Storage cleanup dry-run and execution E2E",
		Category:       "live-auth",
		Mode:           ModePlan,
		FunctionalArea: []string{"live-smoke", "storage-usage", "storage-cleanup", "retention-policy", "archive-before-delete", "audit", "auth-boundary"},
		Live:           true,
	}}
}

// ScheduleNotificationScenarios returns opt-in checks for schedule execution notifications.
func ScheduleNotificationScenarios() []Scenario {
	return []Scenario{{
		ID:             "schedule-notification-e2e",
		Name:           "Schedule execution notification E2E",
		Category:       "live-auth",
		Mode:           ModePlan,
		FunctionalArea: []string{"live-smoke", "schedules", "schedule-execution", "notifications", "auth-boundary", "cleanup"},
		Live:           true,
	}}
}

// GitHubWorkflowScenarios returns opt-in checks that create live GitHub artifacts.
func GitHubWorkflowScenarios() []Scenario {
	return []Scenario{{
		ID:             "github-workflow-e2e",
		Name:           "Live GitHub issue and PR workflow E2E",
		Category:       "live-github",
		Mode:           ModePlan,
		FunctionalArea: []string{"github-workflow", "issue-create", "pull-request-create", "check-lookup", "workflow-runs", "cleanup"},
		Live:           true,
	}}
}

// KubernetesRolloutScenarios returns opt-in checks that exercise a live Kubernetes rollout.
func KubernetesRolloutScenarios() []Scenario {
	return []Scenario{{
		ID:             "kubernetes-rollout-e2e",
		Name:           "Live Kubernetes rollout and rollback E2E",
		Category:       "live-kubernetes",
		Mode:           ModePlan,
		FunctionalArea: []string{"kubernetes", "helm", "rollout", "rollback", "readiness", "cleanup"},
		Live:           true,
	}}
}

// RealLLMSmokeScenarios returns opt-in checks that exercise a real LLM orchestration.
func RealLLMSmokeScenarios() []Scenario {
	return []Scenario{{
		ID:             "real-llm-orchestration-smoke",
		Name:           "Real LLM orchestration smoke",
		Category:       "live-llm",
		Mode:           ModeExecute,
		FunctionalArea: []string{"llm", "planning", "fallback-execution", "quality-gates", "required-artifacts", "governance-limits"},
		Live:           true,
	}}
}

// Run executes the selected deterministic eval scenarios.
func Run(ctx context.Context, opts Options) (*Report, error) {
	started := time.Now().UTC()
	scenarios := filterScenarios(DefaultScenarios(), opts.ScenarioIDs)
	if opts.IncludeLive {
		scenarios = append(scenarios, filterScenarios(LiveScenarios(), opts.ScenarioIDs)...)
	}
	if opts.IncludeAuthE2E {
		scenarios = append(scenarios, filterScenarios(AuthenticatedWebUIScenarios(), opts.ScenarioIDs)...)
	}
	if opts.IncludeStorageCleanupE2E {
		scenarios = append(scenarios, filterScenarios(StorageCleanupScenarios(), opts.ScenarioIDs)...)
	}
	if opts.IncludeScheduleNotifyE2E {
		scenarios = append(scenarios, filterScenarios(ScheduleNotificationScenarios(), opts.ScenarioIDs)...)
	}
	if opts.IncludeGitHubWorkflowE2E {
		scenarios = append(scenarios, filterScenarios(GitHubWorkflowScenarios(), opts.ScenarioIDs)...)
	}
	if opts.IncludeKubernetesRolloutE2E {
		scenarios = append(scenarios, filterScenarios(KubernetesRolloutScenarios(), opts.ScenarioIDs)...)
	}
	if opts.IncludeRealLLMSmokeE2E {
		scenarios = append(scenarios, filterScenarios(RealLLMSmokeScenarios(), opts.ScenarioIDs)...)
	}
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no eval scenarios selected")
	}
	workDir := opts.WorkDir
	if strings.TrimSpace(workDir) == "" {
		var err error
		workDir, err = os.MkdirTemp("", "agentos-evals-*")
		if err != nil {
			return nil, err
		}
	}
	report := &Report{StartedAt: started, Total: len(scenarios)}
	for i := range scenarios {
		result := runScenario(ctx, workDir, &scenarios[i], opts)
		report.ScenarioRuns = append(report.ScenarioRuns, result)
		if result.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
	}
	report.FinishedAt = time.Now().UTC()
	report.DurationMS = report.FinishedAt.Sub(report.StartedAt).Milliseconds()
	if report.Total > 0 {
		report.SuccessRate = float64(report.Passed) / float64(report.Total)
	}
	report.Coverage = coverageFor(scenarios, report.ScenarioRuns)
	return report, nil
}

func filterScenarios(all []Scenario, ids []string) []Scenario {
	if len(ids) == 0 {
		return all
	}
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[strings.TrimSpace(id)] = true
	}
	var selected []Scenario
	for i := range all {
		if want[all[i].ID] {
			selected = append(selected, all[i])
		}
	}
	return selected
}

func runScenario(ctx context.Context, workDir string, scenario *Scenario, opts Options) ScenarioResult {
	started := time.Now()
	result := ScenarioResult{
		ID:             scenario.ID,
		Name:           scenario.Name,
		Category:       scenario.Category,
		Mode:           scenario.Mode,
		ExpectedAgents: append([]string(nil), scenario.ExpectedAgents...),
	}
	if scenario.Live {
		return finishResult(started, runLiveScenario(ctx, scenario, opts.LiveURL, &result))
	}
	repo, err := os.MkdirTemp(workDir, scenario.ID+"-*")
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, err.Error())
		return finishResult(started, &result)
	}
	if err := initRepo(repo); err != nil {
		result.FailureReasons = append(result.FailureReasons, err.Error())
		return finishResult(started, &result)
	}

	orch, err := newScenarioOrchestrator(repo, scenario)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, err.Error())
		return finishResult(started, &result)
	}
	plan, err := orch.Plan(ctx, scenario.Task)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "plan: "+err.Error())
		return finishResult(started, &result)
	}
	result.Subtasks = len(plan.Subtasks)
	for i := range plan.Subtasks {
		result.Agents = append(result.Agents, plan.Subtasks[i].AgentName)
	}
	for _, want := range scenario.ExpectedAgents {
		if !slices.Contains(result.Agents, want) {
			result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("missing expected agent %q", want))
		}
	}

	if scenario.Mode == ModeExecute {
		results, err := orch.Execute(ctx, plan)
		if err != nil {
			result.FailureReasons = append(result.FailureReasons, "execute: "+err.Error())
		}
		for _, subtask := range results {
			if subtask.Success {
				result.Successes++
			} else {
				result.Failures++
				if subtask.Error != "" {
					result.FailureReasons = append(result.FailureReasons, subtask.SubtaskID+": "+subtask.Error)
				}
			}
			if subtask.QualityGate != nil {
				result.QualityGates = append(result.QualityGates, QualityGateCheck{SubtaskID: subtask.SubtaskID, Passed: subtask.QualityGate.Passed})
				if !subtask.QualityGate.Passed {
					result.FailureReasons = append(result.FailureReasons, subtask.SubtaskID+": quality gate failed")
				}
			}
		}
		for _, name := range scenario.RequiredFiles {
			path := filepath.Join(repo, name)
			_, statErr := os.Stat(path)
			exists := statErr == nil
			result.RequiredFiles = append(result.RequiredFiles, FileCheck{Path: name, Exists: exists})
			if !exists {
				result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("missing required file %q", name))
			}
		}
		result.Artifacts = map[string]string{
			"repository": repo,
			"diff":       gitDiff(ctx, repo),
		}
	}
	return finishResult(started, &result)
}

func runLiveScenario(ctx context.Context, scenario *Scenario, liveURL string, result *ScenarioResult) *ScenarioResult {
	if scenario.ID == "authenticated-webui-e2e" {
		return runAuthenticatedWebUIScenario(ctx, liveURL, result)
	}
	if scenario.ID == "storage-cleanup-e2e" {
		return runStorageCleanupScenario(ctx, liveURL, result)
	}
	if scenario.ID == "schedule-notification-e2e" {
		return runScheduleNotificationScenario(ctx, liveURL, result)
	}
	if scenario.ID == "github-workflow-e2e" {
		return runGitHubWorkflowScenario(ctx, result)
	}
	if scenario.ID == "kubernetes-rollout-e2e" {
		return runKubernetesRolloutScenario(ctx, result)
	}
	if scenario.ID == "real-llm-orchestration-smoke" {
		return runRealLLMSmokeScenario(ctx, result)
	}
	base := strings.TrimRight(strings.TrimSpace(liveURL), "/")
	if base == "" {
		base = strings.TrimRight(os.Getenv("AGENTOS_EVAL_LIVE_URL"), "/")
	}
	if base == "" {
		result.FailureReasons = append(result.FailureReasons, "live URL is required via --live-url or AGENTOS_EVAL_LIVE_URL")
		return result
	}
	client := &http.Client{Timeout: 15 * time.Second}
	index, err := getText(ctx, client, base+"/")
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "GET /: "+err.Error())
		return result
	}
	js := firstAsset(index, `src="`, `.js`)
	css := firstAsset(index, `href="`, `.css`)
	for _, check := range []struct {
		name string
		path string
	}{
		{"health", "/api/health"},
		{"agents", "/api/agents"},
		{"javascript", js},
		{"css", css},
	} {
		if check.path == "" {
			result.FailureReasons = append(result.FailureReasons, "missing "+check.name+" asset")
			continue
		}
		if _, err := getText(ctx, client, base+check.path); err != nil {
			result.FailureReasons = append(result.FailureReasons, check.name+": "+err.Error())
		}
	}
	status, err := getStatus(ctx, client, base+"/api/storage")
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "storage auth boundary: "+err.Error())
	} else if status != http.StatusUnauthorized && status != http.StatusOK {
		result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("storage auth boundary status = %d, want 401 or authenticated 200", status))
	}
	result.Artifacts = map[string]string{"url": base, "js": js, "css": css}
	return result
}

type authE2EResult struct {
	Checks    []ScenarioCheck   `json:"checks"`
	Artifacts map[string]string `json:"artifacts,omitempty"`
}

func runAuthenticatedWebUIScenario(ctx context.Context, liveURL string, result *ScenarioResult) *ScenarioResult {
	base := strings.TrimRight(strings.TrimSpace(liveURL), "/")
	if base == "" {
		base = strings.TrimRight(os.Getenv("AGENTOS_EVAL_LIVE_URL"), "/")
	}
	if base == "" {
		result.FailureReasons = append(result.FailureReasons, "live URL is required via --live-url or AGENTOS_EVAL_LIVE_URL")
		return result
	}
	script, err := findAuthE2EScript()
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, err.Error())
		return result
	}
	if !authE2EConfigured() {
		result.FailureReasons = append(result.FailureReasons, "authenticated session material is required via AGENTOS_EVAL_AUTH_STORAGE_STATE or AGENTOS_EVAL_AUTH_COOKIE")
		return result
	}
	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Env = append(os.Environ(), "AGENTOS_EVAL_LIVE_URL="+base)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := sanitizeAuthE2EOutput(strings.TrimSpace(stdout.String() + "\n" + stderr.String()))
		if out == "" {
			out = err.Error()
		}
		result.FailureReasons = append(result.FailureReasons, "authenticated Web UI E2E: "+out)
		return result
	}
	var e2e authE2EResult
	if err := json.Unmarshal(stdout.Bytes(), &e2e); err != nil {
		result.FailureReasons = append(result.FailureReasons, "authenticated Web UI E2E report: "+err.Error())
		return result
	}
	for _, check := range e2e.Checks {
		result.Checks = append(result.Checks, check)
		if !check.Passed {
			result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("%s/%s: %s", check.Page, check.Action, check.Failure))
		}
	}
	result.Artifacts = e2e.Artifacts
	if result.Artifacts == nil {
		result.Artifacts = map[string]string{}
	}
	result.Artifacts["url"] = base
	result.Artifacts["script"] = script
	return result
}

func authE2EConfigured() bool {
	return strings.TrimSpace(os.Getenv("AGENTOS_EVAL_AUTH_STORAGE_STATE")) != "" || authCookieConfigured()
}

func authCookieConfigured() bool {
	return strings.TrimSpace(os.Getenv("AGENTOS_EVAL_AUTH_COOKIE")) != ""
}

func findAuthE2EScript() (string, error) {
	if script := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_AUTH_E2E_SCRIPT")); script != "" {
		if _, err := os.Stat(script); err != nil {
			return "", fmt.Errorf("authenticated Web UI E2E script %q: %w", script, err)
		}
		return script, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(wd, "web", "scripts", "auth-e2e.mjs")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return "", fmt.Errorf("authenticated Web UI E2E script not found; set AGENTOS_EVAL_AUTH_E2E_SCRIPT")
}

func sanitizeAuthE2EOutput(out string) string {
	for _, key := range []string{"AGENTOS_EVAL_AUTH_COOKIE"} {
		value := os.Getenv(key)
		if value != "" {
			out = strings.ReplaceAll(out, value, "[redacted]")
		}
	}
	return out
}

func runGitHubWorkflowScenario(ctx context.Context, result *ScenarioResult) *ScenarioResult {
	repo := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_GITHUB_REPO"))
	if repo == "" {
		result.FailureReasons = append(result.FailureReasons, "live GitHub repo is required via AGENTOS_EVAL_GITHUB_REPO")
		return result
	}
	owner, name, ok := splitGitHubRepo(repo)
	if !ok {
		result.FailureReasons = append(result.FailureReasons, "AGENTOS_EVAL_GITHUB_REPO must be owner/name")
		return result
	}
	token, err := agentosgh.TokenFromEnv(ctx)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "GitHub token: "+err.Error())
		return result
	}
	if strings.TrimSpace(token) == "" {
		result.FailureReasons = append(result.FailureReasons, "GitHub token is required via GITHUB_TOKEN, GH_TOKEN, or GitHub App env")
		return result
	}
	baseBranch := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_GITHUB_BASE_BRANCH"))
	if baseBranch == "" {
		baseBranch = "main"
	}
	stamp := time.Now().UTC().Format("20060102T150405")
	branch := "agentos-eval/live-github-" + stamp
	title := "[AgentOS Eval] Live GitHub workflow " + stamp
	filePath := ".agentos-evals/live-github-" + stamp + ".md"
	client := agentosgh.NewClient(owner, name)

	var issue *agentosgh.Issue
	var pr *agentosgh.PullRequest
	branchCreated := false
	defer func() {
		if pr != nil && pr.Number > 0 {
			_, _ = client.ClosePR(pr.Number)
		}
		if issue != nil && issue.Number > 0 {
			_, _ = client.CloseIssue(issue.Number)
		}
		if branchCreated {
			_ = client.DeleteBranch(branch)
		}
	}()

	issue, err = client.CreateIssue(agentosgh.CreateIssueRequest{
		Title: title + " issue",
		Body:  "Created by AgentOS live GitHub workflow E2E. This test artifact should be closed automatically.",
	})
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "create issue: "+err.Error())
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "github", Action: "create issue", Passed: issue.Number > 0 && issue.HTMLURL != ""})
	if issue.Number == 0 || issue.HTMLURL == "" {
		result.FailureReasons = append(result.FailureReasons, "created issue missing number or URL")
	}

	comment, err := client.CreateIssueComment(issue.Number, agentosgh.CreateIssueCommentRequest{Body: "AgentOS live GitHub workflow E2E is preparing a temporary branch and PR."})
	result.Checks = append(result.Checks, ScenarioCheck{Page: "github", Action: "comment issue", Passed: err == nil && comment != nil && comment.HTMLURL != ""})
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "comment issue: "+err.Error())
	}

	baseSHA, err := client.GetBranchSHA(baseBranch)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "get base branch: "+err.Error())
		return result
	}
	if err := client.CreateBranch(branch, baseSHA); err != nil {
		result.FailureReasons = append(result.FailureReasons, "create branch: "+err.Error())
		return result
	}
	branchCreated = true
	result.Checks = append(result.Checks, ScenarioCheck{Page: "github", Action: "create branch", Passed: true})

	content := fmt.Sprintf("# AgentOS Live GitHub Workflow E2E\n\n- Created: %s\n- Issue: %s\n", time.Now().UTC().Format(time.RFC3339), issue.HTMLURL)
	if err := client.PutFile(filePath, agentosgh.PutFileRequest{
		Message: "add AgentOS live GitHub workflow eval artifact",
		Content: content,
		Branch:  branch,
	}); err != nil {
		result.FailureReasons = append(result.FailureReasons, "create file: "+err.Error())
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "github", Action: "create branch file", Passed: true})

	pr, err = client.CreatePR(agentosgh.CreatePRRequest{
		Title: title + " PR",
		Body:  "Created by AgentOS live GitHub workflow E2E. This PR should be closed automatically.\n\nIssue: " + issue.HTMLURL,
		Head:  branch,
		Base:  baseBranch,
		Draft: true,
	})
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "create pull request: "+err.Error())
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "github", Action: "create draft pull request", Passed: pr.Number > 0 && pr.HTMLURL != ""})
	if pr.Number == 0 || pr.HTMLURL == "" {
		result.FailureReasons = append(result.FailureReasons, "created pull request missing number or URL")
	}

	checkRuns, err := client.GetCheckRuns(baseBranch)
	result.Checks = append(result.Checks, ScenarioCheck{Page: "github", Action: "check lookup", Passed: err == nil})
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "check lookup: "+err.Error())
	}
	workflowRuns, err := client.ListWorkflowRuns(baseBranch)
	result.Checks = append(result.Checks, ScenarioCheck{Page: "github", Action: "workflow run lookup", Passed: err == nil})
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "workflow run lookup: "+err.Error())
	}

	cleanupOK := true
	closedPR, err := client.ClosePR(pr.Number)
	if err != nil {
		cleanupOK = false
		result.FailureReasons = append(result.FailureReasons, "close pull request: "+err.Error())
	} else {
		pr = nil
		pr = closedPR
	}
	closedIssue, err := client.CloseIssue(issue.Number)
	if err != nil {
		cleanupOK = false
		result.FailureReasons = append(result.FailureReasons, "close issue: "+err.Error())
	} else {
		issue = closedIssue
	}
	if err := client.DeleteBranch(branch); err != nil {
		cleanupOK = false
		result.FailureReasons = append(result.FailureReasons, "delete branch: "+err.Error())
	} else {
		branchCreated = false
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "github", Action: "cleanup artifacts", Passed: cleanupOK})
	result.Artifacts = map[string]string{
		"repo":              repo,
		"baseBranch":        baseBranch,
		"branch":            branch,
		"filePath":          filePath,
		"issueURL":          issue.HTMLURL,
		"issueState":        issue.State,
		"pullRequestURL":    pr.HTMLURL,
		"pullRequestState":  pr.State,
		"checkRunCount":     fmt.Sprintf("%d", len(checkRuns)),
		"workflowRunCount":  fmt.Sprintf("%d", len(workflowRuns)),
		"createdArtifactID": stamp,
	}
	return result
}

func splitGitHubRepo(repo string) (owner, name string, ok bool) {
	repo = strings.TrimSpace(repo)
	owner, name, ok = strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return owner, strings.TrimSuffix(name, ".git"), true
}

type kubernetesRolloutConfig struct {
	Kubeconfig  string
	Context     string
	Namespace   string
	Release     string
	BaseImage   string
	TargetImage string
}

type helmStatusResult struct {
	Info struct {
		Status string `json:"status"`
	} `json:"info"`
	Version int `json:"version"`
}

func runKubernetesRolloutScenario(ctx context.Context, result *ScenarioResult) *ScenarioResult {
	cfg, err := kubernetesRolloutConfigFromEnv()
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, err.Error())
		return result
	}
	if _, err := exec.LookPath("helm"); err != nil {
		result.FailureReasons = append(result.FailureReasons, "helm is required for Kubernetes rollout E2E: "+err.Error())
		return result
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		result.FailureReasons = append(result.FailureReasons, "kubectl is required for Kubernetes rollout E2E: "+err.Error())
		return result
	}

	chartDir, err := writeKubernetesEvalChart()
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "write Kubernetes eval chart: "+err.Error())
		return result
	}
	defer os.RemoveAll(filepath.Dir(chartDir))

	deployment := cfg.Release + "-agentos-eval"
	keepRelease := strings.EqualFold(strings.TrimSpace(os.Getenv("AGENTOS_EVAL_KUBE_KEEP_RELEASE")), "true")
	defer func() {
		if !keepRelease {
			_ = runHelm(ctx, &cfg, "uninstall", cfg.Release, "--wait", "--timeout", "2m")
		}
	}()

	if err := runHelm(ctx, &cfg, "upgrade", "--install", cfg.Release, chartDir, "--set-string", "image="+cfg.BaseImage, "--wait", "--timeout", "2m"); err != nil {
		result.FailureReasons = append(result.FailureReasons, "helm baseline install: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "kubernetes", Action: "helm baseline install", Passed: true})
	baseStatus, err := getHelmStatus(ctx, &cfg)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "helm baseline status: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}

	rolloutStarted := time.Now()
	if err := runHelm(ctx, &cfg, "upgrade", cfg.Release, chartDir, "--set-string", "image="+cfg.TargetImage, "--wait", "--timeout", "2m"); err != nil {
		result.FailureReasons = append(result.FailureReasons, "helm upgrade: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "kubernetes", Action: "helm upgrade", Passed: true})
	if err := runKubectl(ctx, &cfg, "rollout", "status", "deployment/"+deployment, "--timeout=120s"); err != nil {
		result.FailureReasons = append(result.FailureReasons, "rollout status: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}
	rolloutDuration := time.Since(rolloutStarted)
	result.Checks = append(result.Checks, ScenarioCheck{Page: "kubernetes", Action: "rollout status", Passed: true, DurationMS: rolloutDuration.Milliseconds()})
	if err := runKubectl(ctx, &cfg, "wait", "--for=condition=Available", "deployment/"+deployment, "--timeout=120s"); err != nil {
		result.FailureReasons = append(result.FailureReasons, "readiness: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "kubernetes", Action: "readiness", Passed: true})
	deployedImage, err := kubectlOutput(ctx, &cfg, "get", "deployment", deployment, "-o", "jsonpath={.spec.template.spec.containers[0].image}")
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "observe deployed image: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}
	deployedImage = strings.TrimSpace(deployedImage)
	imageOK := deployedImage == cfg.TargetImage
	result.Checks = append(result.Checks, ScenarioCheck{Page: "kubernetes", Action: "image observation", Passed: imageOK})
	if !imageOK {
		result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("deployed image = %q, want %q", deployedImage, cfg.TargetImage))
	}
	upgradedStatus, err := getHelmStatus(ctx, &cfg)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "helm upgraded status: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}

	if err := runHelm(ctx, &cfg, "rollback", cfg.Release, fmt.Sprintf("%d", baseStatus.Version), "--wait", "--timeout", "2m"); err != nil {
		result.FailureReasons = append(result.FailureReasons, "helm rollback: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "kubernetes", Action: "helm rollback", Passed: true})
	if err := runKubectl(ctx, &cfg, "rollout", "status", "deployment/"+deployment, "--timeout=120s"); err != nil {
		result.FailureReasons = append(result.FailureReasons, "rollback rollout status: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}
	rollbackImage, err := kubectlOutput(ctx, &cfg, "get", "deployment", deployment, "-o", "jsonpath={.spec.template.spec.containers[0].image}")
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "observe rollback image: "+err.Error())
		addKubernetesEventArtifacts(ctx, &cfg, result)
		return result
	}
	rollbackImage = strings.TrimSpace(rollbackImage)
	rollbackOK := rollbackImage == cfg.BaseImage
	result.Checks = append(result.Checks, ScenarioCheck{Page: "kubernetes", Action: "rollback image observation", Passed: rollbackOK})
	if !rollbackOK {
		result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("rollback image = %q, want %q", rollbackImage, cfg.BaseImage))
	}

	events := kubernetesEventSnippet(ctx, &cfg)
	result.Artifacts = map[string]string{
		"kubeContext":       cfg.Context,
		"namespace":         cfg.Namespace,
		"release":           cfg.Release,
		"deployment":        deployment,
		"baseImage":         cfg.BaseImage,
		"targetImage":       cfg.TargetImage,
		"deployedImage":     deployedImage,
		"rollbackImage":     rollbackImage,
		"helmStatus":        upgradedStatus.Info.Status,
		"helmRevision":      fmt.Sprintf("%d", upgradedStatus.Version),
		"rolloutDurationMs": fmt.Sprintf("%d", rolloutDuration.Milliseconds()),
		"rollbackNote":      fmt.Sprintf("helm rollback %s %d completed and restored %s", cfg.Release, baseStatus.Version, cfg.BaseImage),
		"events":            events,
	}
	return result
}

func kubernetesRolloutConfigFromEnv() (kubernetesRolloutConfig, error) {
	cfg := kubernetesRolloutConfig{
		Kubeconfig:  strings.TrimSpace(os.Getenv("AGENTOS_EVAL_KUBECONFIG")),
		Context:     strings.TrimSpace(os.Getenv("AGENTOS_EVAL_KUBE_CONTEXT")),
		Namespace:   strings.TrimSpace(os.Getenv("AGENTOS_EVAL_KUBE_NAMESPACE")),
		Release:     strings.TrimSpace(os.Getenv("AGENTOS_EVAL_KUBE_RELEASE")),
		BaseImage:   strings.TrimSpace(os.Getenv("AGENTOS_EVAL_KUBE_BASE_IMAGE")),
		TargetImage: strings.TrimSpace(os.Getenv("AGENTOS_EVAL_KUBE_TARGET_IMAGE")),
	}
	var missing []string
	for name, value := range map[string]string{
		"AGENTOS_EVAL_KUBECONFIG":     cfg.Kubeconfig,
		"AGENTOS_EVAL_KUBE_CONTEXT":   cfg.Context,
		"AGENTOS_EVAL_KUBE_NAMESPACE": cfg.Namespace,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		return cfg, fmt.Errorf("Kubernetes rollout E2E requires explicit %s", strings.Join(missing, ", "))
	}
	if cfg.Release == "" {
		cfg.Release = "agentos-eval-rollout"
	}
	if cfg.BaseImage == "" {
		cfg.BaseImage = "registry.k8s.io/pause:3.9"
	}
	if cfg.TargetImage == "" {
		cfg.TargetImage = "registry.k8s.io/pause:3.10"
	}
	return cfg, nil
}

func writeKubernetesEvalChart() (string, error) {
	root, err := os.MkdirTemp("", "agentos-kubernetes-eval-*")
	if err != nil {
		return "", err
	}
	chartDir := filepath.Join(root, "chart")
	if err := os.MkdirAll(filepath.Join(chartDir, "templates"), 0o755); err != nil {
		return "", err
	}
	files := map[string]string{
		filepath.Join(chartDir, "Chart.yaml"): `apiVersion: v2
name: agentos-rollout-eval
description: AgentOS disposable rollout eval chart
type: application
version: 0.1.0
appVersion: "0.1.0"
`,
		filepath.Join(chartDir, "values.yaml"): `image: registry.k8s.io/pause:3.9
`,
		filepath.Join(chartDir, "templates", "deployment.yaml"): `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ .Release.Name }}-agentos-eval
  labels:
    app.kubernetes.io/name: agentos-rollout-eval
    app.kubernetes.io/instance: {{ .Release.Name }}
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: agentos-rollout-eval
      app.kubernetes.io/instance: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: agentos-rollout-eval
        app.kubernetes.io/instance: {{ .Release.Name }}
    spec:
      automountServiceAccountToken: false
      containers:
        - name: pause
          image: {{ .Values.image | quote }}
          imagePullPolicy: IfNotPresent
          resources:
            requests:
              cpu: 1m
              memory: 8Mi
            limits:
              cpu: 20m
              memory: 32Mi
`,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return "", err
		}
	}
	return chartDir, nil
}

func runHelm(ctx context.Context, cfg *kubernetesRolloutConfig, args ...string) error {
	_, err := commandOutput(ctx, "", "helm", append(helmBaseArgs(cfg), args...)...)
	return err
}

func getHelmStatus(ctx context.Context, cfg *kubernetesRolloutConfig) (*helmStatusResult, error) {
	out, err := commandOutput(ctx, "", "helm", append(helmBaseArgs(cfg), "status", cfg.Release, "--output", "json")...)
	if err != nil {
		return nil, err
	}
	var status helmStatusResult
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func helmBaseArgs(cfg *kubernetesRolloutConfig) []string {
	args := []string{"--kubeconfig", cfg.Kubeconfig, "--kube-context", cfg.Context, "--namespace", cfg.Namespace}
	return args
}

func runKubectl(ctx context.Context, cfg *kubernetesRolloutConfig, args ...string) error {
	_, err := kubectlOutput(ctx, cfg, args...)
	return err
}

func kubectlOutput(ctx context.Context, cfg *kubernetesRolloutConfig, args ...string) (string, error) {
	base := []string{"--kubeconfig", cfg.Kubeconfig, "--context", cfg.Context, "-n", cfg.Namespace}
	return commandOutput(ctx, "", "kubectl", append(base, args...)...)
}

func commandOutput(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := sanitizeKubernetesOutput(strings.TrimSpace(stdout.String() + "\n" + stderr.String()))
		if out == "" {
			out = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), out)
	}
	return sanitizeKubernetesOutput(stdout.String()), nil
}

func addKubernetesEventArtifacts(ctx context.Context, cfg *kubernetesRolloutConfig, result *ScenarioResult) {
	if result.Artifacts == nil {
		result.Artifacts = map[string]string{}
	}
	result.Artifacts["kubeContext"] = cfg.Context
	result.Artifacts["namespace"] = cfg.Namespace
	result.Artifacts["release"] = cfg.Release
	result.Artifacts["events"] = kubernetesEventSnippet(ctx, cfg)
}

func kubernetesEventSnippet(ctx context.Context, cfg *kubernetesRolloutConfig) string {
	out, err := kubectlOutput(ctx, cfg, "get", "events", "--sort-by=.lastTimestamp", "-o", "custom-columns=LAST:.lastTimestamp,TYPE:.type,REASON:.reason,KIND:.involvedObject.kind,NAME:.involvedObject.name,MESSAGE:.message")
	if err != nil {
		return err.Error()
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 9 {
		lines = append(lines[:1], lines[len(lines)-8:]...)
	}
	return sanitizeKubernetesOutput(strings.Join(lines, "\n"))
}

func sanitizeKubernetesOutput(out string) string {
	for _, key := range []string{"AGENTOS_EVAL_AUTH_COOKIE", "GITHUB_TOKEN", "GH_TOKEN"} {
		value := os.Getenv(key)
		if value != "" {
			out = strings.ReplaceAll(out, value, "[redacted]")
		}
	}
	return out
}

type realLLMSmokeConfig struct {
	RepoAllowlist string
	Timeout       time.Duration
	MaxTokens     int
	Model         string
}

type countingLLMClient struct {
	inner               llm.LLMClient
	maxTokens           int
	requests            int
	successfulResponses int
	promptTokens        int
	completionTokens    int
	totalTokens         int
}

func (c *countingLLMClient) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	c.requests++
	if c.maxTokens > 0 && (req.MaxTokens == 0 || req.MaxTokens > c.maxTokens) {
		req.MaxTokens = c.maxTokens
	}
	resp, err := c.inner.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	c.successfulResponses++
	if resp.Usage != nil {
		c.promptTokens += resp.Usage.PromptTokens
		c.completionTokens += resp.Usage.CompletionTokens
		c.totalTokens += resp.Usage.TotalTokens
	}
	return resp, nil
}

func (c *countingLLMClient) ModelName() string {
	return c.inner.ModelName()
}

func runRealLLMSmokeScenario(ctx context.Context, result *ScenarioResult) *ScenarioResult {
	cfg, err := realLLMSmokeConfigFromEnv()
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, err.Error())
		return result
	}
	repo, err := os.MkdirTemp("", "agentos-real-llm-smoke-*")
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "create disposable repo: "+err.Error())
		return result
	}
	if err := realLLMRepoAllowed(repo, cfg.RepoAllowlist); err != nil {
		result.FailureReasons = append(result.FailureReasons, err.Error())
		return result
	}
	if err := initRepo(repo); err != nil {
		result.FailureReasons = append(result.FailureReasons, "init disposable repo: "+err.Error())
		return result
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# AgentOS Real LLM Smoke\n\nDisposable eval repository.\n"), 0o600); err != nil {
		result.FailureReasons = append(result.FailureReasons, "seed disposable repo: "+err.Error())
		return result
	}

	llmCfg := llm.DefaultConfig()
	llmCfg.Timeout = cfg.Timeout
	llmCfg.ModelCoder = cfg.Model
	client := &countingLLMClient{inner: llm.NewLiteLLMClient(llmCfg), maxTokens: cfg.MaxTokens}
	docsAgent := &realLLMSmokeAgent{llm: client}
	orch := orchestrator.NewOrchestrator(client, sandbox.NewLocalSandbox(repo), map[string]runtime.Agent{"docs": docsAgent}, &runtime.Config{})
	orch.SetRunID("real-llm-orchestration-smoke")
	orch.SetSubtaskTimeout(cfg.Timeout)
	orch.SetMaxRetries(1)

	runCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	taskText := "Use the docs agent to create AGENTOS_REAL_LLM_SMOKE.md with a short Markdown report. The file must contain the exact phrase AgentOS real LLM smoke passed and a Residual risk section. Do not modify any other files."
	plan, err := orch.Plan(runCtx, taskText)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "plan: "+err.Error())
		return result
	}
	result.Subtasks = len(plan.Subtasks)
	result.Checks = append(result.Checks, ScenarioCheck{Page: "llm", Action: "plan creation", Passed: len(plan.Subtasks) > 0})
	if len(plan.Subtasks) == 0 {
		result.FailureReasons = append(result.FailureReasons, "real LLM plan contained no subtasks")
		return result
	}
	plan.Subtasks = []orchestrator.Subtask{plan.Subtasks[0]}
	plan.Subtasks[0].AgentName = "docs"
	plan.Subtasks[0].Description = taskText
	plan.Subtasks[0].QualityGate = &orchestrator.QualityGate{
		RequiredFiles: []string{"AGENTOS_REAL_LLM_SMOKE.md"},
		ContentChecks: []orchestrator.QualityContentCheck{{
			File:     "AGENTOS_REAL_LLM_SMOKE.md",
			Contains: []string{"AgentOS real LLM smoke passed", "Residual risk"},
		}},
	}
	result.Subtasks = len(plan.Subtasks)
	result.Agents = []string{"docs"}

	executed, err := orch.Execute(runCtx, plan)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "execute: "+err.Error())
	}
	for _, subtask := range executed {
		if subtask.Success {
			result.Successes++
		} else {
			result.Failures++
			if subtask.Error != "" {
				result.FailureReasons = append(result.FailureReasons, subtask.SubtaskID+": "+subtask.Error)
			}
		}
		if subtask.QualityGate != nil {
			result.QualityGates = append(result.QualityGates, QualityGateCheck{SubtaskID: subtask.SubtaskID, Passed: subtask.QualityGate.Passed})
		}
	}
	artifactPath := filepath.Join(repo, "AGENTOS_REAL_LLM_SMOKE.md")
	_, statErr := os.Stat(artifactPath)
	artifactExists := statErr == nil
	result.RequiredFiles = append(result.RequiredFiles, FileCheck{Path: "AGENTOS_REAL_LLM_SMOKE.md", Exists: artifactExists})
	result.Checks = append(result.Checks,
		ScenarioCheck{Page: "llm", Action: "subtask execution", Passed: len(executed) > 0 && result.Failures == 0},
		ScenarioCheck{Page: "llm", Action: "artifact recording", Passed: artifactExists},
		ScenarioCheck{Page: "llm", Action: "real LLM responses", Passed: client.successfulResponses > 0},
	)
	if !artifactExists {
		result.FailureReasons = append(result.FailureReasons, "missing AGENTOS_REAL_LLM_SMOKE.md")
	}
	if client.successfulResponses == 0 {
		result.FailureReasons = append(result.FailureReasons, "real LLM smoke recorded 0 successful LLM responses")
	}
	result.Artifacts = map[string]string{
		"repository":          repo,
		"model":               client.ModelName(),
		"baseURL":             llmCfg.BaseURL,
		"timeout":             cfg.Timeout.String(),
		"maxTokens":           fmt.Sprintf("%d", cfg.MaxTokens),
		"repoAllowlist":       cfg.RepoAllowlist,
		"planSubtasks":        fmt.Sprintf("%d", len(plan.Subtasks)),
		"successes":           fmt.Sprintf("%d", result.Successes),
		"failures":            fmt.Sprintf("%d", result.Failures),
		"llmRequests":         fmt.Sprintf("%d", client.requests),
		"llmResponses":        fmt.Sprintf("%d", client.successfulResponses),
		"promptTokens":        fmt.Sprintf("%d", client.promptTokens),
		"completionTokens":    fmt.Sprintf("%d", client.completionTokens),
		"totalTokens":         fmt.Sprintf("%d", client.totalTokens),
		"costBudget":          strings.TrimSpace(os.Getenv("AGENTOS_EVAL_LLM_COST_BUDGET")),
		"residualRisk":        "LLM output quality is model-dependent; this smoke only validates a bounded docs artifact flow.",
		"requiredArtifact":    "AGENTOS_REAL_LLM_SMOKE.md",
		"requiredArtifactSet": fmt.Sprintf("%t", artifactExists),
		"diff":                gitDiff(ctx, repo),
	}
	return result
}

func realLLMSmokeConfigFromEnv() (realLLMSmokeConfig, error) {
	var cfg realLLMSmokeConfig
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("AGENTOS_EVAL_LIVE_LLM")), "true") {
		return cfg, fmt.Errorf("real LLM smoke requires AGENTOS_EVAL_LIVE_LLM=true")
	}
	if strings.TrimSpace(os.Getenv("LITELLM_BASE_URL")) == "" {
		return cfg, fmt.Errorf("real LLM smoke requires explicit LITELLM_BASE_URL")
	}
	cfg.RepoAllowlist = strings.TrimSpace(os.Getenv("AGENTOS_EVAL_LLM_REPO_ALLOWLIST"))
	if cfg.RepoAllowlist == "" {
		return cfg, fmt.Errorf("real LLM smoke requires AGENTOS_EVAL_LLM_REPO_ALLOWLIST containing temp")
	}
	timeout := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_LLM_TIMEOUT"))
	if timeout == "" {
		timeout = "2m"
	}
	parsedTimeout, err := time.ParseDuration(timeout)
	if err != nil {
		return cfg, fmt.Errorf("AGENTOS_EVAL_LLM_TIMEOUT: %w", err)
	}
	cfg.Timeout = parsedTimeout
	cfg.MaxTokens = 1024
	if raw := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_LLM_MAX_TOKENS")); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &cfg.MaxTokens); err != nil {
			return cfg, fmt.Errorf("AGENTOS_EVAL_LLM_MAX_TOKENS must be an integer")
		}
	}
	if cfg.MaxTokens <= 0 {
		return cfg, fmt.Errorf("AGENTOS_EVAL_LLM_MAX_TOKENS must be positive")
	}
	cfg.Model = strings.TrimSpace(os.Getenv("AGENTOS_EVAL_LLM_MODEL"))
	if cfg.Model == "" {
		cfg.Model = strings.TrimSpace(os.Getenv("AGENTOS_MODEL_CODER"))
	}
	if cfg.Model == "" {
		return cfg, fmt.Errorf("real LLM smoke requires AGENTOS_EVAL_LLM_MODEL or AGENTOS_MODEL_CODER")
	}
	return cfg, nil
}

func realLLMRepoAllowed(repo, allowlist string) error {
	for _, item := range strings.Split(allowlist, ",") {
		item = strings.TrimSpace(item)
		if item == "temp" || item == repo {
			return nil
		}
	}
	return fmt.Errorf("disposable repo %q is not allowed by AGENTOS_EVAL_LLM_REPO_ALLOWLIST", repo)
}

type realLLMSmokeAgent struct {
	llm llm.LLMClient
}

func (a *realLLMSmokeAgent) Name() string { return "docs" }

func (a *realLLMSmokeAgent) Plan(ctx *runtime.RunContext) (*runtime.Plan, error) {
	resp, err := a.llm.Chat(ctx.Context, llm.ChatRequest{
		Model: a.llm.ModelName(),
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You are a bounded AgentOS smoke-test planner. Return a concise plan summary for the requested documentation artifact."},
			{Role: llm.RoleUser, Content: ctx.Task.Description},
		},
		Temperature: 0.1,
		MaxTokens:   512,
	})
	if err != nil {
		return nil, err
	}
	summary := strings.TrimSpace(resp.Choices[0].Message.Content)
	if summary == "" {
		summary = "Create the required real LLM smoke artifact."
	}
	return &runtime.Plan{
		Summary:               summary,
		EstimatedFilesChanged: 1,
		Steps: []runtime.Step{{
			StepNumber:  1,
			Action:      "edit",
			Description: "Create AGENTOS_REAL_LLM_SMOKE.md with smoke status and residual risk.",
			TargetFiles: []string{"AGENTOS_REAL_LLM_SMOKE.md"},
			Reasoning:   "Bounded smoke scenario requires a deterministic artifact while still exercising live LLM planning.",
		}},
	}, nil
}

func (a *realLLMSmokeAgent) Execute(ctx *runtime.RunContext, plan *runtime.Plan) (*runtime.ExecutionResult, error) {
	started := time.Now()
	content := "# AgentOS Real LLM Smoke\n\nAgentOS real LLM smoke passed.\n\n## Residual risk\n\nLLM behavior remains model-dependent; this smoke validates a bounded docs artifact flow.\n"
	tool, ok := ctx.Registry.Get("write_file")
	if !ok {
		return &runtime.ExecutionResult{Success: false, Error: "write_file tool not found"}, fmt.Errorf("write_file tool not found")
	}
	output := tool.Run(ctx.Context, tools.ToolInput{"file": "AGENTOS_REAL_LLM_SMOKE.md", "content": content})
	step := runtime.StepResult{StepNumber: 1, Action: "edit", Duration: time.Since(started)}
	if output.Success {
		step.Success = true
		step.Output = "created AGENTOS_REAL_LLM_SMOKE.md"
		return &runtime.ExecutionResult{
			StepResults: []runtime.StepResult{step},
			Diff:        gitDiff(ctx.Context, ctx.Workspace.RootDir()),
			Success:     true,
		}, nil
	}
	step.Error = output.Error
	result := &runtime.ExecutionResult{StepResults: []runtime.StepResult{step}, Success: false, Error: output.Error}
	return result, fmt.Errorf("%s", output.Error)
}

func (a *realLLMSmokeAgent) Review(ctx *runtime.RunContext, result *runtime.ExecutionResult) (*runtime.ReviewResult, error) {
	resp, err := a.llm.Chat(ctx.Context, llm.ChatRequest{
		Model: a.llm.ModelName(),
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "You are reviewing a bounded AgentOS real LLM smoke artifact. Reply with a concise approval summary."},
			{Role: llm.RoleUser, Content: result.Diff},
		},
		Temperature: 0.1,
		MaxTokens:   512,
	})
	if err != nil {
		return nil, err
	}
	return &runtime.ReviewResult{Approved: true, Summary: strings.TrimSpace(resp.Choices[0].Message.Content)}, nil
}

type scheduleEvalDefinition struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Status        string                  `json:"status"`
	Repo          string                  `json:"repo"`
	BaseBranch    string                  `json:"baseBranch"`
	NextRunAt     time.Time               `json:"nextRunAt"`
	LastRunAt     time.Time               `json:"lastRunAt"`
	LastRunID     string                  `json:"lastRunId"`
	LastRunStatus string                  `json:"lastRunStatus"`
	Executions    []scheduleEvalExecution `json:"executions"`
}

type scheduleEvalExecution struct {
	ID        string    `json:"id"`
	RunID     string    `json:"runId"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	StartedAt time.Time `json:"startedAt"`
}

type notificationEvalRecord struct {
	ID           string                     `json:"id"`
	ScheduleID   string                     `json:"scheduleId"`
	RunID        string                     `json:"runId"`
	Trigger      string                     `json:"trigger"`
	Status       string                     `json:"status"`
	Repo         string                     `json:"repo"`
	Destinations []string                   `json:"destinations"`
	Deliveries   []notificationEvalDelivery `json:"deliveries"`
	CreatedAt    time.Time                  `json:"createdAt"`
}

type notificationEvalDelivery struct {
	Destination string `json:"destination"`
	Status      string `json:"status"`
}

func runScheduleNotificationScenario(ctx context.Context, liveURL string, result *ScenarioResult) *ScenarioResult {
	base := strings.TrimRight(strings.TrimSpace(liveURL), "/")
	if base == "" {
		base = strings.TrimRight(os.Getenv("AGENTOS_EVAL_LIVE_URL"), "/")
	}
	if base == "" {
		result.FailureReasons = append(result.FailureReasons, "live URL is required via --live-url or AGENTOS_EVAL_LIVE_URL")
		return result
	}
	if !authCookieConfigured() {
		result.FailureReasons = append(result.FailureReasons, "authenticated API session cookie is required via AGENTOS_EVAL_AUTH_COOKIE")
		return result
	}
	repo := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_SCHEDULE_REPO"))
	if repo == "" {
		repo = "."
	}
	branch := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_SCHEDULE_BASE_BRANCH"))
	if branch == "" {
		branch = "main"
	}
	client := &http.Client{Timeout: 30 * time.Second}
	name := "AgentOS Eval Schedule Notification " + time.Now().UTC().Format("20060102T150405")
	payload := map[string]any{
		"name":              name,
		"repo":              repo,
		"baseBranch":        branch,
		"task":              "AgentOS schedule notification E2E smoke. Produce a short no-op report. Do not modify files.",
		"agents":            []string{"reporter"},
		"strategy":          "sequential",
		"schedule":          map[string]any{"type": "interval", "interval": "24h", "timezone": "UTC"},
		"concurrencyPolicy": "forbid",
		"limits":            map[string]any{"maxDuration": "2m", "maxSubtasks": 1, "maxConcurrentRepoRuns": 100},
		"notification":      map[string]any{"enabled": true, "triggers": []string{"started"}, "destinations": []string{"inbox"}},
		"github":            map[string]any{"createIssue": false, "createPullRequest": false},
	}

	var created scheduleEvalDefinition
	if err := postJSON(ctx, client, base+"/api/schedules", payload, &created); err != nil {
		result.FailureReasons = append(result.FailureReasons, "create schedule: "+err.Error())
		return result
	}
	var notificationID string
	defer func() {
		if notificationID != "" {
			_ = deleteAPI(ctx, client, base+"/api/notifications/"+notificationID)
		}
		if created.ID != "" {
			_ = deleteAPI(ctx, client, base+"/api/schedules/"+created.ID)
		}
	}()
	createdOK := strings.HasPrefix(created.ID, "schedule-") && created.Status == "active" && !created.NextRunAt.IsZero()
	result.Checks = append(result.Checks, ScenarioCheck{Page: "schedules", Action: "create schedule", Passed: createdOK})
	if !createdOK {
		result.FailureReasons = append(result.FailureReasons, "created schedule missing active status, id, or next run time")
	}

	var execution scheduleEvalExecution
	if err := postJSON(ctx, client, base+"/api/schedules/"+created.ID+"/run", map[string]any{}, &execution); err != nil {
		result.FailureReasons = append(result.FailureReasons, "run schedule: "+err.Error())
		return result
	}
	runOK := execution.Status == "started" && strings.HasPrefix(execution.RunID, "run-")
	result.Checks = append(result.Checks, ScenarioCheck{Page: "schedules", Action: "manual execution", Passed: runOK})
	if !runOK {
		result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("execution status=%q runID=%q, want started run-*", execution.Status, execution.RunID))
	}

	var schedule scheduleEvalDefinition
	scheduleOK := false
	for i := 0; i < 20; i++ {
		if err := getJSON(ctx, client, base+"/api/schedules/"+created.ID, &schedule); err == nil {
			for _, item := range schedule.Executions {
				if item.RunID == execution.RunID {
					scheduleOK = schedule.LastRunID == execution.RunID && item.Status != ""
					break
				}
			}
		}
		if scheduleOK {
			break
		}
		time.Sleep(time.Second)
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "schedules", Action: "execution history", Passed: scheduleOK})
	if !scheduleOK {
		result.FailureReasons = append(result.FailureReasons, "schedule execution history did not include run "+execution.RunID)
	}

	var notification notificationEvalRecord
	notificationOK := false
	for i := 0; i < 20; i++ {
		var notifications []notificationEvalRecord
		if err := getJSON(ctx, client, base+"/api/notifications", &notifications); err == nil {
			for i := range notifications {
				item := &notifications[i]
				if item.ScheduleID == created.ID && item.RunID == execution.RunID && item.Trigger == "started" {
					notification = *item
					notificationID = item.ID
					notificationOK = item.Status == "started" && hasInboxDelivery(item.Deliveries)
					break
				}
			}
		}
		if notificationOK {
			break
		}
		time.Sleep(time.Second)
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "notifications", Action: "started inbox notification", Passed: notificationOK})
	if !notificationOK {
		result.FailureReasons = append(result.FailureReasons, "started inbox notification not found for schedule "+created.ID+" run "+execution.RunID)
	}

	cleanupOK := true
	if notificationID != "" {
		if err := deleteAPI(ctx, client, base+"/api/notifications/"+notificationID); err != nil {
			cleanupOK = false
			result.FailureReasons = append(result.FailureReasons, "delete notification: "+err.Error())
		} else {
			notificationID = ""
		}
	}
	if err := deleteAPI(ctx, client, base+"/api/schedules/"+created.ID); err != nil {
		cleanupOK = false
		result.FailureReasons = append(result.FailureReasons, "delete schedule: "+err.Error())
	} else {
		created.ID = ""
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "schedules", Action: "cleanup artifacts", Passed: cleanupOK})
	result.Artifacts = map[string]string{
		"url":                 base,
		"scheduleID":          schedule.ID,
		"scheduleName":        name,
		"triggerTime":         execution.StartedAt.Format(time.RFC3339Nano),
		"runID":               execution.RunID,
		"executionStatus":     execution.Status,
		"lastRunStatus":       schedule.LastRunStatus,
		"notificationID":      notification.ID,
		"notificationStatus":  notification.Status,
		"notificationTrigger": notification.Trigger,
	}
	return result
}

func hasInboxDelivery(deliveries []notificationEvalDelivery) bool {
	for _, delivery := range deliveries {
		if delivery.Destination == "inbox" && delivery.Status == "success" {
			return true
		}
	}
	return false
}

type storageCleanupEvalResult struct {
	Summary storageCleanupEvalSummary `json:"summary"`
	Items   []storageCleanupEvalItem  `json:"items"`
}

type storageCleanupEvalSummary struct {
	Selected int   `json:"selected"`
	Archived int   `json:"archived"`
	Deleted  int   `json:"deleted"`
	Skipped  int   `json:"skipped"`
	Bytes    int64 `json:"bytes"`
}

type storageCleanupEvalItem struct {
	Type    string `json:"type"`
	ID      string `json:"id"`
	Action  string `json:"action"`
	Skipped bool   `json:"skipped"`
	Reason  string `json:"reason"`
}

type storageUsageEvalResult struct {
	Usage map[string]any `json:"usage"`
}

type storageAuditEvalEvent struct {
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
	Target  string `json:"target"`
	Message string `json:"message"`
}

func runStorageCleanupScenario(ctx context.Context, liveURL string, result *ScenarioResult) *ScenarioResult {
	base := strings.TrimRight(strings.TrimSpace(liveURL), "/")
	if base == "" {
		base = strings.TrimRight(os.Getenv("AGENTOS_EVAL_LIVE_URL"), "/")
	}
	if base == "" {
		result.FailureReasons = append(result.FailureReasons, "live URL is required via --live-url or AGENTOS_EVAL_LIVE_URL")
		return result
	}
	if !authCookieConfigured() {
		result.FailureReasons = append(result.FailureReasons, "authenticated API session cookie is required via AGENTOS_EVAL_AUTH_COOKIE")
		return result
	}
	repo := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_STORAGE_REPO"))
	if repo == "" {
		repo = "agentos-evals/storage-cleanup"
	}
	branch := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_STORAGE_BASE_BRANCH"))
	if branch == "" {
		branch = "agentos-eval-storage-cleanup"
	}
	policy := map[string]any{
		"repo":                     repo,
		"baseBranch":               branch,
		"orchestrationRetention":   "1h",
		"runArtifactRetention":     "0",
		"workspaceRetention":       "0",
		"memoryRetention":          "0",
		"guidelineRetention":       "0",
		"keepLastOrchestrations":   1,
		"archiveBeforeDelete":      true,
		"allowLinkedGitHubCleanup": false,
	}
	client := &http.Client{Timeout: 30 * time.Second}
	usageBefore, err := getStorageUsage(ctx, client, base)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "storage usage before cleanup: "+err.Error())
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "storage", Action: "usage before cleanup", Passed: usageBefore.Usage != nil})
	dryRun, err := postStorageCleanup(ctx, client, base, true, policy)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "storage dry-run: "+err.Error())
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "storage", Action: "dry-run preview", Passed: dryRun.Summary.Selected > 0 && dryRun.Summary.Skipped > 0})
	if dryRun.Summary.Selected == 0 {
		result.FailureReasons = append(result.FailureReasons, "storage dry-run selected 0 cleanup targets; seed disposable fixtures for "+repo+" "+branch)
	}
	if dryRun.Summary.Skipped == 0 {
		result.FailureReasons = append(result.FailureReasons, "storage dry-run skipped 0 protected targets; seed an active or GitHub-linked fixture")
	}
	executed, err := postStorageCleanup(ctx, client, base, false, policy)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "storage cleanup execution: "+err.Error())
		return result
	}
	applied := executed.Summary.Archived + executed.Summary.Deleted
	result.Checks = append(result.Checks, ScenarioCheck{Page: "storage", Action: "cleanup execution", Passed: applied > 0 && executed.Summary.Skipped > 0})
	if applied == 0 {
		result.FailureReasons = append(result.FailureReasons, "storage cleanup archived/deleted 0 targets")
	}
	if executed.Summary.Skipped == 0 {
		result.FailureReasons = append(result.FailureReasons, "storage cleanup skipped 0 protected targets")
	}
	usageAfter, err := getStorageUsage(ctx, client, base)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "storage usage after cleanup: "+err.Error())
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "storage", Action: "usage after cleanup", Passed: usageAfter.Usage != nil})
	auditEvents, err := getStorageAuditEvents(ctx, client, base)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "storage cleanup audit: "+err.Error())
		return result
	}
	auditMatched := hasCleanupAuditEvent(auditEvents, executed.Summary)
	result.Checks = append(result.Checks, ScenarioCheck{Page: "audit", Action: "cleanup audit event", Passed: auditMatched})
	if !auditMatched {
		result.FailureReasons = append(result.FailureReasons, "storage cleanup audit event not found")
	}
	postRun, err := postStorageCleanup(ctx, client, base, true, policy)
	if err != nil {
		result.FailureReasons = append(result.FailureReasons, "storage post-cleanup dry-run: "+err.Error())
		return result
	}
	result.Checks = append(result.Checks, ScenarioCheck{Page: "storage", Action: "post-cleanup preview", Passed: postRun.Summary.Selected == 0})
	if postRun.Summary.Selected != 0 {
		result.FailureReasons = append(result.FailureReasons, fmt.Sprintf("post-cleanup selected = %d, want 0", postRun.Summary.Selected))
	}
	result.Artifacts = map[string]string{
		"url":            base,
		"repo":           repo,
		"baseBranch":     branch,
		"usageBefore":    mustJSON(usageBefore.Usage),
		"usageAfter":     mustJSON(usageAfter.Usage),
		"dryRunSummary":  mustJSON(dryRun.Summary),
		"executeSummary": mustJSON(executed.Summary),
		"postRunSummary": mustJSON(postRun.Summary),
		"dryRunItemIDs":  cleanupItemIDs(dryRun.Items),
		"executeItemIDs": cleanupItemIDs(executed.Items),
		"auditEvents":    mustJSON(auditEvents),
	}
	return result
}

func getStorageUsage(ctx context.Context, client *http.Client, base string) (*storageUsageEvalResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/storage", http.NoBody)
	if err != nil {
		return nil, err
	}
	addAuthHeaders(req)
	var usage storageUsageEvalResult
	if err := doJSON(client, req, &usage); err != nil {
		return nil, err
	}
	return &usage, nil
}

func postStorageCleanup(ctx context.Context, client *http.Client, base string, dryRun bool, policy map[string]any) (*storageCleanupEvalResult, error) {
	body, err := json.Marshal(map[string]any{"dryRun": dryRun, "policy": policy})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/storage/cleanup", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	addAuthHeaders(req)
	var result storageCleanupEvalResult
	if err := doJSON(client, req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func getStorageAuditEvents(ctx context.Context, client *http.Client, base string) ([]storageAuditEvalEvent, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/audit", http.NoBody)
	if err != nil {
		return nil, err
	}
	addAuthHeaders(req)
	var events []storageAuditEvalEvent
	if err := doJSON(client, req, &events); err != nil {
		return nil, err
	}
	return events, nil
}

func getJSON(ctx context.Context, client *http.Client, url string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return err
	}
	addAuthHeaders(req)
	return doJSON(client, req, into)
}

func postJSON(ctx context.Context, client *http.Client, url string, payload, into any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	addAuthHeaders(req)
	return doJSON(client, req, into)
}

func deleteAPI(ctx context.Context, client *http.Client, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, http.NoBody)
	if err != nil {
		return err
	}
	addAuthHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
}

func doJSON(client *http.Client, req *http.Request, into any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, into)
}

func addAuthHeaders(req *http.Request) {
	if cookie := strings.TrimSpace(os.Getenv("AGENTOS_EVAL_AUTH_COOKIE")); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func hasCleanupAuditEvent(events []storageAuditEvalEvent, summary storageCleanupEvalSummary) bool {
	want := fmt.Sprintf("selected=%d archived=%d deleted=%d skipped=%d", summary.Selected, summary.Archived, summary.Deleted, summary.Skipped)
	for _, event := range events {
		if event.Action == "storage.cleanup" && event.Outcome == "success" && event.Target == "storage" && event.Message == want {
			return true
		}
	}
	return false
}

func cleanupItemIDs(items []storageCleanupEvalItem) string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return strings.Join(ids, ",")
}

func getText(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return string(data), nil
}

func getStatus(ctx context.Context, client *http.Client, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func firstAsset(index, prefix, suffix string) string {
	remaining := index
	for {
		start := strings.Index(remaining, prefix)
		if start < 0 {
			return ""
		}
		rest := remaining[start+len(prefix):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			return ""
		}
		path := rest[:end]
		if strings.HasSuffix(path, suffix) {
			return path
		}
		remaining = rest[end+1:]
	}
}

func coverageFor(scenarios []Scenario, results []ScenarioResult) []CoverageArea {
	type counts struct {
		total     int
		covered   int
		scenarios []string
	}
	byArea := map[string]*counts{}
	passed := map[string]bool{}
	for i := range results {
		passed[results[i].ID] = results[i].Passed
	}
	for i := range scenarios {
		scenario := &scenarios[i]
		for _, area := range scenario.FunctionalArea {
			item := byArea[area]
			if item == nil {
				item = &counts{}
				byArea[area] = item
			}
			item.total++
			item.scenarios = append(item.scenarios, scenario.ID)
			if passed[scenario.ID] {
				item.covered++
			}
		}
	}
	names := make([]string, 0, len(byArea))
	for name := range byArea {
		names = append(names, name)
	}
	slices.Sort(names)
	coverage := make([]CoverageArea, 0, len(names))
	for _, name := range names {
		item := byArea[name]
		coverage = append(coverage, CoverageArea{Name: name, Covered: item.covered, Total: item.total, Scenarios: item.scenarios})
	}
	return coverage
}

func finishResult(started time.Time, result *ScenarioResult) ScenarioResult {
	result.DurationMS = time.Since(started).Milliseconds()
	result.Passed = len(result.FailureReasons) == 0
	return *result
}

func newScenarioOrchestrator(repo string, scenario *Scenario) (*orchestrator.Orchestrator, error) {
	home, err := os.MkdirTemp("", scenario.ID+"-home-*")
	if err != nil {
		return nil, err
	}
	if err := os.Setenv("AGENTOS_HOME", home); err != nil {
		return nil, err
	}
	client := llm.NewMockLLMClient(nil)
	reg := agent.DefaultRegistry()
	agents := make(map[string]runtime.Agent, len(scenario.Agents))
	for _, name := range scenario.Agents {
		a, err := reg.Create(name, client)
		if err != nil {
			return nil, err
		}
		agents[name] = a
	}
	orch := orchestrator.NewOrchestrator(client, sandbox.NewLocalSandbox(repo), agents, &runtime.Config{})
	orch.SetRunID(scenario.ID)
	orch.SetSubtaskTimeout(2 * time.Minute)
	return orch, nil
}

func initRepo(repo string) error {
	if err := runCmd(context.Background(), repo, "git", "init", "-b", "main"); err != nil {
		return err
	}
	if err := runCmd(context.Background(), repo, "git", "config", "user.email", "agentos-evals@example.invalid"); err != nil {
		return err
	}
	return runCmd(context.Background(), repo, "git", "config", "user.name", "AgentOS Evals")
}

func runCmd(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		if out == "" {
			out = err.Error()
		}
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), out)
	}
	return nil
}

func gitDiff(ctx context.Context, root string) string {
	cmd := exec.CommandContext(ctx, "git", "diff", "--", ".")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// WriteJSON writes a report as indented JSON.
func WriteJSON(report *Report, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if path == "" || path == "-" {
		_, err = os.Stdout.Write(append(data, '\n'))
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// DefaultOutputPath returns the conventional eval report path.
func DefaultOutputPath(format string) string {
	ext := "json"
	if format == "markdown" {
		ext = "md"
	}
	return filepath.Join(apphome.Dir(), "evals", "orchestration-report."+ext)
}
