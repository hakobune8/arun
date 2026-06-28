package github

import "fmt"

func (c *Client) ListIssues(state string) ([]Issue, error) {
	if state == "" {
		state = "open"
	}
	path := fmt.Sprintf("/%s/issues?state=%s&per_page=50", c.RepoPath(), state)

	var issues []Issue
	if err := c.doJSON("GET", path, nil, &issues); err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}

	return issues, nil
}

func (c *Client) GetIssue(number int) (*Issue, error) {
	path := fmt.Sprintf("/%s/issues/%d", c.RepoPath(), number)

	var issue Issue
	if err := c.doJSON("GET", path, nil, &issue); err != nil {
		return nil, fmt.Errorf("get issue: %w", err)
	}

	return &issue, nil
}
