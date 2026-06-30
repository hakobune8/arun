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

import "time"

// Issue represents a GitHub issue.
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
	Labels    []Label   `json:"labels"`
}

// IssueComment represents a GitHub issue comment.
type IssueComment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
}

// Label represents a GitHub label.
type Label struct {
	Name string `json:"name"`
}

// PullRequest represents a GitHub pull request.
type PullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Head    string `json:"head"`
	Base    string `json:"base"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

// CheckRun represents a GitHub check run.
type CheckRun struct {
	ID          int         `json:"id"`
	Name        string      `json:"name"`
	Status      string      `json:"status"`
	Conclusion  string      `json:"conclusion"`
	HTMLURL     string      `json:"html_url"`
	CompletedAt time.Time   `json:"completed_at"`
	Output      CheckOutput `json:"output"`
}

// CheckOutput contains the output of a check run.
type CheckOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Text    string `json:"text"`
}

// CheckSuite represents a GitHub check suite.
type CheckSuite struct {
	ID         int    `json:"id"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HeadSHA    string `json:"head_sha"`
}

// Repo identifies a GitHub repository.
type Repo struct {
	Owner string
	Name  string
	Full  string
}

// RepositorySummary is a compact GitHub repository listing item.
type RepositorySummary struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Private       bool      `json:"private"`
	HTMLURL       string    `json:"html_url"`
	DefaultBranch string    `json:"default_branch"`
	UpdatedAt     time.Time `json:"updated_at"`
}
