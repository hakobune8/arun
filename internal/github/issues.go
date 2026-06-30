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

package github

import "fmt"

// CreateIssueRequest contains the parameters for creating an issue.
type CreateIssueRequest struct {
	Title  string   `json:"title"`
	Body   string   `json:"body,omitempty"`
	Labels []string `json:"labels,omitempty"`
}

// CreateIssueCommentRequest contains the parameters for creating an issue comment.
type CreateIssueCommentRequest struct {
	Body string `json:"body"`
}

// UpdateIssueRequest contains the parameters for updating an issue.
type UpdateIssueRequest struct {
	State string `json:"state,omitempty"`
}

// CreateIssue creates a new GitHub issue.
func (c *Client) CreateIssue(req CreateIssueRequest) (*Issue, error) {
	path := fmt.Sprintf("/%s/issues", c.RepoPath())

	var issue Issue
	if err := c.doJSON("POST", path, req, &issue); err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}

	return &issue, nil
}

// CloseIssue closes an existing GitHub issue.
func (c *Client) CloseIssue(number int) (*Issue, error) {
	path := fmt.Sprintf("/%s/issues/%d", c.RepoPath(), number)

	var issue Issue
	if err := c.doJSON("PATCH", path, UpdateIssueRequest{State: "closed"}, &issue); err != nil {
		return nil, fmt.Errorf("close issue: %w", err)
	}

	return &issue, nil
}

// CreateIssueComment creates a comment on an existing GitHub issue.
func (c *Client) CreateIssueComment(number int, req CreateIssueCommentRequest) (*IssueComment, error) {
	path := fmt.Sprintf("/%s/issues/%d/comments", c.RepoPath(), number)

	var comment IssueComment
	if err := c.doJSON("POST", path, req, &comment); err != nil {
		return nil, fmt.Errorf("create issue comment: %w", err)
	}

	return &comment, nil
}

// ListIssues lists GitHub issues, optionally filtered by state.
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

// GetIssue retrieves a single GitHub issue by number.
func (c *Client) GetIssue(number int) (*Issue, error) {
	path := fmt.Sprintf("/%s/issues/%d", c.RepoPath(), number)

	var issue Issue
	if err := c.doJSON("GET", path, nil, &issue); err != nil {
		return nil, fmt.Errorf("get issue: %w", err)
	}

	return &issue, nil
}
