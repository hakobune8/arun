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
	"fmt"

	"github.com/hakobune8/arun/internal/llm"
	"github.com/hakobune8/arun/internal/runtime"
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
