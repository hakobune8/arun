package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kazyamaz200/agentos/internal/orchestrator"
)

func TestNewServer_ReturnsServer(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	if s == nil {
		t.Fatal("NewServer returned nil")
		return
	}
	if s.server == nil {
		t.Error("http.Server is nil")
	}
}

func TestNewServer_SetsPort(t *testing.T) {
	t.Parallel()
	s := NewServer(8080)
	if s.port != 8080 {
		t.Errorf("port = %d, want 8080", s.port)
	}
}

func TestServer_ServerAddr(t *testing.T) {
	t.Parallel()
	s := NewServer(9999)
	if s.server == nil {
		t.Fatal("http.Server is nil")
	}
	if s.server.Addr != ":9999" {
		t.Errorf("Addr = %q, want %q", s.server.Addr, ":9999")
	}
}

func TestServer_Shutdown_NotStarted(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := s.Shutdown(ctx)
	if err != nil && err != http.ErrServerClosed {
		t.Fatalf("Shutdown: %v", err)
	}
}

func serveRequest(s *Server, method, path string, body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, http.NoBody)
	}
	s.server.Handler.ServeHTTP(w, req)
	return w
}

func serveRequestAs(s *Server, method, path string, body []byte, user *authUser) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, http.NoBody)
	}
	session, err := json.Marshal(user)
	if err != nil {
		panic(err)
	}
	req.AddCookie(signedCookie(sessionCookieName, string(session), time.Hour, s.auth.SessionSecret))
	s.server.Handler.ServeHTTP(w, req)
	return w
}

func assertStatus(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

func assertJSON(t *testing.T, body []byte, key, want string) {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, ok := m[key]
	if !ok {
		t.Errorf("key %q not found in response", key)
		return
	}
	gotStr, _ := got.(string)
	if gotStr != want {
		t.Errorf("response[%q] = %q, want %q", key, gotStr, want)
	}
}

func assertArrayLen(t *testing.T, body []byte, want int) {
	t.Helper()
	var arr []interface{}
	if err := json.Unmarshal(body, &arr); err != nil {
		t.Fatalf("unmarshal array: %v", err)
	}
	if len(arr) != want {
		t.Errorf("array length = %d, want %d", len(arr), want)
	}
}

// --- Health ---

func TestServer_HealthEndpoint(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/health", nil)
	assertStatus(t, w.Code, http.StatusOK)
	assertJSON(t, w.Body.Bytes(), "status", "ok")
}

// --- Agents ---

func TestServer_AgentsEndpoint_ReturnsList(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/agents", nil)
	assertStatus(t, w.Code, http.StatusOK)
	assertArrayLen(t, w.Body.Bytes(), 4)
}

func TestServer_AgentsEndpoint_GoBackendExists(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/agents", nil)
	var agents []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &agents); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, a := range agents {
		if a["Name"] == "go-backend" {
			found = true
			break
		}
	}
	if !found {
		t.Error("go-backend agent not found in list")
	}
}

// --- Search ---

