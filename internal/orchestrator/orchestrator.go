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
	"fmt"
	"strings"

	"github.com/kazyamaz200/agentos/internal/llm"
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
	llm       llm.LLMClient
	sandbox   sandbox.Sandbox
	agents    map[string]runtime.Agent
	agentDefs []agentInfo
	strategy  Strategy
	cfg       *runtime.Config
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
		llm:       llmClient,
		sandbox:   sb,
		agents:    agents,
		agentDefs: infos,
		strategy:  StrategySequential,
		cfg:       cfg,
	}
}

// DefaultAgent returns the first registered agent, used as fallback.
func (o *Orchestrator) DefaultAgent() runtime.Agent {
	for _, a := range o.agents {
		return a
	}
	return nil
}

// SetStrategy sets the execution strategy for the orchestrator.
func (o *Orchestrator) SetStrategy(s Strategy) {
	o.strategy = s
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
	var results []SubtaskResult

	switch o.strategy {
	case StrategySequential:
		sharedCtx := ""
		for _, subtask := range plan.Subtasks {
			result := o.executeSubtask(ctx, subtask, sharedCtx)
			results = append(results, result)
			if result.Diff != "" {
				sharedCtx = result.Diff
			}
		}
	case StrategyParallel:
		type subResult struct {
			result SubtaskResult
			index  int
		}
		ch := make(chan subResult, len(plan.Subtasks))
		for i, subtask := range plan.Subtasks {
			s := subtask
			idx := i
			go func() {
				ch <- subResult{o.executeSubtask(ctx, s, ""), idx}
			}()
		}
		results = make([]SubtaskResult, len(plan.Subtasks))
		for range plan.Subtasks {
			sr := <-ch
			results[sr.index] = sr.result
		}
	}

	return results, nil
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
		Title:       subtask.Description,
		Description: subtask.Description,
		Branch:      fmt.Sprintf("agentos/%s", subtask.ID),
	}

	rt := runtime.NewRuntime(o.llm, nil, o.sandbox, o.cfg, agt)
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
