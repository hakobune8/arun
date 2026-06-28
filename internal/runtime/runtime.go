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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/safety"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/state"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/kazyamaz200/agentos/internal/tools"
	"gopkg.in/yaml.v3"
)

// Planner defines the interface for generating execution plans from task context.
type Planner interface {
	Plan(ctx *RunContext) (*Plan, error)
}

// Runtime manages the end-to-end execution of a coding task, including planning,
// execution, testing, linting, review, and artifact generation.
type Runtime struct {
	LLM       llm.LLMClient
	Registry  *tools.Registry
	Store     *state.RunStore
	Policy    *safety.CommandPolicy
	Workspace *sandbox.Workspace
	Logger    *state.Logger
	Profile   *profile.Profile
	Config    *Config
	Planner   Planner
}

// NewRuntime creates a new Runtime with the given LLM client, profile, workspace, config, and planner.
func NewRuntime(llmClient llm.LLMClient, prof *profile.Profile, workspace *sandbox.Workspace, cfg *Config, planner Planner) *Runtime {
	registry := tools.NewRegistry()
	policy := safety.NewCommandPolicy(prof.Tools.DenyCommands)

	workDir := workspace.RootDir
	repoPath := workspace.RootDir

	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewSearchTool(workDir))
	registry.Register(tools.NewShellTool(policy, workDir))
	registry.Register(tools.NewGitTool(repoPath))
	registry.Register(tools.NewTestTool(workDir))

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
		Planner:   planner,
	}
}

// Run executes a coding task end-to-end: plan, execute, test, lint, review, and generate artifacts.
func (r *Runtime) Run(ctx context.Context, tk *task.Task) error {
	startTime := time.Now()

	if err := r.Workspace.PrepareRun(tk.ID); err != nil {
		return fmt.Errorf("prepare run: %w", err)
	}

	record := &state.RunRecord{
		TaskID:      tk.ID,
		Status:      state.RunStatusPending,
		StartedAt:   startTime,
		ProfileName: r.Profile.Name,
		Branch:      tk.Branch,
	}
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save
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

	record.Status = state.RunStatusPlanning
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	rctx := NewRunContext(ctx, tk, r)
	plan, err := r.Planner.Plan(rctx)
	if err != nil {
		record.Status = state.RunStatusFailed
		record.Error = err.Error()
		_ = r.Store.Save(record) //nolint:errcheck // best-effort save
		return fmt.Errorf("create plan: %w", err)
	}

	_ = r.Workspace.SaveFile("plan.json", mustMarshalJSON(plan)) //nolint:errcheck // best-effort save
	_ = r.Logger.Log("info", "runtime", "plan created", map[string]interface{}{ //nolint:errcheck // best-effort log
		"summary":              plan.Summary,
		"steps":                len(plan.Steps),
		"estimated_files":      plan.EstimatedFilesChanged,
	})

	record.Status = state.RunStatusExecuting
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	result, err := r.executePlan(ctx, tk, plan)
	if err != nil {
		record.Status = state.RunStatusFailed
		record.Error = err.Error()
		_ = r.Store.Save(record) //nolint:errcheck // best-effort save
		return fmt.Errorf("execute plan: %w", err)
	}

	if result.Diff != "" {
		_ = r.Workspace.SaveFile("diff.patch", []byte(result.Diff)) //nolint:errcheck // best-effort save
	}

	record.Status = state.RunStatusTesting
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	testResult := r.runTests(ctx, tk)
	if testResult != "" {
		_ = r.Workspace.SaveFile("test.log", []byte(testResult)) //nolint:errcheck // best-effort save
	}

	lintResult := r.runLint(ctx, tk)
	if lintResult != "" {
		_ = r.Workspace.SaveFile("lint.log", []byte(lintResult)) //nolint:errcheck // best-effort save
	}

	retryCount := 0
	maxRetries := r.Profile.Limits.MaxRetries
	for (testResult != "" || lintResult != "") && retryCount < maxRetries {
		retryCount++
		_ = r.Logger.Log("info", "runtime", "retry attempt", map[string]interface{}{ //nolint:errcheck // best-effort log
			"attempt": retryCount,
			"max":     maxRetries,
		})

		record.Iteration = retryCount
		record.Status = state.RunStatusExecuting
		_ = r.Store.Save(record) //nolint:errcheck // best-effort save

		result, err = r.executePlan(ctx, tk, plan)
		if err != nil {
			continue
		}
		if result.Diff != "" {
		_ = r.Workspace.SaveFile("diff.patch", []byte(result.Diff)) //nolint:errcheck // best-effort save
		}

		testResult = r.runTests(ctx, tk)
		if testResult != "" {
			_ = r.Workspace.SaveFile("test.log", []byte(testResult)) //nolint:errcheck // best-effort save
		}
		lintResult = r.runLint(ctx, tk)
		if lintResult != "" {
			_ = r.Workspace.SaveFile("lint.log", []byte(lintResult)) //nolint:errcheck // best-effort save
		}
	}

	record.Status = state.RunStatusReviewing
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	diffContent := ""
	if result != nil {
		diffContent = result.Diff
	}
	reviewResult, err := r.reviewResults(ctx, tk, diffContent, testResult)
	if err != nil {
		_ = r.Logger.Log("warn", "runtime", "review failed", err.Error()) //nolint:errcheck // best-effort log
	}

	summary := r.generateSummary(tk, record, diffContent, reviewResult)
	_ = r.Workspace.SaveFile("summary.md", []byte(summary)) //nolint:errcheck // best-effort save

	prBody := r.generatePRBody(tk, plan, diffContent, reviewResult)
	_ = r.Workspace.SaveFile("pr_body.md", []byte(prBody)) //nolint:errcheck // best-effort save

	record.Status = state.RunStatusCompleted
	record.FinishedAt = time.Now()
	_ = r.Store.Save(record) //nolint:errcheck // best-effort save

	duration := time.Since(startTime)
	_ = r.Logger.Log("info", "runtime", "run completed", map[string]interface{}{ //nolint:errcheck // best-effort log
		"duration": duration.String(),
		"retries":  retryCount,
	})

	slog.Info("run completed", "duration", duration.Round(time.Second), "path", r.Workspace.RunPath())

	return nil
}

