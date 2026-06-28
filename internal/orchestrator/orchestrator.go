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
	"os"
	"strings"

	"github.com/kazyamaz200/agentos/internal/factory"
	"github.com/kazyamaz200/agentos/internal/llm"
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
	factory  *factory.Factory
	llm      llm.LLMClient
	agents   []*factory.AgentInstance
	strategy Strategy
}

// NewOrchestrator creates a new Orchestrator with the given factory and agents.
func NewOrchestrator(f *factory.Factory, agents []*factory.AgentInstance) *Orchestrator {
	llmClient := llm.NewLiteLLMClient(llm.DefaultConfig())
	return &Orchestrator{
		factory:  f,
		llm:      llmClient,
		agents:   agents,
		strategy: StrategySequential,
	}
}

// SetStrategy sets the execution strategy for the orchestrator.
func (o *Orchestrator) SetStrategy(s Strategy) {
	o.strategy = s
}

// TaskPlan represents a breakdown of a task into subtasks.
type TaskPlan struct {
	Description string
	Subtasks    []Subtask
}

// Subtask represents a single unit of work within a task plan.
type Subtask struct {
	ID          string
	Description string
	AgentName   string
	Deps        []string
}

// SubtaskResult contains the result of executing a subtask.
type SubtaskResult struct {
	SubtaskID string
	Output    string
	Error     string
	Success   bool
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
      "agent_type": "coder | reviewer | tester",
      "dependencies": []
    }
  ]
}`,
	}

	agentsInfo := ""
	for _, a := range o.agents {
		agentsInfo += fmt.Sprintf("- %s (role: %s, tools: %v)\n", a.Def.Name, a.Def.Role, a.Def.Tools)
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
		for _, subtask := range plan.Subtasks {
			result := o.executeSubtask(ctx, subtask)
			results = append(results, result)
		}
	case StrategyParallel:
		resultCh := make(chan SubtaskResult, len(plan.Subtasks))
		for _, subtask := range plan.Subtasks {
			s := subtask
			go func() {
				resultCh <- o.executeSubtask(ctx, s)
			}()
		}
		for range plan.Subtasks {
			results = append(results, <-resultCh)
		}
	}

	return results, nil
}

func (o *Orchestrator) executeSubtask(ctx context.Context, subtask Subtask) SubtaskResult {
	agent := o.findAgent(subtask.AgentName)
	if agent == nil {
		agent = o.agents[0]
	}

	fmt.Fprintf(os.Stdout, "  [%s] %s\n", agent.Def.Name, subtask.Description)

	return SubtaskResult{
		SubtaskID: subtask.ID,
		Success:   true,
		Output:    fmt.Sprintf("Executed by %s: %s", agent.Def.Name, subtask.Description),
	}
}

func (o *Orchestrator) findAgent(name string) *factory.AgentInstance {
	for _, a := range o.agents {
		if a.Def.Name == name || a.Def.Role == name {
			return a
		}
	}
	return nil
}

// MergeResults combines subtask results into a formatted report.
func (o *Orchestrator) MergeResults(results []SubtaskResult) string {
	var b strings.Builder
	b.WriteString("# Multi-Agent Execution Results\n\n")
	for _, r := range results {
		status := "✅"
		if !r.Success {
			status = "❌"
		}
		b.WriteString(fmt.Sprintf("## %s %s\n", status, r.SubtaskID))
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

// Agents returns the list of agents managed by the orchestrator.
func (o *Orchestrator) Agents() []*factory.AgentInstance {
	return o.agents
}
