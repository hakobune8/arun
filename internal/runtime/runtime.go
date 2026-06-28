package runtime

import (
	"context"
	"encoding/json"
	"fmt"
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

type Runtime struct {
	LLM       llm.LLMClient
	Registry  *tools.Registry
	Store     *state.RunStore
	Policy    *safety.CommandPolicy
	Workspace *sandbox.Workspace
	Logger    *state.Logger
	Profile   *profile.Profile
	Config    *Config
}

func NewRuntime(llmClient llm.LLMClient, prof *profile.Profile, workspace *sandbox.Workspace, cfg *Config) *Runtime {
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
	}
}

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
	r.Store.Save(record)
	r.Logger.Log("info", "runtime", "starting run", map[string]interface{}{
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
	r.Store.Save(record)

	plan, err := r.createPlan(ctx, tk)
	if err != nil {
		record.Status = state.RunStatusFailed
		record.Error = err.Error()
		r.Store.Save(record)
		return fmt.Errorf("create plan: %w", err)
	}

	r.Workspace.SaveFile("plan.json", mustMarshalJSON(plan))
	r.Logger.Log("info", "runtime", "plan created", map[string]interface{}{
		"summary":              plan.Summary,
		"steps":                len(plan.Steps),
		"estimated_files":      plan.EstimatedFilesChanged,
	})

	record.Status = state.RunStatusExecuting
	r.Store.Save(record)

	result, err := r.executePlan(ctx, tk, plan)
	if err != nil {
		record.Status = state.RunStatusFailed
		record.Error = err.Error()
		r.Store.Save(record)
		return fmt.Errorf("execute plan: %w", err)
	}

	if result.Diff != "" {
		r.Workspace.SaveFile("diff.patch", []byte(result.Diff))
	}

	record.Status = state.RunStatusTesting
	r.Store.Save(record)

	testResult := r.runTests(ctx, tk)
	if testResult != "" {
		r.Workspace.SaveFile("test.log", []byte(testResult))
	}

	lintResult := r.runLint(ctx, tk)
	if lintResult != "" {
		r.Workspace.SaveFile("lint.log", []byte(lintResult))
	}

	retryCount := 0
	maxRetries := r.Profile.Limits.MaxRetries
	for (testResult != "" || lintResult != "") && retryCount < maxRetries {
		retryCount++
		r.Logger.Log("info", "runtime", "retry attempt", map[string]interface{}{
			"attempt": retryCount,
			"max":     maxRetries,
		})

		record.Iteration = retryCount
		record.Status = state.RunStatusExecuting
		r.Store.Save(record)

		result, err = r.executePlan(ctx, tk, plan)
		if err != nil {
			continue
		}
		if result.Diff != "" {
			r.Workspace.SaveFile("diff.patch", []byte(result.Diff))
		}

		testResult = r.runTests(ctx, tk)
		if testResult != "" {
			r.Workspace.SaveFile("test.log", []byte(testResult))
		}
		lintResult = r.runLint(ctx, tk)
		if lintResult != "" {
			r.Workspace.SaveFile("lint.log", []byte(lintResult))
		}
	}

	record.Status = state.RunStatusReviewing
	r.Store.Save(record)

	diffContent := ""
	if result != nil {
		diffContent = result.Diff
	}
	reviewResult, err := r.reviewResults(ctx, tk, diffContent, testResult)
	if err != nil {
		r.Logger.Log("warn", "runtime", "review failed", err.Error())
	}

	summary := r.generateSummary(tk, record, diffContent, reviewResult)
	r.Workspace.SaveFile("summary.md", []byte(summary))

	prBody := r.generatePRBody(tk, plan, diffContent, reviewResult)
	r.Workspace.SaveFile("pr_body.md", []byte(prBody))

	record.Status = state.RunStatusCompleted
	record.FinishedAt = time.Now()
	r.Store.Save(record)

	duration := time.Since(startTime)
	r.Logger.Log("info", "runtime", "run completed", map[string]interface{}{
		"duration": duration.String(),
		"retries":  retryCount,
	})

	fmt.Printf("\nRun completed in %s\n", duration.Round(time.Second))
	fmt.Printf("Results saved to %s\n", r.Workspace.RunPath())

	return nil
}

func (r *Runtime) createPlan(ctx context.Context, tk *task.Task) (*Plan, error) {
	r.Logger.Log("info", "planner", "creating plan", nil)

	systemMsg := llm.Message{Role: llm.RoleSystem, Content: llm.SystemPromptPlanner}
	userMsg := llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf(`Task: %s
Description:
%s

Repository: %s
Base branch: %s

Create a plan to implement this task.`, tk.Title, tk.Description, tk.Repo, tk.BaseBranch),
	}

	resp, err := r.LLM.Chat(ctx, llm.ChatRequest{
		Model:       r.LLM.ModelName(),
		Messages:    []llm.Message{systemMsg, userMsg},
		Temperature: r.Profile.LLM.Temperature,
		MaxTokens:   r.Profile.LLM.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM plan request: %w", err)
	}

	r.Logger.LogLLM(
		llm.ChatRequest{Messages: []llm.Message{systemMsg, userMsg}},
		resp,
		r.LLM.ModelName(),
		0,
		getPromptTokens(resp),
		getCompletionTokens(resp),
	)

	plan, err := ParsePlan(resp)
	if err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}

	return plan, nil
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
			r.Logger.Log("warn", "runtime", "branch checkout", coResult.Error)
		}
	}

	for _, step := range plan.Steps {
		r.Logger.Log("info", "executor", fmt.Sprintf("step %d: %s", step.StepNumber, step.Description), nil)

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

		r.Logger.LogTool(step.Action, step, stepResult, stepResult.Duration)
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

	r.Logger.Log("info", "tester", "running tests", map[string]string{"command": testCmd})

	tool := tools.NewTestTool(r.Workspace.RootDir)
	output := tool.Run(ctx, tools.ToolInput{"command": testCmd})

	logData := ""
	if data, ok := output.Data.(map[string]string); ok {
		logData = data["stdout"] + "\n" + data["stderr"]
	}

	if !output.Success {
		r.Logger.Log("warn", "tester", "tests failed", map[string]string{
			"output": logData,
			"error":  output.Error,
		})
		return logData
	}

	r.Logger.Log("info", "tester", "tests passed", nil)
	return ""
}

