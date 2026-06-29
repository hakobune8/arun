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

package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/task"
)

type mockAgent struct {
	plan          *Plan
	err           error
	executeResult *ExecutionResult
	executeErr    error
}

func (m *mockAgent) Name() string { return "mock-agent" }

func (m *mockAgent) Plan(ctx *RunContext) (*Plan, error) {
	return m.plan, m.err
}

func (m *mockAgent) Execute(ctx *RunContext, plan *Plan) (*ExecutionResult, error) {
	if m.executeResult != nil || m.executeErr != nil {
		return m.executeResult, m.executeErr
	}
	return &ExecutionResult{Success: true}, nil
}

func (m *mockAgent) Review(ctx *RunContext, result *ExecutionResult) (*ReviewResult, error) {
	return &ReviewResult{Approved: true, Summary: "mock review"}, nil
}

func TestNewRuntime(t *testing.T) {
	t.Parallel()

	mockLLM := llm.NewMockLLMClient(nil)
	prof := profile.DefaultProfile()
	ws := sandbox.NewWorkspace(t.TempDir())
	cfg := &Config{DryRun: true}
	agt := &mockAgent{}

	rt := NewRuntime(mockLLM, &prof, ws, cfg, agt)

	if rt.LLM != mockLLM {
		t.Error("LLM field not set correctly")
	}
	if rt.Profile != &prof {
		t.Error("Profile field not set correctly")
	}
	if rt.Workspace != ws {
		t.Error("Workspace field not set correctly")
	}
	if rt.Config != cfg {
		t.Error("Config field not set correctly")
	}
	if rt.Agent != agt {
		t.Error("Agent field not set correctly")
	}
	if rt.Registry == nil {
		t.Error("Registry should not be nil")
	}
	if rt.Store == nil {
		t.Error("Store should not be nil")
	}
	if rt.Logger == nil {
		t.Error("Logger should not be nil")
	}
	if rt.Policy == nil {
		t.Error("Policy should not be nil")
	}
}

func TestNewRuntime_RespectsAllowedTools(t *testing.T) {
	t.Parallel()

	mockLLM := llm.NewMockLLMClient(nil)
	prof := profile.DefaultProfile()
	prof.Tools.Allow = []string{"read_file", "git"}
	ws := sandbox.NewWorkspace(t.TempDir())
	rt := NewRuntime(mockLLM, &prof, ws, &Config{}, &mockAgent{})

	if _, ok := rt.Registry.Get("read_file"); !ok {
		t.Fatal("read_file tool was not registered")
	}
	if _, ok := rt.Registry.Get("git"); !ok {
		t.Fatal("git tool was not registered")
	}
	if _, ok := rt.Registry.Get("test"); ok {
		t.Fatal("test tool was registered despite not being allowed")
	}
	if _, ok := rt.Registry.Get("shell"); ok {
		t.Fatal("shell tool was registered despite not being allowed")
	}
}

func TestRuntime_RunSavesExecutionArtifactsOnFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGENTOS_HOME", home)

	prof := profile.DefaultProfile()
	ws := sandbox.NewWorkspace(t.TempDir())
	agt := &mockAgent{
		plan: &Plan{Summary: "test"},
		executeResult: &ExecutionResult{
			TestLog: "test failed",
			LintLog: "lint failed",
		},
		executeErr: fmt.Errorf("validation failed"),
	}
	rt := NewRuntime(llm.NewMockLLMClient(nil), &prof, ws, &Config{}, agt)

	err := rt.Run(context.Background(), &task.Task{
		ID:         "failure-artifacts",
		Repo:       ws.RootDir(),
		BaseBranch: "main",
		Branch:     "agent/failure-artifacts",
		Title:      "failure artifacts",
	})
	if err == nil {
		t.Fatal("expected run error")
	}

	runDir := filepath.Join(home, "runs", "failure-artifacts")
	if data, err := os.ReadFile(filepath.Join(runDir, "test.log")); err != nil || string(data) != "test failed" {
		t.Fatalf("test.log = %q, err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(runDir, "lint.log")); err != nil || string(data) != "lint failed" {
		t.Fatalf("lint.log = %q, err=%v", data, err)
	}
}

