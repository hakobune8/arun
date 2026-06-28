// Package agent provides core agent interfaces and base implementations for coding agents.
package agent

import (
	"context"
	"fmt"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/runtime"
)

// Reviewer uses an LLM to review code changes and execution results.
type Reviewer struct {
	llm llm.LLMClient
}

// NewReviewer creates a new Reviewer with the given LLM client.
func NewReviewer(llmClient llm.LLMClient) *Reviewer {
	return &Reviewer{llm: llmClient}
}

// Review sends the diff and task context to the LLM and returns a structured review result.
func (r *Reviewer) Review(ctx *runtime.RunContext, result *runtime.ExecutionResult) (*runtime.ReviewResult, error) {
	if result.Diff == "" {
		return &runtime.ReviewResult{Approved: true, Summary: "No changes to review"}, nil
	}

	systemMsg := llm.Message{Role: llm.RoleSystem, Content: llm.SystemPromptReviewer}
	userMsg := llm.Message{
		Role: llm.RoleUser,
		Content: fmt.Sprintf(`Review the following diff for task: %s

Description: %s

Diff:
%s`, ctx.Task.Title, ctx.Task.Description, result.Diff),
	}

	resp, err := r.llm.Chat(context.Background(), llm.ChatRequest{
		Model:       r.llm.ModelName(),
		Messages:    []llm.Message{systemMsg, userMsg},
		Temperature: 0.1,
		MaxTokens:   ctx.Profile.LLM.MaxTokens,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM review request: %w", err)
	}

	return runtime.ParseReview(resp)
}
