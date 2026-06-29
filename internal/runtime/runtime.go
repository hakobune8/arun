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

// Package runtime manages the execution lifecycle of coding tasks, including
// planning, execution, testing, linting, review, and result generation.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/event"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/safety"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/state"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/kazyamaz200/agentos/internal/tools"
	"gopkg.in/yaml.v3"
)

// Agent defines the interface for a coding agent that can plan, execute, and review tasks.
// Runtime can execute any Agent implementation without knowing its concrete type.
type Agent interface {
	Name() string
	Plan(ctx *RunContext) (*Plan, error)
	Execute(ctx *RunContext, plan *Plan) (*ExecutionResult, error)
	Review(ctx *RunContext, result *ExecutionResult) (*ReviewResult, error)
}

// LifecycleHooks provides optional callbacks that are invoked at key points
// during a run. All hooks are optional (nil is safe).
type LifecycleHooks struct {
	BeforeRun    func(ctx context.Context, rctx *RunContext) error
	AfterPlan    func(ctx context.Context, rctx *RunContext, plan *Plan) error
	AfterExecute func(ctx context.Context, rctx *RunContext, result *ExecutionResult) error
	AfterReview  func(ctx context.Context, rctx *RunContext, review *ReviewResult) error
	AfterRun     func(ctx context.Context, rctx *RunContext, result *ExecutionResult) error
	OnFail       func(ctx context.Context, rctx *RunContext, err error) error
}

// Runtime manages the end-to-end execution of a coding task through an Agent,
// including lifecycle hooks, state persistence, event emission, and artifact generation.
type Runtime struct {
	LLM       llm.LLMClient
	Registry  *tools.Registry
	Store     *state.RunStore
	Policy    *safety.CommandPolicy
	Workspace sandbox.Sandbox
	Logger    *state.Logger
	Profile   *profile.Profile
	Config    *Config
	Agent     Agent
	Hooks     LifecycleHooks
	Events    event.Bus
}

// NewRuntime creates a new Runtime with the given dependencies.
func NewRuntime(llmClient llm.LLMClient, prof *profile.Profile, workspace sandbox.Sandbox, cfg *Config, agent Agent) *Runtime {
	registry := tools.NewRegistry()
	policy := safety.NewCommandPolicy(prof.Tools.DenyCommands)

	workDir := workspace.RootDir()
	repoPath := workspace.RootDir()

	allowed := make(map[string]bool, len(prof.Tools.Allow))
	for _, name := range prof.Tools.Allow {
		allowed[name] = true
	}
	allowAll := len(allowed) == 0

	if allowAll || allowed["read_file"] {
		registry.MustRegister(tools.NewReadFileTool(workDir))
	}
	if allowAll || allowed["write_file"] {
		registry.MustRegister(tools.NewWriteFileTool(workDir))
	}
	if allowAll || allowed["search"] {
		registry.MustRegister(tools.NewSearchTool(workDir))
	}
	if allowAll || allowed["shell"] {
		registry.MustRegister(tools.NewShellTool(policy, workDir))
	}
	if allowAll || allowed["git"] {
		registry.MustRegister(tools.NewGitTool(repoPath))
	}
	if allowAll || allowed["test"] {
		registry.MustRegister(tools.NewTestTool(workDir))
	}

	runDir := workspace.RunPath()
	logger := state.NewLogger(runDir)
	store := state.NewRunStore(runDir)

	return &Runtime{
		LLM:       llmClient,
		Registry:  registry,
		Store:     store,
		Policy:    policy,
		Workspace: workspace,
		Logger:    logger,
		Profile:   prof,
		Config:    cfg,
		Agent:     agent,
	}
}

// emit publishes an event to the event bus if one is configured.
func (r *Runtime) emit(ctx context.Context, runID string, t event.Type, data interface{}) {
	if r.Events == nil {
		return
	}
	_ = r.Events.Publish(ctx, &event.Event{ //nolint:errcheck // best-effort event
		ID:        fmt.Sprintf("%s-%d", runID, time.Now().UnixNano()),
		Type:      t,
		Timestamp: time.Now(),
		RunID:     runID,
		AgentID:   r.Agent.Name(),
		Data:      data,
	})
}

