package task

import (
	"fmt"

	gh "github.com/kazyamaz200/agentos/internal/github"
)

func FromGitHubIssue(issue *gh.Issue, repo string) *Task {
	branchName := fmt.Sprintf("agent/issue-%d", issue.Number)
	title := issue.Title
	body := issue.Body
	if body == "" {
		body = "No description provided."
	}

	return &Task{
		ID:          fmt.Sprintf("issue-%d", issue.Number),
		Type:        "issue_to_patch",
		Repo:        repo,
		BaseBranch:  "main",
		Branch:      branchName,
		Title:       title,
		Description: body,
	}
}
