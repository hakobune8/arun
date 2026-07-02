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

// CreatePRRequest contains the parameters for creating a pull request.
type CreatePRRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Draft bool   `json:"draft,omitempty"`
}

// UpdatePRRequest contains the parameters for updating a pull request.
type UpdatePRRequest struct {
	State string `json:"state,omitempty"`
}

type createPRResponse struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

// CreatePR creates a new pull request on GitHub.
func (c *Client) CreatePR(req CreatePRRequest) (*PullRequest, error) {
	path := fmt.Sprintf("/%s/pulls", c.RepoPath())

	var resp createPRResponse
	if err := c.doJSON("POST", path, req, &resp); err != nil {
		return nil, fmt.Errorf("create PR: %w", err)
	}

	return &PullRequest{
		Number:  resp.Number,
		Title:   resp.Title,
		HTMLURL: resp.HTMLURL,
		State:   resp.State,
		Head:    req.Head,
		Base:    req.Base,
	}, nil
}

// ClosePR closes an existing pull request.
func (c *Client) ClosePR(number int) (*PullRequest, error) {
	path := fmt.Sprintf("/%s/pulls/%d", c.RepoPath(), number)

	var resp createPRResponse
	if err := c.doJSON("PATCH", path, UpdatePRRequest{State: "closed"}, &resp); err != nil {
		return nil, fmt.Errorf("close PR: %w", err)
	}

	return &PullRequest{
		Number:  resp.Number,
		Title:   resp.Title,
		HTMLURL: resp.HTMLURL,
		State:   resp.State,
	}, nil
}

// ListPRs lists pull requests, optionally filtered by state.
func (c *Client) ListPRs(state string) ([]PullRequest, error) {
	if state == "" {
		state = "open"
	}
	path := fmt.Sprintf("/%s/pulls?state=%s", c.RepoPath(), state)

	var prs []struct {
		Number  int    `json:"number"`
		Title   string `json:"title"`
		HTMLURL string `json:"html_url"`
		State   string `json:"state"`
		Head    struct {
			Ref string `json:"ref"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := c.doJSON("GET", path, nil, &prs); err != nil {
		return nil, fmt.Errorf("list PRs: %w", err)
	}

	var result []PullRequest
	for _, pr := range prs {
		result = append(result, PullRequest{
			Number:  pr.Number,
			Title:   pr.Title,
			HTMLURL: pr.HTMLURL,
			State:   pr.State,
			Head:    pr.Head.Ref,
			Base:    pr.Base.Ref,
		})
	}
	return result, nil
}
