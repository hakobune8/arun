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

package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/task"
)

type mockLLM struct {
	resp *llm.ChatResponse
	err  error
}

func (m *mockLLM) Chat(_ context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	return m.resp, m.err
}

func (m *mockLLM) ModelName() string { return "mock-model" }

func TestNewPlanner(t *testing.T) {
	t.Parallel()
	mock := &mockLLM{}
	p := NewPlanner(mock)
	if p == nil {
		t.Fatal("expected non-nil planner")
		return
	}
	if p.llm != mock {
		t.Error("expected llm field to be set")
	}
}

func TestPlanner_Plan(t *testing.T) {
	t.Parallel()
	content := `{"plan_summary":"Add logging","steps":[{"step_number":1,"action":"read","description":"read main.go","target_files":["main.go"],"reasoning":"understand"}],"estimated_files_changed":1}`
	mock := &mockLLM{
		resp: &llm.ChatResponse{
			Choices: []llm.Choice{
				{Message: llm.Message{Content: content}},
			},
		},
	}
	p := NewPlanner(mock)
	rctx := &runtime.RunContext{
		Context: context.Background(),
		Task: &task.Task{
			Title:       "add logging",
			Description: "add structured logging",
			Repo:        "org/repo",
			BaseBranch:  "main",
		},
		Profile: &profile.Profile{
			LLM: profile.LLMConfig{Temperature: 0.2, MaxTokens: 8192},
		},
	}
	plan, err := p.Plan(rctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Summary != "Add logging" {
		t.Errorf("expected summary 'Add logging', got %q", plan.Summary)
	}
	if len(plan.Steps) != 1 {
		t.Errorf("expected 1 step, got %d", len(plan.Steps))
	}
	if plan.EstimatedFilesChanged != 1 {
		t.Errorf("expected EstimatedFilesChanged=1, got %d", plan.EstimatedFilesChanged)
	}
}

func TestPlanner_Plan_Error(t *testing.T) {
	t.Parallel()
	mock := &mockLLM{err: errors.New("llm failure")}
	p := NewPlanner(mock)
	rctx := &runtime.RunContext{
		Context: context.Background(),
		Task:    &task.Task{},
		Profile: &profile.Profile{},
	}
	_, err := p.Plan(rctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