func (r *Runtime) runLint(ctx context.Context, tk *task.Task) string {
	lintCmd := r.Profile.Commands.Lint
	if lintCmd == "" {
		return ""
	}

	r.Logger.Log("info", "linter", "running lint", map[string]string{"command": lintCmd})

	tool := tools.NewShellTool(r.Policy, r.Workspace.RootDir)
	output := tool.Run(ctx, tools.ToolInput{"command": lintCmd})

	logData := ""
	if data, ok := output.Data.(map[string]string); ok {
		logData = data["stdout"] + "\n" + data["stderr"]
	}

	if !output.Success {
		r.Logger.Log("warn", "linter", "lint failed", map[string]string{
			"output": logData,
			"error":  output.Error,
		})
		return logData
	}

	r.Logger.Log("info", "linter", "lint passed", nil)
	return ""
}

func (r *Runtime) reviewResults(ctx context.Context, tk *task.Task, diff string, testLog string) (*ReviewResult, error) {
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

func ParsePlan(resp *llm.ChatResponse) (*Plan, error) {
	content := resp.Choices[0].Message.Content
	content = stripJSONFences(content)
	var plan Plan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("parse plan JSON: %w\ncontent: %s", err, content)
	}
	return &plan, nil
}

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

func getPromptTokens(resp *llm.ChatResponse) int {
	if resp.Usage != nil {
		return resp.Usage.PromptTokens
	}
	return 0
}

func getCompletionTokens(resp *llm.ChatResponse) int {
	if resp.Usage != nil {
		return resp.Usage.CompletionTokens
	}
	return 0
}

func init() {
	_ = os.MkdirAll(filepath.Join(os.TempDir(), ".agentos", "runs"), 0755)
}
