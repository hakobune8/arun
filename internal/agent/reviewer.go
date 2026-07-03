// Copyright 2026 ARUN Authors
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

	"github.com/hakobune8/arun/internal/llm"
	"github.com/hakobune8/arun/internal/runtime"
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
