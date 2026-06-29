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
	"errors"
	"testing"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/task"
)

func TestNewReviewer(t *testing.T) {
	t.Parallel()
	mock := &mockLLM{}
	r := NewReviewer(mock)
	if r == nil {
		t.Fatal("expected non-nil reviewer")
		return
	}
	if r.llm != mock {
		t.Error("expected llm field to be set")
	}
}

func TestReviewer_Review_Approved(t *testing.T) {
	t.Parallel()
	mock := &mockLLM{}
	r := NewReviewer(mock)
	ctx := &runtime.RunContext{
		Task:    &task.Task{Title: "test", Description: "desc"},
		Profile: &profile.Profile{},
	}
	result := &runtime.ExecutionResult{Diff: ""}
	review, err := r.Review(ctx, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !review.Approved {
		t.Error("expected approved when diff is empty")
	}
	if review.Summary != "No changes to review" {
		t.Errorf("expected 'No changes to review', got %q", review.Summary)
	}
}

func TestReviewer_Review_WithDiff(t *testing.T) {
	t.Parallel()
	content := `{"approved":true,"issues":[],"summary":"looks good"}`
	mock := &mockLLM{
		resp: &llm.ChatResponse{
			Choices: []llm.Choice{
				{Message: llm.Message{Content: content}},
			},
		},
	}
	r := NewReviewer(mock)
	ctx := &runtime.RunContext{
		Task:    &task.Task{Title: "test", Description: "desc"},
		Profile: &profile.Profile{LLM: profile.LLMConfig{MaxTokens: 8192}},
	}
	result := &runtime.ExecutionResult{Diff: "some diff"}
	review, err := r.Review(ctx, result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !review.Approved {
		t.Error("expected approved")
	}
	if review.Summary != "looks good" {
		t.Errorf("expected summary 'looks good', got %q", review.Summary)
	}
}

func TestReviewer_Review_Error(t *testing.T) {
	t.Parallel()
	mock := &mockLLM{err: errors.New("llm failure")}
	r := NewReviewer(mock)
	ctx := &runtime.RunContext{
		Task:    &task.Task{Title: "test", Description: "desc"},
		Profile: &profile.Profile{LLM: profile.LLMConfig{MaxTokens: 8192}},
	}
	result := &runtime.ExecutionResult{Diff: "some diff"}
	_, err := r.Review(ctx, result)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
