// Package agent provides core agent interfaces and base implementations for coding agents.
package agent

import (
	"fmt"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/runtime"
)

// Planner uses an LLM to generate execution plans for coding tasks.
type Planner struct {
	llm llm.LLMClient
}

// NewPlanner creates a new Planner with the given LLM client.
func NewPlanner(llmClient llm.LLMClient) *Planner {
	return &Planner{llm: llmClient}
}

// Plan generates a plan for the given task by sending the task context to the LLM.
func (p *Planner) Plan(rctx *runtime.RunContext) (*runtime.Plan, error) {
	systemMsg := llm.Message{Role: llm.RoleSystem, Content: llm.SystemPromptPlanner}
	userMsg := llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf(`Task: %s
Description:
%s

Repository: %s
Base branch: %s

Create a plan to implement this task.`, rctx.Task.Title, rctx.Task.Description, rctx.Task.Repo, rctx.Task.BaseBranch),
	}

	resp, err := p.llm.Chat(rctx.Context, llm.ChatRequest{
		Model:       p.llm.ModelName(),
		Messages:    []llm.Message{systemMsg, userMsg},
		Temperature: rctx.Profile.LLM.Temperature,
		MaxTokens:   rctx.Profile.LLM.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM plan request: %w", err)
	}

	return runtime.ParsePlan(resp)
}