func (r *Runtime) executePlan(ctx context.Context, tk *task.Task, plan *Plan) (*ExecutionResult, error) {
	result := &ExecutionResult{Success: true}

	gitTool := &tools.GitTool{RepoPath: r.Workspace.RootDir}
	currentBranch, _ := gitTool.CurrentBranch(ctx)

	if currentBranch != tk.Branch {
		coResult := gitTool.Run(ctx, tools.ToolInput{
			"subcommand": "checkout_new_branch",
			"args":       tk.Branch,
		})
		if !coResult.Success {
			_ = r.Logger.Log("warn", "runtime", "branch checkout", coResult.Error) //nolint:errcheck // best-effort log
		}
	}

	for _, step := range plan.Steps {
		_ = r.Logger.Log("info", "executor", fmt.Sprintf("step %d: %s", step.StepNumber, step.Description), nil) //nolint:errcheck // best-effort log

		stepResult := StepResult{
			StepNumber: step.StepNumber,
			Action:     step.Action,
		}
		stepStart := time.Now()

		switch step.Action {
		case "search":
			tool, ok := r.Registry.Get("search")
			if !ok {
				stepResult.Error = "search tool not found"
				stepResult.Success = false
			} else {
				pattern := ""
				if len(step.TargetFiles) > 0 {
					pattern = step.TargetFiles[0]
				}
				output := tool.Run(ctx, tools.ToolInput{"pattern": pattern})
				stepResult.Success = output.Success
				if output.Success {
					stepResult.Output = fmt.Sprintf("%v", output.Data)
				} else {
					stepResult.Error = output.Error
				}
			}

		case "read":
			tool, ok := r.Registry.Get("read_file")
			if !ok {
				stepResult.Error = "read_file tool not found"
				stepResult.Success = false
			} else {
				for _, f := range step.TargetFiles {
					output := tool.Run(ctx, tools.ToolInput{"file": f})
					if !output.Success {
						stepResult.Error = output.Error
						stepResult.Success = false
						break
					}
					stepResult.Output += fmt.Sprintf("=== %s ===\n%s\n", f, output.Data)
				}
				stepResult.Success = true
			}

		case "shell":
			tool, ok := r.Registry.Get("shell")
			if !ok {
				stepResult.Error = "shell tool not found"
				stepResult.Success = false
			} else {
				desc := step.Description
				output := tool.Run(ctx, tools.ToolInput{"command": desc})
				stepResult.Success = output.Success
				if output.Success {
					if data, ok := output.Data.(map[string]string); ok {
						stepResult.Output = data["stdout"]
					}
				} else {
					stepResult.Error = output.Error
				}
			}

		default:
			stepResult.Error = fmt.Sprintf("unknown action: %s", step.Action)
			stepResult.Success = false
		}

		stepResult.Duration = time.Since(stepStart)
		result.StepResults = append(result.StepResults, stepResult)

		_ = r.Logger.LogTool(step.Action, step, stepResult, stepResult.Duration) //nolint:errcheck // best-effort log
	}

	diffContent, err := gitTool.Diff(ctx)
	if err == nil {
		result.Diff = diffContent
	}

	return result, nil
}

