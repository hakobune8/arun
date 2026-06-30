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

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_API_URL", "https://api.example.com")
	c := NewClient("owner", "repo")
	if c == nil {
		t.Fatal("expected non-nil client")
		return
	}
	if c.RepoOwner != "owner" || c.RepoName != "repo" {
		t.Errorf("expected owner/repo, got %s/%s", c.RepoOwner, c.RepoName)
	}
	if c.Token != "test-token" {
		t.Errorf("expected token 'test-token', got %q", c.Token)
	}
	if c.BaseURL != "https://api.example.com" {
		t.Errorf("expected base URL 'https://api.example.com', got %q", c.BaseURL)
	}
}

func TestNewClient_NoToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_API_URL", "")
	c := NewClient("owner", "repo")
	if c == nil {
		t.Fatal("expected non-nil client")
		return
	}
	if c.Token != "" {
		t.Errorf("expected empty token, got %q", c.Token)
	}
	if c.BaseURL != "https://api.github.com" {
		t.Errorf("expected default base URL, got %q", c.BaseURL)
	}
}

func TestClient_RepoPath(t *testing.T) {
	t.Parallel()
	c := &Client{RepoOwner: "testorg", RepoName: "testrepo"}
	expected := "repos/testorg/testrepo"
	if got := c.RepoPath(); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestClient_Repo(t *testing.T) {
	t.Parallel()
	c := &Client{RepoOwner: "testorg", RepoName: "testrepo"}
	expected := "testorg/testrepo"
	if got := c.Repo(); got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestClient_GetCheckRuns(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/commits/main/check-runs") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string][]CheckRun{ //nolint:errcheck // test helper
			"check_runs": {
				{ID: 1, Name: "test", Conclusion: "success", Status: "completed"},
				{ID: 2, Name: "lint", Conclusion: "failure", Status: "completed"},
			},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	runs, err := c.GetCheckRuns("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
	if runs[0].Name != "test" || runs[0].Conclusion != "success" {
		t.Errorf("unexpected first run: %+v", runs[0])
	}
	if runs[1].Name != "lint" || runs[1].Conclusion != "failure" {
		t.Errorf("unexpected second run: %+v", runs[1])
	}
}

func TestClient_GetCheckRuns_Error(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`)) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	_, err := c.GetCheckRuns("main")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 error, got: %v", err)
	}
}

func TestClient_GetCheckRunAnnotations(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]string{ //nolint:errcheck // test helper
			{"path": "main.go", "message": "issue found", "annotation_level": "warning"},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	annotations, err := c.GetCheckRunAnnotations(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(annotations, "warning") || !strings.Contains(annotations, "main.go") {
		t.Errorf("unexpected annotations output: %q", annotations)
	}
}

func TestClient_GetCheckRunAnnotations_Error(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	_, err := c.GetCheckRunAnnotations(1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_GetCheckSuites(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string][]CheckSuite{ //nolint:errcheck // test helper
			"check_suites": {
				{ID: 1, Status: "completed", Conclusion: "success", HeadSHA: "abc123"},
			},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	suites, err := c.GetCheckSuites("main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(suites))
	}
	if suites[0].ID != 1 || suites[0].Conclusion != "success" {
		t.Errorf("unexpected suite: %+v", suites[0])
	}
}

func TestClient_GetWorkflowRunLogs(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("step 1: build\nstep 2: test\n")) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	logs, err := c.GetWorkflowRunLogs(42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logs != "step 1: build\nstep 2: test\n" {
		t.Errorf("unexpected logs: %q", logs)
	}
}

func TestClient_GetWorkflowRunLogs_Error(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	_, err := c.GetWorkflowRunLogs(42)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_CreatePR(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck // test helper
			"number":   1,
			"title":    "Test PR",
			"html_url": "https://github.com/owner/repo/pull/1",
			"state":    "open",
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	pr, err := c.CreatePR(CreatePRRequest{
		Title: "Test PR",
		Body:  "Description",
		Head:  "feature",
		Base:  "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pr.Number != 1 {
		t.Errorf("expected number 1, got %d", pr.Number)
	}
	if pr.Title != "Test PR" {
		t.Errorf("expected title 'Test PR', got %q", pr.Title)
	}
	if pr.State != "open" {
		t.Errorf("expected state 'open', got %q", pr.State)
	}
	if pr.Head != "feature" || pr.Base != "main" {
		t.Errorf("unexpected head/base: %s/%s", pr.Head, pr.Base)
	}
}

func TestClient_CreatePR_Error(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation failed"}`)) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	_, err := c.CreatePR(CreatePRRequest{
		Title: "Test",
		Body:  "Body",
		Head:  "feature",
		Base:  "main",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_CreateIssue(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues" {
			t.Errorf("path = %s, want /repos/owner/repo/issues", r.URL.Path)
		}
		var req CreateIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Title != "Track orchestration" || req.Body != "body" || len(req.Labels) != 1 || req.Labels[0] != "agentos" {
			t.Fatalf("request = %+v", req)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck // test helper
			"number":   7,
			"title":    req.Title,
			"body":     req.Body,
			"html_url": "https://github.com/owner/repo/issues/7",
			"state":    "open",
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	issue, err := c.CreateIssue(CreateIssueRequest{
		Title:  "Track orchestration",
		Body:   "body",
		Labels: []string{"agentos"},
	})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if issue.Number != 7 || issue.HTMLURL != "https://github.com/owner/repo/issues/7" {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestClient_CloseIssue(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/7" {
			t.Errorf("path = %s, want /repos/owner/repo/issues/7", r.URL.Path)
		}
		var req UpdateIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.State != "closed" {
			t.Fatalf("request = %+v", req)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck // test helper
			"number":   7,
			"title":    "Closed",
			"html_url": "https://github.com/owner/repo/issues/7",
			"state":    "closed",
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	issue, err := c.CloseIssue(7)
	if err != nil {
		t.Fatalf("CloseIssue() error = %v", err)
	}
	if issue.State != "closed" || issue.Number != 7 {
		t.Fatalf("issue = %+v", issue)
	}
}

func TestClient_CreateIssueComment(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/issues/7/comments" {
			t.Errorf("path = %s, want /repos/owner/repo/issues/7/comments", r.URL.Path)
		}
		var req CreateIssueCommentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Body != "AgentOS update" {
			t.Fatalf("request = %+v", req)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck // test helper
			"id":       101,
			"body":     req.Body,
			"html_url": "https://github.com/owner/repo/issues/7#issuecomment-101",
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	comment, err := c.CreateIssueComment(7, CreateIssueCommentRequest{Body: "AgentOS update"})
	if err != nil {
		t.Fatalf("CreateIssueComment() error = %v", err)
	}
	if comment.ID != 101 || comment.HTMLURL != "https://github.com/owner/repo/issues/7#issuecomment-101" {
		t.Fatalf("comment = %+v", comment)
	}
}

func TestClient_ListPRs(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{ //nolint:errcheck // test helper
			{
				"number":   1,
				"title":    "First PR",
				"html_url": "https://github.com/owner/repo/pull/1",
				"state":    "open",
				"head":     map[string]interface{}{"ref": "feature-a"},
				"base":     map[string]interface{}{"ref": "main"},
			},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	prs, err := c.ListPRs("open")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("expected 1 PR, got %d", len(prs))
	}
	if prs[0].Number != 1 || prs[0].Title != "First PR" {
		t.Errorf("unexpected PR: %+v", prs[0])
	}
}

func TestClient_ListIssues(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "open" {
			t.Errorf("expected state=open, got %q", r.URL.Query().Get("state"))
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode([]map[string]interface{}{ //nolint:errcheck // test helper
			{
				"number":   42,
				"title":    "Bug report",
				"body":     "Something broke",
				"state":    "open",
				"html_url": "https://github.com/owner/repo/issues/42",
			},
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	issues, err := c.ListIssues("open")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Number != 42 || issues[0].Title != "Bug report" {
		t.Errorf("unexpected issue: %+v", issues[0])
	}
}

func TestClient_ListIssues_DefaultState(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != "open" {
			t.Errorf("expected state=open, got %q", r.URL.Query().Get("state"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`)) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	issues, err := c.ListIssues("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

func TestClient_GetIssue(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck // test helper
			"number":   7,
			"title":    "Feature request",
			"body":     "Please add feature",
			"state":    "open",
			"html_url": "https://github.com/owner/repo/issues/7",
		})
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	issue, err := c.GetIssue(7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Number != 7 || issue.Title != "Feature request" {
		t.Errorf("unexpected issue: %+v", issue)
	}
}

func TestClient_GetIssue_Error(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`)) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	_, err := c.GetIssue(999)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_NonJSONResponse(t *testing.T) {
	t.Parallel()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`this is not json`)) //nolint:errcheck // test helper
	}))
	defer ts.Close()

	c := &Client{
		BaseURL:   ts.URL,
		Token:     "test-token",
		RepoOwner: "owner",
		RepoName:  "repo",
		http:      ts.Client(),
	}
	_, err := c.GetCheckRuns("main")
	if err == nil {
		t.Fatal("expected error for non-JSON response, got nil")
	}
}