func TestServer_SearchEndpoint_NoQuery(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/search", nil)
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_SearchEndpoint_WithQuery(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/search?q=test", nil)
	assertStatus(t, w.Code, http.StatusOK)
	assertArrayLen(t, w.Body.Bytes(), 0)
}

// --- Runs ---

func TestServer_RunsEndpoint_ReturnsEmptyList(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/runs", nil)
	assertStatus(t, w.Code, http.StatusOK)
	// Should return [] not null
	body := strings.TrimSpace(w.Body.String())
	if body == "null" {
		t.Error("runs endpoint returned null, expected []")
	}
}

func TestServer_CreateRun_MissingAgent(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	body := `{"task":"test task"}`
	w := serveRequest(s, "POST", "/api/runs", []byte(body))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_CreateRun_MissingTask(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	body := `{"agent":"go-backend"}`
	w := serveRequest(s, "POST", "/api/runs", []byte(body))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_CreateRun_InvalidAgent(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	body := `{"agent":"nonexistent","task":"test"}`
	w := serveRequest(s, "POST", "/api/runs", []byte(body))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_CreateRun_ValidRequest(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	body := `{"agent":"go-backend","task":"add feature","description":"test"}`
	w := serveRequest(s, "POST", "/api/runs", []byte(body))
	assertStatus(t, w.Code, http.StatusOK)
	assertJSON(t, w.Body.Bytes(), "status", "started")
	// Verify run ID is returned
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if id, ok := resp["id"]; !ok || id == "" {
		t.Error("run id not returned")
	}
}

func TestServer_RunDetail_NotFound(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/runs/run-0123456789abcdef", nil)
	assertStatus(t, w.Code, http.StatusOK) // returns empty artifacts, not error
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if id, ok := resp["id"]; !ok || id != "run-0123456789abcdef" {
		t.Errorf("id = %v, want run-0123456789abcdef", id)
	}
}

func TestServer_RunDetail_RejectsInvalidID(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/runs/not-a-run-id", nil)
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_RunDetail_RedactsArtifacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGENTOS_HOME", home)

	runID := "run-0123456789abcdef"
	runDir := filepath.Join(home, "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(runDir, "summary.md"),
		[]byte("Authorization: Bearer ghp_123456789012345678901234567890123456"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/runs/"+runID, nil)
	assertStatus(t, w.Code, http.StatusOK)
	if strings.Contains(w.Body.String(), "ghp_123456789012345678901234567890123456") {
		t.Fatalf("run detail leaked token: %s", w.Body.String())
	}
}

// --- GitHub ---

func TestServer_GitHub_MissingRepo(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/github/issues", nil)
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_GitHub_InvalidRepo(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/github/issues?repo=invalid", nil)
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func testGitHubEndpoint(t *testing.T, path string) {
	t.Helper()
	s := NewServer(0)
	w := serveRequest(s, "GET", path, nil)
	// GitHub API may be unavailable on CI (no token), so accept 500
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 200 or 500", w.Code)
	}
	if w.Code == http.StatusOK {
		var arr []interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	}
}

func TestServer_GitHub_Issues_ValidRepo(t *testing.T) {
	t.Parallel()
	testGitHubEndpoint(t, "/api/github/issues?repo=kazyamaz200/agentos")
}

func TestServer_GitHub_Pulls_ValidRepo(t *testing.T) {
	t.Parallel()
	testGitHubEndpoint(t, "/api/github/pulls?repo=kazyamaz200/agentos")
}

func TestServer_GitHub_Checks_ValidRepo(t *testing.T) {
	t.Parallel()
	testGitHubEndpoint(t, "/api/github/checks?repo=kazyamaz200/agentos")
}

func TestServer_GitHub_EmptyListsReturnArrays(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/issues"):
			_, _ = w.Write([]byte(`[]`))
		case strings.Contains(r.URL.Path, "/pulls"):
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	t.Setenv("GITHUB_API_URL", api.URL)

	s := NewServer(0)
	for _, path := range []string{
		"/api/github/issues?repo=owner/repo",
		"/api/github/pulls?repo=owner/repo",
	} {
		w := serveRequest(s, "GET", path, nil)
		assertStatus(t, w.Code, http.StatusOK)
		if body := strings.TrimSpace(w.Body.String()); body != "[]" {
			t.Fatalf("%s body = %q, want []", path, body)
		}
	}
}

func TestServer_GitHub_ChecksReturnCheckRuns(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/commits/main/check-runs") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"check_runs":[{"id":1,"name":"build","status":"completed","conclusion":"success","html_url":"https://example.test/check/1"}]}`))
	}))
	defer api.Close()
	t.Setenv("GITHUB_API_URL", api.URL)

	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/github/checks?repo=owner/repo&ref=main", nil)
	assertStatus(t, w.Code, http.StatusOK)

	var runs []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &runs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1", len(runs))
	}
	if runs[0]["name"] != "build" || runs[0]["id"].(float64) != 1 {
		t.Fatalf("unexpected check run: %+v", runs[0])
	}
}

func TestServer_GitHub_UnknownResource(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/github/unknown?repo=kazyamaz200/agentos", nil)
	assertStatus(t, w.Code, http.StatusNotFound)
}

// --- Orchestrate ---

func TestServer_Orchestrate_RequiresPOST(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/orchestrate", nil)
	assertStatus(t, w.Code, http.StatusMethodNotAllowed)
}

func TestServer_Orchestrate_MissingAgents(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	body := `{"task":"test"}`
	w := serveRequest(s, "POST", "/api/orchestrate", []byte(body))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_Orchestrate_MissingTask(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	body := `{"agents":["go-backend"]}`
	w := serveRequest(s, "POST", "/api/orchestrate", []byte(body))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_Orchestrate_InvalidAgent(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	body := `{"agents":["nonexistent"],"task":"test"}`
	w := serveRequest(s, "POST", "/api/orchestrate", []byte(body))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_Orchestrate_InvalidRepo(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	body := `{"agents":["go-backend"],"repo":"/path/that/does/not/exist","task":"test"}`
	w := serveRequest(s, "POST", "/api/orchestrate", []byte(body))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestResolveOrchestrateRepo_CurrentDirectory(t *testing.T) {
	t.Parallel()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	got, err := resolveOrchestrateRepo(".", "")
	if err != nil {
		t.Fatalf("resolveOrchestrateRepo() error = %v", err)
	}
	if got != wd {
		t.Fatalf("repo = %q, want %q", got, wd)
	}
}

func TestResolveOrchestrateRepo_RejectsLocalPath(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	if _, err := resolveOrchestrateRepo(repo, ""); err == nil {
		t.Fatal("resolveOrchestrateRepo() error = nil, want local path rejection")
	}
}

func TestShouldRetryCloneWithoutBranch(t *testing.T) {
	t.Parallel()
	if !shouldRetryCloneWithoutBranch("fatal: Remote branch main not found in upstream origin") {
		t.Fatal("expected retry for missing remote branch")
	}
	if shouldRetryCloneWithoutBranch("fatal: Authentication failed") {
		t.Fatal("did not expect retry for auth failure")
	}
}

func TestGitCloneEnv_AddsGitHubBasicAuth(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	env := gitCloneEnv([]string{"clone", "https://github.com/owner/repo.git", "/tmp/repo"})

	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "GIT_CONFIG_KEY_0=http.https://github.com/.extraheader") {
		t.Fatal("missing git extraheader config key")
	}
	if !strings.Contains(joined, "GIT_CONFIG_VALUE_0=AUTHORIZATION: basic ") {
		t.Fatal("missing basic auth extraheader")
	}
	for _, item := range env {
		if strings.HasPrefix(item, "GIT_CONFIG_VALUE_0=") && strings.Contains(item, "secret-token") {
			t.Fatal("git extraheader must not expose the raw token")
		}
	}
}

func TestGitCloneEnv_SkipsNonGitHubRemote(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "secret-token")
	env := gitCloneEnv([]string{"clone", "https://example.com/owner/repo.git", "/tmp/repo"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "GIT_CONFIG_KEY_0") {
		t.Fatal("did not expect GitHub auth config for non-GitHub remote")
	}
}

func TestNormalizeRemoteRepo(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"kazyamaz200/agentos":                        "https://github.com/kazyamaz200/agentos.git",
		"https://github.com/owner/repo.git":          "https://github.com/owner/repo.git",
		"https://github.com/owner/repo":              "https://github.com/owner/repo.git",
		"https://github.com/owner/repo.git?ref=main": "",
		"https://example.com/owner/repo.git":         "",
		"git@github.com:owner/repo.git":              "",
		"file:///tmp/repo.git":                       "",
		"/workspace/scenario-repo":                   "",
		"relative-repo":                              "",
		"owner/repo;touch-x":                         "",
	}
	for input, want := range tests {
		got, ok := normalizeRemoteRepo(input)
		if want == "" {
			if ok {
				t.Fatalf("normalizeRemoteRepo(%q) = %q, want local", input, got)
			}
			continue
		}
		if !ok || got != want {
			t.Fatalf("normalizeRemoteRepo(%q) = %q, %v, want %q, true", input, got, ok, want)
		}
	}
}

func TestValidateGitRef(t *testing.T) {
	t.Parallel()
	valid := []string{"main", "release/v1.0", "feature_1.2-rc"}
	for _, ref := range valid {
		if err := validateGitRef(ref); err != nil {
			t.Fatalf("validateGitRef(%q) error = %v", ref, err)
		}
	}
	invalid := []string{"", "../main", "main..next", "main@{1}", "main/", "-bad"}
	for _, ref := range invalid[1:] {
		if err := validateGitRef(ref); err == nil {
			t.Fatalf("validateGitRef(%q) error = nil, want error", ref)
		}
	}
}

func TestServer_Orchestrate_InvalidJSON(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "POST", "/api/orchestrate", []byte("{invalid}"))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_Orchestrate_EmptyBody(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "POST", "/api/orchestrate", []byte(""))
	assertStatus(t, w.Code, http.StatusBadRequest)
}

func TestServer_Orchestrates_EmptyList(t *testing.T) {
	t.Setenv("AGENTOS_HOME", shortTestDir(t))
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/orchestrates", nil)
	assertStatus(t, w.Code, http.StatusOK)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("body = %s, want []", w.Body.String())
	}
}

func TestOrchestrationRecordStore_RoundTrip(t *testing.T) {
	t.Setenv("AGENTOS_HOME", shortTestDir(t))
	now := time.Now().UTC().Truncate(time.Second)
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "test",
		Agents:     []string{"go-backend"},
		Strategy:   "parallel",
		Status:     "completed",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := saveOrchestrationRecord(record); err != nil {
		t.Fatalf("saveOrchestrationRecord() error = %v", err)
	}
	got, err := readOrchestrationRecord(record.ID)
	if err != nil {
		t.Fatalf("readOrchestrationRecord() error = %v", err)
	}
	if got.ID != record.ID || got.Repo != record.Repo || got.Status != record.Status {
		t.Fatalf("record = %+v, want %+v", got, record)
	}
	records, err := listOrchestrationRecords()
	if err != nil {
		t.Fatalf("listOrchestrationRecords() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("records = %+v, want one %s", records, record.ID)
	}
}

func TestPrepareOrchestrationGitHub_Defaults(t *testing.T) {
	req := orchestrateRequest{
		Repo:       "https://github.com/owner/repo.git",
		BaseBranch: "main",
		Task:       "Implement feature",
		GitHub: &orchestrateGitHubRequest{
			CreateIssue:       true,
			CreatePullRequest: true,
		},
	}
	got, err := prepareOrchestrationGitHub("run-0123456789abcdef", &req)
	if err != nil {
		t.Fatalf("prepareOrchestrationGitHub() error = %v", err)
	}
	if got.Repo != "owner/repo" {
		t.Fatalf("Repo = %q, want owner/repo", got.Repo)
	}
	if got.BranchName != "agentos/run-0123456789abcdef" {
		t.Fatalf("BranchName = %q", got.BranchName)
	}
	if got.IssueTitle != "Implement feature" || got.PRTitle != "Implement feature" {
		t.Fatalf("titles = %q/%q", got.IssueTitle, got.PRTitle)
	}
	if got.PRBase != "main" || !got.CreateIssue || !got.CreatePullRequest {
		t.Fatalf("github state = %+v", got)
	}
}

func TestPrepareOrchestrationGitHub_RejectsNonGitHubRepo(t *testing.T) {
	req := orchestrateRequest{
		Repo: "https://example.com/owner/repo.git",
		Task: "test",
		GitHub: &orchestrateGitHubRequest{
			CreateIssue: true,
		},
	}
	_, err := prepareOrchestrationGitHub("run-0123456789abcdef", &req)
	if err == nil {
		t.Fatal("prepareOrchestrationGitHub() error = nil, want error")
	}
}

func TestOrchestrationRecordStore_PreservesGitHubArtifacts(t *testing.T) {
	t.Setenv("AGENTOS_HOME", shortTestDir(t))
	now := time.Now().UTC().Truncate(time.Second)
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "test",
		Agents:     []string{"go-backend"},
		Strategy:   "parallel",
		Status:     "completed",
		GitHub: &orchestrationGitHubState{
			Repo:              "owner/repo",
			BranchName:        "agentos/run-0123456789abcdef",
			IssueURL:          "https://github.com/owner/repo/issues/1",
			IssueNumber:       1,
			PullRequestURL:    "https://github.com/owner/repo/pull/2",
			PullRequestNumber: 2,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := saveOrchestrationRecord(record); err != nil {
		t.Fatalf("saveOrchestrationRecord() error = %v", err)
	}
	got, err := readOrchestrationRecord(record.ID)
	if err != nil {
		t.Fatalf("readOrchestrationRecord() error = %v", err)
	}
	if got.GitHub == nil || got.GitHub.IssueNumber != 1 || got.GitHub.PullRequestNumber != 2 {
		t.Fatalf("GitHub = %+v", got.GitHub)
	}
}

func TestReadOrchestrationRecord_RejectsInvalidID(t *testing.T) {
	t.Setenv("AGENTOS_HOME", shortTestDir(t))
	invalid := []string{"", ".", "../run-0123456789abcdef", "run-test", "run-0123456789abcdeg"}
	for _, id := range invalid {
		if _, err := readOrchestrationRecord(id); err == nil {
			t.Fatalf("readOrchestrationRecord(%q) error = nil, want error", id)
		}
	}
}

// --- Static Files ---

func TestServer_ServesIndexHTML(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/", nil)
	assertStatus(t, w.Code, http.StatusOK)
	body := w.Body.String()
	if !strings.Contains(body, "AgentOS") {
		t.Error("index.html does not contain 'AgentOS'")
	}
	if !strings.Contains(body, "Orchestrates") {
		t.Error("index.html does not contain 'Orchestrates'")
	}
}

func TestServer_IndexHTML_HasPrimaryNavLinks(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/", nil)
	body := w.Body.String()
	links := []string{"Orchestrates", "Agents"}
	for _, link := range links {
		if !strings.Contains(body, link) {
			t.Errorf("index.html missing nav link: %s", link)
		}
	}
	if strings.Contains(body, `data-page="dashboard"`) || strings.Contains(body, `data-page="github"`) {
		t.Error("index.html should not expose dashboard or github as top-level pages")
	}
}

// --- CORS ---

func TestServer_CORS_Headers(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/health", nil)
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS Allow-Origin header missing")
	}
}

func TestServer_CORS_Preflight(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/api/health", http.NoBody)
	s.server.Handler.ServeHTTP(w, req)
	assertStatus(t, w.Code, http.StatusOK)
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("CORS Allow-Methods header missing")
	}
}

// --- Helpers ---

func TestSplitRepo_Valid(t *testing.T) {
	t.Parallel()
	parts := splitRepo("owner/repo")
	if len(parts) != 2 || parts[0] != "owner" || parts[1] != "repo" {
		t.Errorf("splitRepo = %v, want [owner repo]", parts)
	}
}

func TestSplitRepo_Invalid(t *testing.T) {
	t.Parallel()
	parts := splitRepo("invalid")
	if parts != nil {
		t.Errorf("splitRepo = %v, want nil", parts)
	}
}

func TestSplitRepo_Empty(t *testing.T) {
	t.Parallel()
	parts := splitRepo("")
	if parts != nil {
		t.Errorf("splitRepo = %v, want nil", parts)
	}
}

func TestSplitRepo_MultiSlash(t *testing.T) {
	t.Parallel()
	parts := splitRepo("a/b/c")
	if len(parts) != 2 || parts[0] != "a" || parts[1] != "b/c" {
		t.Errorf("splitRepo = %v, want [a b/c]", parts)
	}
}

func shortTestDir(t *testing.T) string {
	t.Helper()
	parent := ""
	if runtime.GOOS == "windows" {
		parent = filepath.VolumeName(os.TempDir()) + string(os.PathSeparator)
	}
	dir, err := os.MkdirTemp(parent, "ao-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func TestGenerateID_NotEmpty(t *testing.T) {
	t.Parallel()
	id := generateID()
	if id == "" {
		t.Error("generateID returned empty string")
	}
	if !strings.HasPrefix(id, "run-") {
		t.Errorf("generateID = %q, want run- prefix", id)
	}
}

func TestGenerateID_Unique(t *testing.T) {
	t.Parallel()
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateID()
		if ids[id] {
			t.Errorf("duplicate ID generated: %s", id)
		}
		ids[id] = true
	}
}

// --- Content-Type ---

func TestServer_JSONEndpoints(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/agents", ""},
		{"GET", "/api/runs", ""},
		{"POST", "/api/runs", `{"agent":"go-backend","task":"test"}`},
		{"GET", "/api/search?q=test", ""},
	}
	for _, ep := range endpoints {
		var bodyBytes []byte
		if ep.body != "" {
			bodyBytes = []byte(ep.body)
		}
		w := serveRequest(s, ep.method, ep.path, bodyBytes)
		if w.Code != http.StatusOK {
			continue
		}
		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("%s %s: Content-Type = %q, want application/json", ep.method, ep.path, ct)
		}
	}
}

func TestServer_AuthDisabledByDefault(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/auth/session", nil)
	assertStatus(t, w.Code, http.StatusOK)
	if !strings.Contains(w.Body.String(), `"authRequired":false`) {
		t.Fatalf("session = %s, want authRequired false", w.Body.String())
	}
}

func TestServer_AuthRequiredProtectsWorkAPIs(t *testing.T) {
	t.Setenv("AGENTOS_AUTH_REQUIRED", "true")
	t.Setenv("AGENTOS_SESSION_SECRET", "test-secret")
	s := NewServer(0)

	protected := []string{
		"/api/runs",
		"/api/search?q=test",
		"/api/github/issues?repo=owner/repo",
		"/api/orchestrates",
		"/api/audit",
	}
	for _, path := range protected {
		w := serveRequest(s, "GET", path, nil)
		assertStatus(t, w.Code, http.StatusUnauthorized)
	}
}

func TestAuthConfig_UserCanAutomate(t *testing.T) {
	t.Parallel()

	cfg := authConfig{Required: true, AdminUsers: parseAdminUsers("alice, Bob")}
	if !cfg.userCanAutomate(&authUser{Login: "alice"}) {
		t.Fatal("alice should be allowed")
	}
	if !cfg.userCanAutomate(&authUser{Login: "bob"}) {
		t.Fatal("bob should be allowed case-insensitively")
	}
	if cfg.userCanAutomate(&authUser{Login: "mallory"}) {
		t.Fatal("mallory should be denied")
	}
}

func TestAuditEventsPersistAndRedact(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGENTOS_HOME", home)

	err := appendAuditEvent(&auditEvent{
		Actor:   "alice",
		Action:  "github.issue.create",
		Target:  "owner/repo",
		Outcome: auditOutcomeFailure,
		Message: "Authorization: Bearer ghp_123456789012345678901234567890123456",
	})
	if err != nil {
		t.Fatal(err)
	}

	events, err := listAuditEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if strings.Contains(events[0].Message, "ghp_123456789012345678901234567890123456") {
		t.Fatalf("audit event leaked token: %+v", events[0])
	}
	if !strings.Contains(events[0].Message, "[REDACTED]") {
		t.Fatalf("audit event was not redacted: %+v", events[0])
	}
}

func TestServer_OrchestrateDeniedForNonAdminAudits(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGENTOS_HOME", home)
	t.Setenv("AGENTOS_AUTH_REQUIRED", "true")
	t.Setenv("AGENTOS_SESSION_SECRET", "test-secret")
	t.Setenv("AGENTOS_ADMIN_USERS", "admin")
	s := NewServer(0)

	body := []byte(`{"agents":["go-backend"],"repo":".","task":"test task"}`)
	w := serveRequestAs(s, "POST", "/api/orchestrate", body, &authUser{
		Login:     "mallory",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	assertStatus(t, w.Code, http.StatusForbidden)

	events, err := listAuditEvents(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Actor != "mallory" || events[0].Action != "orchestrate.create" || events[0].Outcome != auditOutcomeDenied {
		t.Fatalf("unexpected audit event: %+v", events[0])
	}
}

func TestServer_AuditEndpointReturnsEventsForAdmin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGENTOS_HOME", home)
	t.Setenv("AGENTOS_AUTH_REQUIRED", "true")
	t.Setenv("AGENTOS_SESSION_SECRET", "test-secret")
	t.Setenv("AGENTOS_ADMIN_USERS", "admin")
	s := NewServer(0)

	if err := appendAuditEvent(&auditEvent{Actor: "admin", Action: "orchestrate.create", Target: "orchestration", Outcome: auditOutcomeAllowed}); err != nil {
		t.Fatal(err)
	}
	w := serveRequestAs(s, "GET", "/api/audit", nil, &authUser{
		Login:     "admin",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	assertStatus(t, w.Code, http.StatusOK)
	var events []auditEvent
	if err := json.Unmarshal(w.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) < 1 {
		t.Fatal("expected audit events")
	}
}

func TestServer_LLMSettingsDoesNotExposeSecret(t *testing.T) {
	t.Setenv("LITELLM_BASE_URL", "http://litellm.test")
	t.Setenv("LITELLM_API_KEY", "secret-key")
	t.Setenv("AGENTOS_MODEL_CODER", "coder-test")
	s := NewServer(0)

	w := serveRequest(s, "GET", "/api/settings/llm", nil)
	assertStatus(t, w.Code, http.StatusOK)
	body := w.Body.String()
	if strings.Contains(body, "secret-key") || strings.Contains(body, "LITELLM_API_KEY") {
		t.Fatalf("LLM settings leaked secret metadata: %s", body)
	}
	if !strings.Contains(body, `"keyConfigured":true`) || !strings.Contains(body, `"model":"coder-test"`) {
		t.Fatalf("LLM settings = %s, want configured coder-test preset", body)
	}
}

func TestParseLLMPresets(t *testing.T) {
	raw := `[{"id":"staips","name":"STAIPS","provider":"litellm","baseUrl":"http://litellm:4000/","model":"coder","apiKeyEnv":"LITELLM_API_KEY"}]`
	presets := parseLLMPresets(raw)
	if len(presets) != 1 {
		t.Fatalf("len(presets) = %d, want 1", len(presets))
	}
	if presets[0].BaseURL != "http://litellm:4000" || presets[0].APIKeyEnv != "LITELLM_API_KEY" {
		t.Fatalf("preset = %+v", presets[0])
	}
}

func TestApplySubtaskEvent(t *testing.T) {
	record := &orchestrationRecord{
		Plan: &orchestrator.TaskPlan{Subtasks: []orchestrator.Subtask{{
			ID:          "step-1",
			Description: "do work",
			AgentName:   "go-backend",
		}}},
	}
	started := time.Now().UTC()
	applySubtaskEvent(record, &orchestrator.SubtaskEvent{
		Type:    orchestrator.SubtaskStarted,
		Subtask: record.Plan.Subtasks[0],
		Started: started,
	})
	if len(record.Subtasks) != 1 || record.Subtasks[0].Status != "running" || record.Subtasks[0].StartedAt == nil {
		t.Fatalf("started state = %+v", record.Subtasks)
	}

	finished := started.Add(time.Second)
	applySubtaskEvent(record, &orchestrator.SubtaskEvent{
		Type:     orchestrator.SubtaskCompleted,
		Subtask:  record.Plan.Subtasks[0],
		Finished: finished,
		Result:   &orchestrator.SubtaskResult{SubtaskID: "step-1", Success: true},
	})
	if record.Subtasks[0].Status != "completed" || record.Subtasks[0].FinishedAt == nil || record.Subtasks[0].Result == nil {
		t.Fatalf("completed state = %+v", record.Subtasks[0])
	}
}
