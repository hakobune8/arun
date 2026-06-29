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

// Package agent provides core agent interfaces and base implementations for coding agents.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/tools"
)

// BaseAgent provides a default implementation of runtime.Agent that uses an LLM
// for planning and review, and the tool registry for execution.
type BaseAgent struct {
	name     string
	llm      llm.LLMClient
	planner  *Planner
	reviewer *Reviewer
}

type codingAction struct {
	Action    string `json:"action"`
	File      string `json:"file,omitempty"`
	Content   string `json:"content,omitempty"`
	Command   string `json:"command,omitempty"`
	Pattern   string `json:"pattern,omitempty"`
	Path      string `json:"path,omitempty"`
	Reasoning string `json:"reasoning,omitempty"`
}

// NewBaseAgent creates a new BaseAgent that uses the given LLM client.
func NewBaseAgent(name string, llmClient llm.LLMClient) *BaseAgent {
	return &BaseAgent{
		name:     name,
		llm:      llmClient,
		planner:  NewPlanner(llmClient),
		reviewer: NewReviewer(llmClient),
	}
}

// Name returns the agent's name.
func (a *BaseAgent) Name() string { return a.name }

// Plan generates an execution plan via the LLM.
func (a *BaseAgent) Plan(ctx *runtime.RunContext) (*runtime.Plan, error) {
	return a.planner.Plan(ctx)
}

// Execute carries out the plan using the tool registry, runs tests and lint,
// and retries on failure up to MaxRetries times.
func (a *BaseAgent) Execute(ctx *runtime.RunContext, plan *runtime.Plan) (*runtime.ExecutionResult, error) {
	result := &runtime.ExecutionResult{}
	tk := ctx.Task

	gitTool := &tools.GitTool{RepoPath: ctx.Workspace.RootDir()}
	currentBranch, _ := gitTool.CurrentBranch(ctx.Context)

	if currentBranch != tk.Branch {
		coResult := gitTool.Run(ctx.Context, tools.ToolInput{
			"subcommand": "checkout_new_branch",
			"args":       tk.Branch,
		})
		if !coResult.Success {
			ctx.Logger.Log("warn", "executor", "branch checkout failed", map[string]string{"error": coResult.Error}) //nolint:errcheck // best-effort log
		}
	}

	for _, step := range plan.Steps {
		ctx.Logger.Log("info", "executor", fmt.Sprintf("step %d: %s", step.StepNumber, step.Description), nil) //nolint:errcheck // best-effort log

		stepStart := time.Now()
		stepResult := a.executeStep(ctx, &step)
		stepResult.Duration = time.Since(stepStart)
		result.StepResults = append(result.StepResults, stepResult)

		ctx.Logger.LogTool(step.Action, step, stepResult, stepResult.Duration) //nolint:errcheck // best-effort log
	}

	diffContent, err := gitTool.Diff(ctx.Context)
	if err == nil {
		result.Diff = diffContent
	}

	// Tests
	testCmd := ctx.Profile.Commands.Test
	if testCmd == "" {
		testCmd = "go test ./..."
	}
	testPassed := true
	if testTool, ok := ctx.Registry.Get("test"); ok {
		result.TestLog, testPassed = a.runValidation(ctx, testTool, testCmd, "tester", "tests")
	}

	// Lint
	lintCmd := ctx.Profile.Commands.Lint
	lintPassed := true
	if lintCmd != "" {
		if shellTool, ok := ctx.Registry.Get("shell"); ok {
			result.LintLog, lintPassed = a.runValidation(ctx, shellTool, lintCmd, "linter", "lint")
		}
	}

	// Retry loop
	maxRetries := ctx.MaxRetries
	for retryCount := 0; (!testPassed || !lintPassed) && retryCount < maxRetries; retryCount++ {
		result.Retries = retryCount + 1
		ctx.Logger.Log("info", "runtime", "retry attempt", map[string]interface{}{ //nolint:errcheck // best-effort log
			"attempt": retryCount + 1,
			"max":     maxRetries,
		})

		// Re-execute plan
		for _, step := range plan.Steps {
			ctx.Logger.Log("info", "executor", fmt.Sprintf("retry step %d: %s", step.StepNumber, step.Description), nil) //nolint:errcheck // best-effort log

			stepStart := time.Now()
			stepResult := a.executeStep(ctx, &step)
			stepResult.Duration = time.Since(stepStart)
			result.StepResults = append(result.StepResults, stepResult)
			ctx.Logger.LogTool(step.Action, step, stepResult, stepResult.Duration) //nolint:errcheck // best-effort log
		}

		diffContent, err := gitTool.Diff(ctx.Context)
		if err == nil {
			result.Diff = diffContent
		}

		// Re-run tests
		if testTool, ok := ctx.Registry.Get("test"); ok {
			result.TestLog, testPassed = a.runValidation(ctx, testTool, testCmd, "tester", "tests")
		}

		// Re-run lint
		if lintCmd != "" {
			if shellTool, ok := ctx.Registry.Get("shell"); ok {
				result.LintLog, lintPassed = a.runValidation(ctx, shellTool, lintCmd, "linter", "lint")
			}
		}
	}

	if !testPassed || !lintPassed {
		var failed []string
		if !testPassed {
			failed = append(failed, "tests")
		}
		if !lintPassed {
			failed = append(failed, "lint")
		}
		result.Success = false
		result.Error = fmt.Sprintf("validation failed after %d retries: %s", result.Retries, strings.Join(failed, ", "))
		return result, fmt.Errorf("%s", result.Error)
	}

	result.Success = true
	return result, nil
}