// Run executes a coding task through the configured Agent.
// It delegates planning, execution, and review to the Agent while
// managing lifecycle hooks, state persistence, and artifact generation.
func (r *Runtime) Run(ctx context.Context, tk *task.Task) error {
	startTime := time.Now()

	if err := r.Workspace.PrepareRun(tk.ID); err != nil {
		return fmt.Errorf("prepare run: %w", err)
	}

	rctx := NewRunContext(ctx, tk, r)

	r.emit(ctx, tk.ID, event.TypeTaskCreated, map[string]interface{}{
		"task_id": tk.ID,
		"title":   tk.Title,
		"profile": r.Profile.Name,
		"repo":    tk.Repo,
	})

	record := &state.RunRecord{
		TaskID:      tk.ID,
		Status:      state.RunStatusPending,
		StartedAt:   startTime,
		ProfileName: r.Profile.Name,
		Branch:      tk.Branch,
	}
	_ = r.Store.Save(record)                                                    //nolint:errcheck // best-effort save
	_ = r.Logger.Log("info", "runtime", "starting run", map[string]interface{}{ //nolint:errcheck // best-effort log
		"task_id": tk.ID,
		"profile": r.Profile.Name,
		"repo":    tk.Repo,
	})

	if err := r.Workspace.SaveFile("task.yaml", mustMarshalYAML(tk)); err != nil {
		return fmt.Errorf("save task.yaml: %w", err)
	}
	if err := r.Workspace.SaveFile("profile.yaml", mustMarshalYAML(r.Profile)); err != nil {
		return fmt.Errorf("save profile.yaml: %w", err)
	}

	if h := r.Hooks.BeforeRun; h != nil {
		if err := h(ctx, rctx); err != nil {
			return fmt.Errorf("before run hook: %w", err)
		}
	}

	// --- Plan ---
	record.Status = state.RunStatusPlanning
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	r.emit(ctx, tk.ID, event.TypePlanningStarted, nil)
	plan, err := r.Agent.Plan(rctx)
	if err != nil {
		r.emit(ctx, tk.ID, event.TypeRunFailed, map[string]string{"error": err.Error()})
		record.Status = state.RunStatusFailed
		record.Error = err.Error()
		_ = r.Store.Save(record) //nolint:errcheck // best-effort save
		if h := r.Hooks.OnFail; h != nil {
			_ = h(ctx, rctx, err) //nolint:errcheck // best-effort hook
		}
		return fmt.Errorf("create plan: %w", err)
	}
	r.emit(ctx, tk.ID, event.TypePlanningFinished, map[string]interface{}{
		"steps":   len(plan.Steps),
		"summary": plan.Summary,
	})

	_ = r.Workspace.SaveFile("plan.json", mustMarshalJSON(plan))                //nolint:errcheck // best-effort save
	_ = r.Logger.Log("info", "runtime", "plan created", map[string]interface{}{ //nolint:errcheck // best-effort log
		"summary":         plan.Summary,
		"steps":           len(plan.Steps),
		"estimated_files": plan.EstimatedFilesChanged,
	})

	if h := r.Hooks.AfterPlan; h != nil {
		if err := h(ctx, rctx, plan); err != nil {
			return fmt.Errorf("after plan hook: %w", err)
		}
	}

	// --- Execute ---
	record.Status = state.RunStatusExecuting
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	result, err := r.Agent.Execute(rctx, plan)
	if err != nil {
		r.saveExecutionArtifacts(result)
		r.emit(ctx, tk.ID, event.TypeRunFailed, map[string]string{"error": err.Error(), "phase": "execute"})
		record.Status = state.RunStatusFailed
		record.Error = err.Error()
		_ = r.Store.Save(record) //nolint:errcheck // best-effort save
		if h := r.Hooks.OnFail; h != nil {
			_ = h(ctx, rctx, err) //nolint:errcheck // best-effort hook
		}
		return fmt.Errorf("execute plan: %w", err)
	}

	r.saveExecutionArtifacts(result)

	if h := r.Hooks.AfterExecute; h != nil {
		if err := h(ctx, rctx, result); err != nil {
			return fmt.Errorf("after execute hook: %w", err)
		}
	}

	// --- Review ---
	record.Status = state.RunStatusReviewing
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	r.emit(ctx, tk.ID, event.TypeReviewStarted, nil)
	reviewResult, err := r.Agent.Review(rctx, result)
	if err != nil {
		_ = r.Logger.Log("warn", "runtime", "review failed", err.Error()) //nolint:errcheck // best-effort log
	}
	r.emit(ctx, tk.ID, event.TypeReviewFinished, map[string]interface{}{
		"approved": reviewResult != nil && reviewResult.Approved,
		"summary":  reviewResult.Summary,
	})

	if h := r.Hooks.AfterReview; h != nil {
		if err := h(ctx, rctx, reviewResult); err != nil {
			return fmt.Errorf("after review hook: %w", err)
		}
	}

	// --- Artifacts ---
	diffContent := ""
	if result != nil {
		diffContent = result.Diff
	}
	summary := r.generateSummary(tk, record, diffContent, reviewResult)
	_ = r.Workspace.SaveFile("summary.md", []byte(summary)) //nolint:errcheck // best-effort save

	prBody := r.generatePRBody(tk, plan, diffContent, reviewResult)
	_ = r.Workspace.SaveFile("pr_body.md", []byte(prBody)) //nolint:errcheck // best-effort save

	record.Status = state.RunStatusCompleted
	record.FinishedAt = time.Now()
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	duration := time.Since(startTime)
	r.emit(ctx, tk.ID, event.TypeRunCompleted, map[string]interface{}{
		"duration": duration.String(),
		"retries":  result.Retries,
		"status":   "completed",
	})
	_ = r.Logger.Log("info", "runtime", "run completed", map[string]interface{}{ //nolint:errcheck // best-effort log
		"duration": duration.String(),
		"retries":  result.Retries,
	})

	slog.Info("run completed", "duration", duration.Round(time.Second), "path", r.Workspace.RunPath())

	if h := r.Hooks.AfterRun; h != nil {
		if err := h(ctx, rctx, result); err != nil {
			return fmt.Errorf("after run hook: %w", err)
		}
	}

	return nil
}