func TestParsePlan_ValidJSON(t *testing.T) {
	t.Parallel()

	resp := &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: `{"plan_summary": "Test plan", "steps": [{"step_number": 1, "action": "search", "description": "Find things", "target_files": ["main.go"], "reasoning": "Need to find"}], "estimated_files_changed": 2}`,
				},
			},
		},
	}

	plan, err := ParsePlan(resp)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v", err)
	}
	if plan.Summary != "Test plan" {
		t.Errorf("Summary = %q, want %q", plan.Summary, "Test plan")
	}
	if len(plan.Steps) != 1 {
		t.Fatalf("len(Steps) = %d, want 1", len(plan.Steps))
	}
	if plan.Steps[0].StepNumber != 1 {
		t.Errorf("StepNumber = %d, want 1", plan.Steps[0].StepNumber)
	}
	if plan.Steps[0].Action != "search" {
		t.Errorf("Action = %q, want %q", plan.Steps[0].Action, "search")
	}
	if plan.Steps[0].Description != "Find things" {
		t.Errorf("Description = %q, want %q", plan.Steps[0].Description, "Find things")
	}
	if plan.EstimatedFilesChanged != 2 {
		t.Errorf("EstimatedFilesChanged = %d, want 2", plan.EstimatedFilesChanged)
	}
}

func TestParsePlan_ValidJSONWithFences(t *testing.T) {
	t.Parallel()

	resp := &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "```json\n{\"plan_summary\": \"Fenced plan\", \"steps\": [], \"estimated_files_changed\": 0}\n```",
				},
			},
		},
	}

	plan, err := ParsePlan(resp)
	if err != nil {
		t.Fatalf("ParsePlan() error = %v", err)
	}
	if plan.Summary != "Fenced plan" {
		t.Errorf("Summary = %q, want %q", plan.Summary, "Fenced plan")
	}
}

func TestParsePlan_InvalidJSON(t *testing.T) {
	t.Parallel()

	resp := &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "this is not json",
				},
			},
		},
	}

	_, err := ParsePlan(resp)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseReview_ValidJSON(t *testing.T) {
	t.Parallel()

	resp := &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: `{"approved": true, "summary": "Looks good", "issues": [{"severity": "warning", "file": "main.go", "line": 42, "message": "Consider renaming"}]}`,
				},
			},
		},
	}

	review, err := ParseReview(resp)
	if err != nil {
		t.Fatalf("ParseReview() error = %v", err)
	}
	if !review.Approved {
		t.Error("Approved should be true")
	}
	if review.Summary != "Looks good" {
		t.Errorf("Summary = %q, want %q", review.Summary, "Looks good")
	}
	if len(review.Issues) != 1 {
		t.Fatalf("len(Issues) = %d, want 1", len(review.Issues))
	}
	if review.Issues[0].Severity != "warning" {
		t.Errorf("Severity = %q, want %q", review.Issues[0].Severity, "warning")
	}
	if review.Issues[0].File != "main.go" {
		t.Errorf("File = %q, want %q", review.Issues[0].File, "main.go")
	}
	if review.Issues[0].Line != 42 {
		t.Errorf("Line = %d, want 42", review.Issues[0].Line)
	}
	if review.Issues[0].Message != "Consider renaming" {
		t.Errorf("Message = %q, want %q", review.Issues[0].Message, "Consider renaming")
	}
}

func TestParseReview_ValidJSONWithFences(t *testing.T) {
	t.Parallel()

	resp := &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "```\n{\"approved\": false, \"summary\": \"Needs work\", \"issues\": []}\n```",
				},
			},
		},
	}

	review, err := ParseReview(resp)
	if err != nil {
		t.Fatalf("ParseReview() error = %v", err)
	}
	if review.Approved {
		t.Error("Approved should be false")
	}
	if review.Summary != "Needs work" {
		t.Errorf("Summary = %q, want %q", review.Summary, "Needs work")
	}
}

func TestParseReview_InvalidJSON(t *testing.T) {
	t.Parallel()

	resp := &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: "not json at all",
				},
			},
		},
	}

	_, err := ParseReview(resp)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