func (a *BaseAgent) executeStep(ctx *runtime.RunContext, step *runtime.Step) runtime.StepResult {
	stepResult := runtime.StepResult{
		StepNumber: step.StepNumber,
		Action:     step.Action,
	}

	switch step.Action {
	case "test":
		return runToolStep(ctx, stepResult, "test", tools.ToolInput{"command": ctx.Profile.Commands.Test})
	case "lint":
		if ctx.Profile.Commands.Lint == "" {
			stepResult.Success = true
			stepResult.Output = "lint command not configured"
			return stepResult
		}
		return runToolStep(ctx, stepResult, "shell", tools.ToolInput{"command": ctx.Profile.Commands.Lint})
	case "search", "read", "edit", "shell":
		action, err := a.planCodingAction(ctx, step)
		if err != nil {
			stepResult.Error = err.Error()
			return stepResult
		}
		return a.runCodingAction(ctx, stepResult, action)
	default:
		stepResult.Error = fmt.Sprintf("unknown action: %s", step.Action)
		return stepResult
	}
}

func (a *BaseAgent) planCodingAction(ctx *runtime.RunContext, step *runtime.Step) (*codingAction, error) {
	stepJSON, _ := json.Marshal(step)
	resp, err := a.llm.Chat(ctx.Context, llm.ChatRequest{
		Model: a.llm.ModelName(),
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: llm.SystemPromptCoder},
			{
				Role: llm.RoleUser,
				Content: fmt.Sprintf(`Task: %s
Description:
%s

Repository: %s
Base branch: %s

Convert this execution plan step into exactly one concrete tool action JSON.
Plan step:
%s`, ctx.Task.Title, ctx.Task.Description, ctx.Task.Repo, ctx.Task.BaseBranch, string(stepJSON)),
			},
		},
		Temperature: ctx.Profile.LLM.Temperature,
		MaxTokens:   ctx.Profile.LLM.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("plan coding action: %w", err)
	}

	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var action codingAction
	if err := json.Unmarshal([]byte(content), &action); err != nil {
		return nil, fmt.Errorf("parse coding action JSON: %w", err)
	}
	if action.Action == "" {
		return nil, fmt.Errorf("coding action is required")
	}
	return &action, nil
}

func (a *BaseAgent) runCodingAction(ctx *runtime.RunContext, stepResult runtime.StepResult, action *codingAction) runtime.StepResult {
	switch action.Action {
	case "edit":
		stepResult.Action = "edit"
		return runToolStep(ctx, stepResult, "write_file", tools.ToolInput{"file": action.File, "content": action.Content})
	case "shell":
		stepResult.Action = "shell"
		return runToolStep(ctx, stepResult, "shell", tools.ToolInput{"command": action.Command})
	case "search":
		stepResult.Action = "search"
		return runToolStep(ctx, stepResult, "search", tools.ToolInput{"pattern": action.Pattern, "path": action.Path})
	case "read":
		stepResult.Action = "read"
		return runToolStep(ctx, stepResult, "read_file", tools.ToolInput{"file": action.File})
	default:
		stepResult.Error = fmt.Sprintf("unknown coding action: %s", action.Action)
		return stepResult
	}
}

func runToolStep(ctx *runtime.RunContext, stepResult runtime.StepResult, toolName string, input tools.ToolInput) runtime.StepResult {
	tool, ok := ctx.Registry.Get(toolName)
	if !ok {
		stepResult.Error = toolName + " tool not found"
		return stepResult
	}
	output := tool.Run(ctx.Context, input)
	stepResult.Success = output.Success
	if output.Success {
		switch data := output.Data.(type) {
		case string:
			stepResult.Output = data
		case map[string]string:
			stepResult.Output = strings.TrimSpace(data["stdout"] + "\n" + data["stderr"])
		default:
			stepResult.Output = fmt.Sprintf("%v", data)
		}
	} else {
		stepResult.Error = output.Error
	}
	return stepResult
}

func (a *BaseAgent) runValidation(ctx *runtime.RunContext, tool tools.Tool, command, component, label string) (string, bool) {
	ctx.Logger.Log("info", component, "running "+label, map[string]string{"command": command}) //nolint:errcheck // best-effort log
	output := tool.Run(ctx.Context, tools.ToolInput{"command": command})
	log := ""
	if data, ok := output.Data.(map[string]string); ok {
		log = strings.TrimSpace(data["stdout"] + "\n" + data["stderr"])
	}
	if !output.Success {
		ctx.Logger.Log("warn", component, label+" failed", map[string]string{"output": log, "error": output.Error}) //nolint:errcheck // best-effort log
		return log, false
	}
	ctx.Logger.Log("info", component, label+" passed", nil) //nolint:errcheck // best-effort log
	return log, true
}

// Review sends the result diff to the LLM for structured review.
func (a *BaseAgent) Review(ctx *runtime.RunContext, result *runtime.ExecutionResult) (*runtime.ReviewResult, error) {
	return a.reviewer.Review(ctx, result)
}

// RunAgent executes the full agent lifecycle (plan → execute → review) directly.
// This is a convenience for callers that want to run an agent without the Runtime wrapper.
func RunAgent(ctx context.Context, agent runtime.Agent, llmClient llm.LLMClient, rctx *runtime.RunContext) error {
	plan, err := agent.Plan(rctx)
	if err != nil {
		return fmt.Errorf("plan: %w", err)
	}
	execResult, err := agent.Execute(rctx, plan)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	_, err = agent.Review(rctx, execResult)
	if err != nil {
		return fmt.Errorf("review: %w", err)
	}
	return nil
}
