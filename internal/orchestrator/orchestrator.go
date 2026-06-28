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

type Strategy string

const (
	StrategySequential Strategy = "sequential"
	StrategyParallel   Strategy = "parallel"
)

type Orchestrator struct {
	factory  *factory.Factory
	llm      llm.LLMClient
	agents   []*factory.AgentInstance
	strategy Strategy
}

func NewOrchestrator(f *factory.Factory, agents []*factory.AgentInstance) *Orchestrator {
	llmClient := llm.NewLiteLLMClient(llm.DefaultConfig())
	return &Orchestrator{
		factory:  f,
		llm:      llmClient,
		agents:   agents,
		strategy: StrategySequential,
	}
}

func (o *Orchestrator) SetStrategy(s Strategy) {
	o.strategy = s
}

type TaskPlan struct {
	Description string
	Subtasks    []Subtask
}

type Subtask struct {
	ID          string
	Description string
	AgentName   string
	Deps        []string
}

type SubtaskResult struct {
	SubtaskID string
	Output    string
	Error     string
	Success   bool
}

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
	if err := jsonUnmarshal([]byte(content), &plan); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}

	plan.Description = taskDesc
	return &plan, nil
}

func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

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

func (o *Orchestrator) Agents() []*factory.AgentInstance {
	return o.agents
}