func (r *Runtime) saveExecutionArtifacts(result *ExecutionResult) {
	if result == nil {
		return
	}
	if result.Diff != "" {
		_ = r.Workspace.SaveFile("diff.patch", []byte(result.Diff)) //nolint:errcheck // best-effort save
	}
	if result.TestLog != "" {
		_ = r.Workspace.SaveFile("test.log", []byte(result.TestLog)) //nolint:errcheck // best-effort save
	}
	if result.LintLog != "" {
		_ = r.Workspace.SaveFile("lint.log", []byte(result.LintLog)) //nolint:errcheck // best-effort save
	}
}

func (r *Runtime) generateSummary(tk *task.Task, record *state.RunRecord, diff string, review *ReviewResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Run Summary: %s\n\n", tk.ID))
	b.WriteString(fmt.Sprintf("**Task:** %s\n", tk.Title))
	b.WriteString(fmt.Sprintf("**Status:** %s\n", record.Status))
	b.WriteString(fmt.Sprintf("**Duration:** %s\n\n", time.Since(record.StartedAt).Round(time.Second)))
	b.WriteString(fmt.Sprintf("## Description\n\n%s\n\n", tk.Description))

	if diff != "" {
		lineCount := strings.Count(diff, "\n")
		b.WriteString(fmt.Sprintf("## Changes\n\n- Files changed\n- Diff lines: %d\n\n", lineCount))
		b.WriteString("```diff\n")
		b.WriteString(diff)
		b.WriteString("\n```\n")
	}

	if review != nil {
		b.WriteString(fmt.Sprintf("## Review\n\n**Approved:** %v\n\n", review.Approved))
		b.WriteString(fmt.Sprintf("%s\n", review.Summary))
		if len(review.Issues) > 0 {
			b.WriteString("\n### Issues\n\n")
			for _, issue := range review.Issues {
				b.WriteString(fmt.Sprintf("- [%s] %s:%d - %s\n", issue.Severity, issue.File, issue.Line, issue.Message))
			}
		}
	}

	return b.String()
}

func (r *Runtime) generatePRBody(tk *task.Task, plan *Plan, diff string, review *ReviewResult) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", tk.Title))
	b.WriteString("## Description\n\n")
	b.WriteString(tk.Description)
	b.WriteString("\n\n## Changes\n\n")

	if diff != "" {
		b.WriteString("```diff\n")
		b.WriteString(diff)
		b.WriteString("\n```\n")
	}

	if plan != nil {
		b.WriteString("\n## Plan\n\n")
		b.WriteString(plan.Summary)
		b.WriteString("\n\n")
		for _, step := range plan.Steps {
			b.WriteString(fmt.Sprintf("%d. **%s**: %s\n", step.StepNumber, step.Action, step.Description))
		}
	}

	if review != nil {
		b.WriteString("\n## Review\n\n")
		b.WriteString(review.Summary)
	}

	return b.String()
}

func mustMarshalJSON(v interface{}) []byte {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return data
}

func mustMarshalYAML(v interface{}) []byte {
	data, err := yaml.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// ParsePlan extracts a Plan from an LLM chat response by unmarshalling the JSON content.
func ParsePlan(resp *llm.ChatResponse) (*Plan, error) {
	content := resp.Choices[0].Message.Content
	content = stripJSONFences(content)
	var plan Plan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("parse plan JSON: %w\ncontent: %s", err, content)
	}
	return &plan, nil
}

// ParseReview extracts a ReviewResult from an LLM chat response by unmarshalling the JSON content.
func ParseReview(resp *llm.ChatResponse) (*ReviewResult, error) {
	content := resp.Choices[0].Message.Content
	content = stripJSONFences(content)
	var review ReviewResult
	if err := json.Unmarshal([]byte(content), &review); err != nil {
		return nil, fmt.Errorf("parse review JSON: %w\ncontent: %s", err, content)
	}
	return &review, nil
}

func stripJSONFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	return s
}
