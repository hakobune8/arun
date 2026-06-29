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
	agentDefs      []agentInfo
	strategy       Strategy
	cfg            *runtime.Config
	baseBranch     string
	subtaskTimeout time.Duration
}

type agentInfo struct {
	name        string
	description string
}

// NewOrchestrator creates a new Orchestrator with the given llm client, sandbox, and agents.
func NewOrchestrator(llmClient llm.LLMClient, sb sandbox.Sandbox, agents map[string]runtime.Agent, cfg *runtime.Config) *Orchestrator {
	var infos []agentInfo
	for name, a := range agents {
		infos = append(infos, agentInfo{name: name, description: a.Name()})
		_ = a
	}
	return &Orchestrator{
		llm:        llmClient,
		sandbox:    sb,
		agents:     agents,
		agentDefs:  infos,
		strategy:   StrategySequential,
		cfg:        cfg,
		baseBranch: "main",
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

// SetSubtaskTimeout sets the maximum runtime for a single subtask.
func (o *Orchestrator) SetSubtaskTimeout(timeout time.Duration) {
	o.subtaskTimeout = timeout
}

// TaskPlan represents a breakdown of a task into subtasks.
type TaskPlan struct {
	Description string    `json:"description"`
	Subtasks    []Subtask `json:"subtasks"`
}

// Subtask represents a single unit of work within a task plan.
type Subtask struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	AgentName   string   `json:"agent_type"`
	Deps        []string `json:"dependencies"`
}

// SubtaskResult contains the result of executing a subtask.
type SubtaskResult struct {
	SubtaskID string `json:"subtask_id"`
	Output    string `json:"output"`
	Diff      string `json:"diff,omitempty"`
	Error     string `json:"error,omitempty"`
	Success   bool   `json:"success"`
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
      "dependencies": []
    }
  ]
}`,
	}

	agentsInfo := ""
	for _, info := range o.agentDefs {
		agentsInfo += fmt.Sprintf("- %s: %s\n", info.name, info.description)
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
		return nil, fmt.Errorf("LLM plan: %w", err)
	}

	content := resp.Choices[0].Message.Content
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var plan TaskPlan
	if err := json.Unmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}

	plan.Description = taskDesc
	return &plan, nil
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
		sharedCtx := ""
		for _, subtask := range plan.Subtasks {
			result := o.executeObservedSubtask(ctx, subtask, sharedCtx, observer)
			results = append(results, result)
			if result.Diff != "" {
				sharedCtx = result.Diff
			}
		}
	case StrategyParallel:
		results = o.executeParallel(ctx, plan, observer)
	}

	if err := executionError(results); err != nil {
		return results, err
	}
	return results, nil
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
			if failed, reason := failedDependency(subtask, subtasksByID, completed, successful); failed {
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
			if !dependenciesSatisfied(subtask, completed, successful) {
				continue
			}

			started[subtask.ID] = true
			running++
			progressed = true
			go func(index int, st Subtask) {
				ch <- indexedSubtaskResult{o.executeObservedSubtask(ctx, st, "", observer), index}
			}(i, subtask)
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

func failedDependency(subtask Subtask, subtasksByID map[string]Subtask, completed, successful map[string]bool) (failed bool, reason string) {
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

func dependenciesSatisfied(subtask Subtask, completed, successful map[string]bool) bool {
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

func (o *Orchestrator) executeObservedSubtask(ctx context.Context, subtask Subtask, sharedCtx string, observer SubtaskObserver) SubtaskResult {
	started := time.Now().UTC()
	if observer != nil {
		observer(SubtaskEvent{Type: SubtaskStarted, Subtask: subtask, Started: started})
	}

	runCtx := ctx
	cancel := func() {}
	if o.subtaskTimeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, o.subtaskTimeout)
	}
	defer cancel()

	result := o.executeSubtask(runCtx, subtask, sharedCtx)
	if runCtx.Err() == context.DeadlineExceeded && result.Error == "" {
		result.Success = false
		result.Error = fmt.Sprintf("subtask timed out after %s", o.subtaskTimeout)
	}
	finished := time.Now().UTC()
	if observer != nil {
		observer(SubtaskEvent{Type: SubtaskCompleted, Subtask: subtask, Result: &result, Started: started, Finished: finished})
	}
	return result
}

func (o *Orchestrator) executeSubtask(ctx context.Context, subtask Subtask, sharedCtx string) SubtaskResult {
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
		ID:          subtask.ID,
		Type:        "orchestrated_subtask",
		Repo:        o.sandbox.RootDir(),
		BaseBranch:  o.baseBranch,
		Title:       subtask.Description,
		Description: subtask.Description,
		Branch:      fmt.Sprintf("agentos/%s", subtask.ID),
	}

	prof := subtaskProfile(agt.Name())
	runSandbox := sandbox.NewLocalSandbox(o.sandbox.RootDir())
	rt := runtime.NewRuntime(o.llm, &prof, runSandbox, o.cfg, agt)
	if err := rt.Run(ctx, tk); err != nil {
		return SubtaskResult{
			SubtaskID: subtask.ID,
			Success:   false,
			Error:     err.Error(),
		}
	}

	return SubtaskResult{
		SubtaskID: subtask.ID,
		Success:   true,
		Output:    fmt.Sprintf("Executed by %s: %s", agt.Name(), subtask.Description),
		Diff:      sharedCtx,
	}
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
	}

	return prof
}

// MergeResults combines subtask results into a formatted report.
func (o *Orchestrator) MergeResults(results []SubtaskResult) string {
	var b strings.Builder
	b.WriteString("# Multi-Agent Execution Results\n\n")
	for _, r := range results {
		status := "PASS"
		if !r.Success {
			status = "FAIL"
		}
		b.WriteString(fmt.Sprintf("## [%s] %s\n", status, r.SubtaskID))
		if r.Output != "" {
			b.WriteString(fmt.Sprintf("%s\n", r.Output))
		}
		if r.Error != "" {
			b.WriteString(fmt.Sprintf("Error: %s\n", r.Error))
		}
		b.WriteString("\n")
	}
	return b.String()
}
