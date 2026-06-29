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
	"fmt"
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

		stepResult := runtime.StepResult{
			StepNumber: step.StepNumber,
			Action:     step.Action,
		}
		stepStart := time.Now()

		switch step.Action {
		case "search":
			tool, ok := ctx.Registry.Get("search")
			if !ok {
				stepResult.Error = "search tool not found"
				stepResult.Success = false
			} else {
				pattern := ""
				if len(step.TargetFiles) > 0 {
					pattern = step.TargetFiles[0]
				}
				output := tool.Run(ctx.Context, tools.ToolInput{"pattern": pattern})
				stepResult.Success = output.Success
				if output.Success {
					stepResult.Output = fmt.Sprintf("%v", output.Data)
				} else {
					stepResult.Error = output.Error
				}
			}

		case "read":
			tool, ok := ctx.Registry.Get("read_file")
			if !ok {
				stepResult.Error = "read_file tool not found"
				stepResult.Success = false
			} else {
				for _, f := range step.TargetFiles {
					output := tool.Run(ctx.Context, tools.ToolInput{"file": f})
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
			tool, ok := ctx.Registry.Get("shell")
			if !ok {
				stepResult.Error = "shell tool not found"
				stepResult.Success = false
			} else {
				desc := step.Description
				output := tool.Run(ctx.Context, tools.ToolInput{"command": desc})
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
	if testTool, ok := ctx.Registry.Get("test"); ok {
		ctx.Logger.Log("info", "tester", "running tests", map[string]string{"command": testCmd}) //nolint:errcheck // best-effort log
		output := testTool.Run(ctx.Context, tools.ToolInput{"command": testCmd})
		testLog := ""
		if data, ok := output.Data.(map[string]string); ok {
			testLog = data["stdout"] + "\n" + data["stderr"]
		}
		if !output.Success {
			ctx.Logger.Log("warn", "tester", "tests failed", map[string]string{"output": testLog, "error": output.Error}) //nolint:errcheck // best-effort log
		} else {
			ctx.Logger.Log("info", "tester", "tests passed", nil) //nolint:errcheck // best-effort log
		}
		result.TestLog = testLog
	}

	// Lint
	lintCmd := ctx.Profile.Commands.Lint
	if lintCmd != "" {
		if shellTool, ok := ctx.Registry.Get("shell"); ok {
			ctx.Logger.Log("info", "linter", "running lint", map[string]string{"command": lintCmd}) //nolint:errcheck // best-effort log
			output := shellTool.Run(ctx.Context, tools.ToolInput{"command": lintCmd})
			lintLog := ""
			if data, ok := output.Data.(map[string]string); ok {
				lintLog = data["stdout"] + "\n" + data["stderr"]
			}
			if !output.Success {
				ctx.Logger.Log("warn", "linter", "lint failed", map[string]string{"output": lintLog, "error": output.Error}) //nolint:errcheck // best-effort log
			} else {
				ctx.Logger.Log("info", "linter", "lint passed", nil) //nolint:errcheck // best-effort log
			}
			result.LintLog = lintLog
		}
	}

	// Retry loop
	maxRetries := ctx.MaxRetries
	for retryCount := 0; (result.TestLog != "" || result.LintLog != "") && retryCount < maxRetries; retryCount++ {
		result.Retries = retryCount + 1
		ctx.Logger.Log("info", "runtime", "retry attempt", map[string]interface{}{ //nolint:errcheck // best-effort log
			"attempt": retryCount + 1,
			"max":     maxRetries,
		})

		// Re-execute plan
		for _, step := range plan.Steps {
			ctx.Logger.Log("info", "executor", fmt.Sprintf("retry step %d: %s", step.StepNumber, step.Description), nil) //nolint:errcheck // best-effort log

			stepResult := runtime.StepResult{
				StepNumber: step.StepNumber,
				Action:     step.Action,
			}
			stepStart := time.Now()

			switch step.Action {
			case "search":
				tool, ok := ctx.Registry.Get("search")
				if !ok {
					stepResult.Error = "search tool not found"
					stepResult.Success = false
				} else {
					pattern := ""
					if len(step.TargetFiles) > 0 {
						pattern = step.TargetFiles[0]
					}
					output := tool.Run(ctx.Context, tools.ToolInput{"pattern": pattern})
					if output.Success {
						stepResult.Output = fmt.Sprintf("%v", output.Data)
					} else {
						stepResult.Error = output.Error
					}
					stepResult.Success = output.Success
				}

			case "read":
				tool, ok := ctx.Registry.Get("read_file")
				if !ok {
					stepResult.Error = "read_file tool not found"
					stepResult.Success = false
				} else {
					for _, f := range step.TargetFiles {
						output := tool.Run(ctx.Context, tools.ToolInput{"file": f})
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
				tool, ok := ctx.Registry.Get("shell")
				if !ok {
					stepResult.Error = "shell tool not found"
					stepResult.Success = false
				} else {
					output := tool.Run(ctx.Context, tools.ToolInput{"command": step.Description})
					if output.Success {
						if data, ok := output.Data.(map[string]string); ok {
							stepResult.Output = data["stdout"]
						}
					} else {
						stepResult.Error = output.Error
					}
					stepResult.Success = output.Success
				}

			default:
				stepResult.Error = fmt.Sprintf("unknown action: %s", step.Action)
				stepResult.Success = false
			}

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
			output := testTool.Run(ctx.Context, tools.ToolInput{"command": testCmd})
			testLog := ""
			if data, ok := output.Data.(map[string]string); ok {
				testLog = data["stdout"] + "\n" + data["stderr"]
			}
			result.TestLog = testLog
			if !output.Success {
				ctx.Logger.Log("warn", "tester", "tests failed on retry", map[string]string{"output": testLog, "error": output.Error}) //nolint:errcheck // best-effort log
			} else {
				ctx.Logger.Log("info", "tester", "tests passed on retry", nil) //nolint:errcheck // best-effort log
			}
		}

		// Re-run lint
		if lintCmd != "" {
			if shellTool, ok := ctx.Registry.Get("shell"); ok {
				output := shellTool.Run(ctx.Context, tools.ToolInput{"command": lintCmd})
				lintLog := ""
				if data, ok := output.Data.(map[string]string); ok {
					lintLog = data["stdout"] + "\n" + data["stderr"]
				}
				result.LintLog = lintLog
				if !output.Success {
					ctx.Logger.Log("warn", "linter", "lint failed on retry", map[string]string{"output": lintLog, "error": output.Error}) //nolint:errcheck // best-effort log
				} else {
					ctx.Logger.Log("info", "linter", "lint passed on retry", nil) //nolint:errcheck // best-effort log
				}
			}
		}
	}

	result.Success = true
	return result, nil
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
