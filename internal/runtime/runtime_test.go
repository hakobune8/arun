package runtime

import (
	"testing"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/sandbox"
)

type mockPlanner struct {
	plan *Plan
	err  error
}

func (m *mockPlanner) Plan(ctx *RunContext) (*Plan, error) {
	return m.plan, m.err
}

func TestNewRuntime(t *testing.T) {
	t.Parallel()

	mockLLM := llm.NewMockLLMClient(nil)
	prof := profile.DefaultProfile()
	ws := sandbox.NewWorkspace(t.TempDir())
	cfg := &Config{DryRun: true}
	planner := &mockPlanner{}

	rt := NewRuntime(mockLLM, &prof, ws, cfg, planner)

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
	if rt.Planner != planner {
		t.Error("Planner field not set correctly")
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
