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
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/orchestrator"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/sandbox"
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
	ScenarioIDs              []string
	WorkDir                  string
	IncludeLive              bool
	LiveURL                  string
	IncludeAuthE2E           bool
	IncludeStorageCleanupE2E bool
	IncludeScheduleNotifyE2E bool
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
