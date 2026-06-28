package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/llm"
)

type CIFixer struct {
	llm    llm.LLMClient
	gh     *github.Client
}

func NewCIFixer(llmClient llm.LLMClient, ghClient *github.Client) *CIFixer {
	return &CIFixer{
		llm: llmClient,
		gh:  ghClient,
	}
}

type CIFixResult struct {
	Success      bool
	FailedChecks []FailedCheck
	FixSummary   string
	Error        string
}

type FailedCheck struct {
	Name       string
	Conclusion string
	Logs       string
	Annotations string
}

func (f *CIFixer) AnalyzeAndFix(ctx context.Context, ref string) (*CIFixResult, error) {
	checkRuns, err := f.gh.GetCheckRuns(ref)
	if err != nil {
		return nil, fmt.Errorf("get check runs: %w", err)
	}

	var failed []FailedCheck
	for _, cr := range checkRuns {
		if cr.Conclusion == "failure" || cr.Conclusion == "timed_out" || cr.Conclusion == "action_required" {
			fc := FailedCheck{
				Name:       cr.Name,
				Conclusion: cr.Conclusion,
			}

			annotations, err := f.gh.GetCheckRunAnnotations(cr.ID)
			if err == nil {
				fc.Annotations = annotations
			}

			failed = append(failed, fc)
		}
	}

	if len(failed) == 0 {
		return &CIFixResult{Success: true}, nil
	}

	result := &CIFixResult{
		FailedChecks: failed,
	}

	fixSummary, err := f.generateFix(ctx, failed)
	if err != nil {
		result.Error = fmt.Sprintf("generate fix: %v", err)
		return result, nil
	}

	result.FixSummary = fixSummary
	result.Success = true

	return result, nil
}

func (f *CIFixer) generateFix(ctx context.Context, failed []FailedCheck) (string, error) {
	var b strings.Builder
	b.WriteString("The following CI checks failed:\n\n")
	for _, fc := range failed {
		b.WriteString(fmt.Sprintf("## %s (%s)\n", fc.Name, fc.Conclusion))
		if fc.Annotations != "" {
			b.WriteString(fmt.Sprintf("Annotations:\n%s\n", fc.Annotations))
		}
	}

	systemMsg := llm.Message{
		Role: llm.RoleSystem,
		Content: `You are a CI fix agent. Analyze the CI failures and provide a detailed fix plan.

Output ONLY valid JSON with this structure:
{
  "analysis": "root cause analysis of the failures",
  "fix_steps": [
    {
      "file": "path/to/file.go",
      "change": "description of what to change",
      "code": "the corrected code"
    }
  ],
  "summary": "brief summary of the fix"
}`,
	}

	userMsg := llm.Message{
		Role:    llm.RoleUser,
		Content: b.String(),
	}

	resp, err := f.llm.Chat(ctx, llm.ChatRequest{
		Model:       f.llm.ModelName(),
		Messages:    []llm.Message{systemMsg, userMsg},
		Temperature: 0.1,
		MaxTokens:   8192,
	})
	if err != nil {
		return "", fmt.Errorf("LLM fix request: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}

func (f *CIFixer) Name() string {
	return "ci-fixer"
}