func (r *Runtime) runTests(ctx context.Context, tk *task.Task) string {
	testCmd := r.Profile.Commands.Test
	if testCmd == "" {
		testCmd = "go test ./..."
	}

	_ = r.Logger.Log("info", "tester", "running tests", map[string]string{"command": testCmd}) //nolint:errcheck // best-effort log

	tool := tools.NewTestTool(r.Workspace.RootDir)
	output := tool.Run(ctx, tools.ToolInput{"command": testCmd})

	logData := ""
	if data, ok := output.Data.(map[string]string); ok {
		logData = data["stdout"] + "\n" + data["stderr"]
	}

	if !output.Success {
		_ = r.Logger.Log("warn", "tester", "tests failed", map[string]string{ //nolint:errcheck // best-effort log
			"output": logData,
			"error":  output.Error,
		})
		return logData
	}

	_ = r.Logger.Log("info", "tester", "tests passed", nil) //nolint:errcheck // best-effort log
	return ""
}

func (r *Runtime) runLint(ctx context.Context, tk *task.Task) string {
	lintCmd := r.Profile.Commands.Lint
	if lintCmd == "" {
		return ""
	}

	_ = r.Logger.Log("info", "linter", "running lint", map[string]string{"command": lintCmd}) //nolint:errcheck // best-effort log

	tool := tools.NewShellTool(r.Policy, r.Workspace.RootDir)
	output := tool.Run(ctx, tools.ToolInput{"command": lintCmd})

	logData := ""
	if data, ok := output.Data.(map[string]string); ok {
		logData = data["stdout"] + "\n" + data["stderr"]
	}

	if !output.Success {
		_ = r.Logger.Log("warn", "linter", "lint failed", map[string]string{ //nolint:errcheck // best-effort log
			"output": logData,
			"error":  output.Error,
		})
		return logData
	}

	_ = r.Logger.Log("info", "linter", "lint passed", nil) //nolint:errcheck // best-effort log
	return ""
}

func (r *Runtime) reviewResults(ctx context.Context, tk *task.Task, diff, testLog string) (*ReviewResult, error) {
	if diff == "" {
		return &ReviewResult{Approved: true, Summary: "No changes to review"}, nil
	}

	systemMsg := llm.Message{Role: llm.RoleSystem, Content: llm.SystemPromptReviewer}
	userMsg := llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf(`Review the following diff for task: %s

Description: %s

Diff:
%s

Test output:
%s`, tk.Title, tk.Description, diff, testLog),
	}

	resp, err := r.LLM.Chat(ctx, llm.ChatRequest{
		Model:       r.LLM.ModelName(),
		Messages:    []llm.Message{systemMsg, userMsg},
		Temperature: 0.1,
		MaxTokens:   r.Profile.LLM.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM review request: %w", err)
	}

	review, err := ParseReview(resp)
	if err != nil {
		return nil, fmt.Errorf("parse review: %w", err)
	}

	return review, nil
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

func init() {
	_ = os.MkdirAll(filepath.Join(os.TempDir(), ".agentos", "runs"), 0o755)
}
