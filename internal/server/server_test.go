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

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/guideline"
	"github.com/kazyamaz200/agentos/internal/memory"
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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
	assertArrayLen(t, w.Body.Bytes(), 9)
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
			if guidance, ok := a["ArchitectureGuidance"].([]interface{}); !ok || len(guidance) == 0 {
				t.Fatalf("go-backend missing architecture guidance: %+v", a)
			}
			if outputs, ok := a["OutputExpectations"].([]interface{}); !ok || len(outputs) == 0 {
				t.Fatalf("go-backend missing output expectations: %+v", a)
			}
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

func TestServer_RepositoryMemoryLifecycle(t *testing.T) {
	t.Setenv("AGENTOS_HOME", t.TempDir())
	s := NewServer(0)

	createBody := []byte(`{"repo":"owner/repo","baseBranch":"main","type":"validation","content":"Run go test ./...","status":"pending"}`)
	w := serveRequest(s, "POST", "/api/repository-memory", createBody)
	assertStatus(t, w.Code, http.StatusOK)
	var created memory.RepositoryEntry
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Status != memory.RepositoryMemoryPending {
		t.Fatalf("Status = %q, want pending", created.Status)
	}

	w = serveRequest(s, "GET", "/api/repository-memory?repo=owner/repo&baseBranch=main&status=pending", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var listed []memory.RepositoryEntry
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed = %+v, want created memory", listed)
	}

	w = serveRequest(s, "POST", "/api/repository-memory/"+created.ID+"/approve", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var approved memory.RepositoryEntry
	if err := json.Unmarshal(w.Body.Bytes(), &approved); err != nil {
		t.Fatal(err)
	}
	if approved.Status != memory.RepositoryMemoryApproved {
		t.Fatalf("Status = %q, want approved", approved.Status)
	}

	w = serveRequest(s, "PUT", "/api/repository-memory/"+created.ID, []byte(`{"pinned":true}`))
	assertStatus(t, w.Code, http.StatusOK)
	var updated memory.RepositoryEntry
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if !updated.Pinned {
		t.Fatalf("Pinned = false, want true")
	}

	w = serveRequest(s, "DELETE", "/api/repository-memory/"+created.ID, nil)
	assertStatus(t, w.Code, http.StatusNoContent)
}

func TestRepositoryMemory_PlanningContextAndProposals(t *testing.T) {
	t.Setenv("AGENTOS_HOME", t.TempDir())
	store, err := repositoryMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	entry := &memory.RepositoryEntry{
		Repo:    "owner/repo",
		Branch:  "main",
		Type:    "architecture",
		Content: "Use internal/server for Web UI API handlers.",
		Status:  memory.RepositoryMemoryApproved,
	}
	if err := store.Save(ctx, entry); err != nil {
		t.Fatal(err)
	}
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "Use internal/server for Web UI API handlers",
		Agents:     []string{"go-backend", "reviewer"},
		Plan: &orchestrator.TaskPlan{Subtasks: []orchestrator.Subtask{{
			ID:          "step-1",
			AgentName:   "go-backend",
			Description: "implement",
			QualityGate: &orchestrator.QualityGate{ValidationCommands: []string{"go test ./..."}},
		}}},
	}

	used := repositoryMemoryForPlanning(ctx, record)
	if len(used) != 1 || !strings.Contains(taskWithRepositoryMemory(record.Task, used), "Use internal/server") {
		t.Fatalf("used memory = %+v", used)
	}

	proposals := proposeRepositoryMemory(ctx, record, []orchestrator.SubtaskResult{{
		SubtaskID: "step-1",
		Success:   true,
		QualityGate: &orchestrator.QualityGateStatus{Checks: []orchestrator.QualityGateCheckResult{{
			Type:   "command",
			Target: "go test ./...",
			Passed: true,
		}}},
	}})
	if len(proposals) == 0 {
		t.Fatal("expected memory proposals")
	}
	reloaded, err := repositoryMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	pending, err := reloaded.List(ctx, &memory.RepositoryQuery{Repo: "owner/repo", Branch: "main", Status: memory.RepositoryMemoryPending})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) == 0 {
		t.Fatal("expected pending proposals in store")
	}
}

func TestServer_RepositoryGuidelineLifecycle(t *testing.T) {
	t.Setenv("AGENTOS_HOME", t.TempDir())
	s := NewServer(0)

	createBody := []byte(`{"repo":"owner/repo","baseBranch":"main","title":"Server APIs","type":"architecture","content":"Place handlers under internal/server.","required":true}`)
	w := serveRequest(s, "POST", "/api/repository-guidelines", createBody)
	assertStatus(t, w.Code, http.StatusOK)
	var created guideline.RepositoryGuideline
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !created.Required || created.Status != guideline.RepositoryGuidelineActive {
		t.Fatalf("created = %+v, want active required guideline", created)
	}

	w = serveRequest(s, "GET", "/api/repository-guidelines?repo=owner/repo&baseBranch=main&q=internal/server", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var listed []guideline.RepositoryGuideline
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed = %+v, want created guideline", listed)
	}

	w = serveRequest(s, "PUT", "/api/repository-guidelines/"+created.ID, []byte(`{"title":"Server API convention","content":"Keep Web UI handlers in internal/server.","type":"architecture","required":false}`))
	assertStatus(t, w.Code, http.StatusOK)
	var updated guideline.RepositoryGuideline
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Server API convention" || updated.Required {
		t.Fatalf("updated = %+v, want edited advisory guideline", updated)
	}

	w = serveRequest(s, "DELETE", "/api/repository-guidelines/"+created.ID, nil)
	assertStatus(t, w.Code, http.StatusOK)
	var archived guideline.RepositoryGuideline
	if err := json.Unmarshal(w.Body.Bytes(), &archived); err != nil {
		t.Fatal(err)
	}
	if archived.Status != guideline.RepositoryGuidelineArchived {
		t.Fatalf("Status = %q, want archived", archived.Status)
	}
}

func TestRepositoryGuidelines_PlanningContextAndRequiredEnforcement(t *testing.T) {
	t.Setenv("AGENTOS_HOME", t.TempDir())
	store, err := repositoryGuidelineStore()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	required := &guideline.RepositoryGuideline{
		Repo:     "owner/repo",
		Branch:   "main",
		Title:    "Run validation",
		Type:     "validation",
		Content:  "Run go test ./... before reporting success.",
		Required: true,
		Status:   guideline.RepositoryGuidelineActive,
	}
	if err := store.Save(ctx, required); err != nil {
		t.Fatal(err)
	}
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "Add validation for go tests",
	}
	used := repositoryGuidelinesForPlanning(ctx, record, "go-backend")
	if len(used) != 1 || !strings.Contains(taskWithRepositoryGuidelines(record.Task, used), "Run validation") {
		t.Fatalf("used guidelines = %+v", used)
	}
	plan := &orchestrator.TaskPlan{Subtasks: []orchestrator.Subtask{{
		ID:          "step-1",
		AgentName:   "go-backend",
		Description: "implement validation",
	}}}
	applied := applyRepositoryGuidelinesToPlan(plan, used)
	if len(applied) != 1 || !strings.Contains(plan.Subtasks[0].Description, "Run go test ./...") {
		t.Fatalf("applied = %+v description=%q", applied, plan.Subtasks[0].Description)
	}
	if missed := missedRequiredGuidelines(used, applied); len(missed) != 0 {
		t.Fatalf("missed = %+v, want none", missed)
	}
	if missed := missedRequiredGuidelines(used, nil); len(missed) != 1 || missed[0].ID != required.ID {
		t.Fatalf("missed = %+v, want required guideline", missed)
	}
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
		CustomAgents: []agent.Definition{{
			APIVersion: agent.CurrentSchemaVersion,
			Kind:       "Agent",
			Metadata:   agent.DefinitionMetadata{Name: "repo-security", Labels: map[string]string{"role": "security"}},
			Spec: agent.DefinitionSpec{
				LLM:   agent.LLMConfig{Model: "coder"},
				Tools: agent.ToolsConfig{Allow: []string{"read_file", "search"}},
			},
		}},
		Scenario:  &scenarioTemplateSelection{ID: "security-remediation", Name: "Security Remediation", Source: "built-in"},
		Strategy:  "parallel",
		Status:    "completed",
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
	if got.ID != record.ID || got.Repo != record.Repo || got.Status != record.Status {
		t.Fatalf("record = %+v, want %+v", got, record)
	}
	if len(got.CustomAgents) != 1 || got.CustomAgents[0].Metadata.Name != "repo-security" {
		t.Fatalf("custom agents were not preserved: %+v", got.CustomAgents)
	}
	if got.Scenario == nil || got.Scenario.ID != "security-remediation" {
		t.Fatalf("scenario template was not preserved: %+v", got.Scenario)
	}
	records, err := listOrchestrationRecords()
	if err != nil {
		t.Fatalf("listOrchestrationRecords() error = %v", err)
	}
	if len(records) != 1 || records[0].ID != record.ID {
		t.Fatalf("records = %+v, want one %s", records, record.ID)
	}
}

func TestServer_OrchestrateTemplates_ReturnsBuiltIns(t *testing.T) {
	s := NewServer(0)
	w := serveRequest(s, "POST", "/api/orchestrate/templates", []byte(`{"repo":"","baseBranch":"main"}`))
	assertStatus(t, w.Code, http.StatusOK)
	var templates []scenarioTemplate
	if err := json.Unmarshal(w.Body.Bytes(), &templates); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(templates) < 7 {
		t.Fatalf("templates = %d, want at least 7", len(templates))
	}
	if templates[0].ID == "" || templates[0].TaskTemplate == "" || len(templates[0].Agents) == 0 {
		t.Fatalf("template missing required fields: %+v", templates[0])
	}
}

func TestLoadRepositoryScenarioTemplates_LoadsValidTemplates(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".agentos", "scenarios")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs.yaml"), []byte(`id: repo-docs
name: Repository Docs
description: Repository-specific documentation update
agents:
  - docs
  - reviewer
strategy: sequential
createPullRequest: true
taskTemplate: |
  Update {{docTarget}} for {{repo}}.
variables:
  - name: repo
    label: Repository
    required: true
  - name: docTarget
    label: Doc target
    required: true
`), 0o600); err != nil {
		t.Fatal(err)
	}

	templates, err := loadRepositoryScenarioTemplates(repo, agent.DefaultRegistry())
	if err != nil {
		t.Fatalf("loadRepositoryScenarioTemplates() error = %v", err)
	}
	if len(templates) != 1 || templates[0].ID != "repo-docs" || templates[0].Source != "repository" {
		t.Fatalf("templates = %+v", templates)
	}
}

func TestLoadRepositoryScenarioTemplates_RejectsUnknownAgent(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".agentos", "scenarios")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte(`id: bad-template
name: Bad Template
agents:
  - missing-agent
taskTemplate: Do work.
`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := loadRepositoryScenarioTemplates(repo, agent.DefaultRegistry())
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("error = %v, want unknown agent rejection", err)
	}
}

func TestLoadRepositoryAgentDefinitions_LoadsValidDefinitions(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".agentos", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frontend.yaml"), []byte(`apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: frontend-app
  labels:
    role: frontend
spec:
  llm:
    model: coder
  tools:
    allow:
      - read_file
      - write_file
      - search
      - shell
      - git
      - test
  safety:
    denyCommands:
      - rm -rf
      - sudo
  commands:
    test: npm test
  guidance:
    architecture:
      - Follow existing components.
    outputExpectations:
      - Build passes.
`), 0o600); err != nil {
		t.Fatal(err)
	}

	defs, err := loadRepositoryAgentDefinitions(repo, agent.DefaultRegistry())
	if err != nil {
		t.Fatalf("loadRepositoryAgentDefinitions() error = %v", err)
	}
	if len(defs) != 1 || defs[0].Metadata.Name != "frontend-app" {
		t.Fatalf("defs = %+v", defs)
	}
}

func TestValidateCustomAgentDefinitions_RejectsBuiltInOverride(t *testing.T) {
	def := agent.Definition{
		APIVersion: agent.CurrentSchemaVersion,
		Kind:       "Agent",
		Metadata:   agent.DefinitionMetadata{Name: "go-backend"},
		Spec: agent.DefinitionSpec{
			LLM:   agent.LLMConfig{Model: "coder"},
			Tools: agent.ToolsConfig{Allow: []string{"read_file"}},
		},
	}
	_, err := validateCustomAgentDefinitions([]agent.Definition{def}, agent.DefaultRegistry())
	if err == nil || !strings.Contains(err.Error(), "cannot override") {
		t.Fatalf("error = %v, want override rejection", err)
	}
}

func TestValidateCustomAgentDefinitions_RejectsUnsafeCommands(t *testing.T) {
	def := agent.Definition{
		APIVersion: agent.CurrentSchemaVersion,
		Kind:       "Agent",
		Metadata:   agent.DefinitionMetadata{Name: "repo-security"},
		Spec: agent.DefinitionSpec{
			LLM:      agent.LLMConfig{Model: "coder"},
			Tools:    agent.ToolsConfig{Allow: []string{"read_file", "shell"}},
			Safety:   agent.SafetyConfig{DenyCommands: []string{"sudo"}},
			Commands: agent.CommandsConfig{Test: "sudo go test ./..."},
		},
	}
	_, err := validateCustomAgentDefinitions([]agent.Definition{def}, agent.DefaultRegistry())
	if err == nil || !strings.Contains(err.Error(), "unsafe command") {
		t.Fatalf("error = %v, want unsafe command rejection", err)
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
	if got.IssueTemplate != "default" || got.PRTemplate != "default" {
		t.Fatalf("templates = %q/%q, want default/default", got.IssueTemplate, got.PRTemplate)
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
		ID:             "run-0123456789abcdef",
		Repo:           "owner/repo",
		BaseBranch:     "main",
		Task:           "test",
		Agents:         []string{"go-backend"},
		Strategy:       "parallel",
		OutputLanguage: "ja",
		Status:         "completed",
		GitHub: &orchestrationGitHubState{
			Repo:                  "owner/repo",
			BranchName:            "agentos/run-0123456789abcdef",
			IssueTemplate:         "repository",
			IssueURL:              "https://github.com/owner/repo/issues/1",
			IssueNumber:           1,
			PRTemplate:            "repository",
			PullRequestURL:        "https://github.com/owner/repo/pull/2",
			PullRequestNumber:     2,
			SourceIssueNumber:     1,
			SourceStartCommentURL: "https://github.com/owner/repo/issues/1#issuecomment-10",
			SourceFinalCommentURL: "https://github.com/owner/repo/issues/1#issuecomment-11",
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
	if got.OutputLanguage != "ja" || got.GitHub.IssueTemplate != "repository" || got.GitHub.PRTemplate != "repository" {
		t.Fatalf("language/templates = %q/%q/%q", got.OutputLanguage, got.GitHub.IssueTemplate, got.GitHub.PRTemplate)
	}
	if got.GitHub.SourceStartCommentURL == "" || got.GitHub.SourceFinalCommentURL == "" {
		t.Fatalf("source comment URLs were not preserved: %+v", got.GitHub)
	}
}

func TestArtifactTemplates_DefaultEnglishAndJapanese(t *testing.T) {
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "Implement feature",
		Agents:     []string{"go-backend", "reviewer"},
		Strategy:   "sequential",
		GitHub: &orchestrationGitHubState{
			Repo:          "owner/repo",
			BranchName:    "agentos/run-0123456789abcdef",
			IssueTemplate: "default",
			PRTemplate:    "default",
			PRBase:        "main",
		},
		Summary: "Done",
	}
	issue := orchestrationIssueBody(record)
	if !strings.Contains(issue, "## Task") || !strings.Contains(issue, "Implement feature") {
		t.Fatalf("english issue body = %q", issue)
	}
	pr := orchestrationPRBody(record)
	if !strings.Contains(pr, "## Summary") || !strings.Contains(pr, "Done") {
		t.Fatalf("english PR body = %q", pr)
	}

	record.OutputLanguage = "ja"
	issue = orchestrationIssueBody(record)
	if !strings.Contains(issue, "## タスク") || !strings.Contains(issue, "AgentOS Orchestrate により作成されました") {
		t.Fatalf("japanese issue body = %q", issue)
	}
	pr = orchestrationPRBody(record)
	if !strings.Contains(pr, "## 概要") || !strings.Contains(pr, "Done") {
		t.Fatalf("japanese PR body = %q", pr)
	}
}

func TestArtifactTemplates_RepositoryConfigFallback(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".agentos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".agentos", "config.yaml"), []byte(`outputLanguage: ja
templates:
  issue:
    body: |
      Repo issue for {{.RunID}}
      Task={{.Task}}
  pullRequest:
    body: |
      Repo PR for {{.RunID}}
      Summary={{.Summary}}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	req := &orchestrateRequest{Repo: "owner/repo", Task: "Task text", GitHub: &orchestrateGitHubRequest{CreateIssue: true, CreatePullRequest: true}}
	cfg := loadArtifactConfig(repo)
	applyArtifactConfig(req, cfg)
	if req.OutputLanguage != "ja" || req.GitHub.IssueTemplate != "repository" || req.GitHub.PRTemplate != "repository" {
		t.Fatalf("request after config = %+v github=%+v", req, req.GitHub)
	}

	record := &orchestrationRecord{
		ID:             "run-0123456789abcdef",
		RepoPath:       repo,
		Task:           "Task text",
		OutputLanguage: req.OutputLanguage,
		GitHub: &orchestrationGitHubState{
			Repo:          "owner/repo",
			IssueTemplate: req.GitHub.IssueTemplate,
			PRTemplate:    req.GitHub.PRTemplate,
		},
		Summary: "Summary text",
	}
	if body := orchestrationIssueBody(record); !strings.Contains(body, "Repo issue for run-0123456789abcdef") || !strings.Contains(body, "Task=Task text") {
		t.Fatalf("repository issue body = %q", body)
	}
	if body := orchestrationPRBody(record); !strings.Contains(body, "Repo PR for run-0123456789abcdef") || !strings.Contains(body, "Summary=Summary text") {
		t.Fatalf("repository PR body = %q", body)
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

func TestRecommendOrchestration_ClassifiesCommonTasks(t *testing.T) {
	reg := agent.DefaultRegistry()
	tests := []struct {
		name   string
		task   string
		signal []string
		want   string
	}{
		{"frontend", "Improve React responsive UI with Tailwind CSS", nil, "frontend"},
		{"ops", "Fix Helm deployment for Kubernetes ingress", nil, "ops"},
		{"reporting", "Investigate incident and write a report", nil, "reporting"},
		{"ci", "GitHub Actions CI check failed on lint", nil, "ci-fix"},
		{"backend", "Add API endpoint to Go server", nil, "backend"},
		{"docs", "Update README documentation", nil, "docs"},
		{"security", "Fix CVE vulnerability and authz permission issue", nil, "security"},
		{"release", "Prepare release notes and changelog for v1.2.0", nil, "release"},
		{"dependency", "Bump dependencies", []string{"dependency"}, "dependency"},
		{"qa", "Add smoke test and manual verification notes", nil, "qa"},
		{"bugfix", "Fix regression causing panic", nil, "bugfix"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recommendOrchestration(tt.task, tt.signal, reg)
			if got.Preset != tt.want {
				t.Fatalf("Preset = %q, want %q; recommendation=%+v", got.Preset, tt.want, got)
			}
			if len(got.Agents) == 0 || got.Rationale == "" || got.Confidence <= 0 {
				t.Fatalf("incomplete recommendation: %+v", got)
			}
		})
	}
}

func TestRecommendOrchestration_RoutesFrontendAndOpsToSpecialists(t *testing.T) {
	reg := agent.DefaultRegistry()
	tests := []struct {
		name       string
		task       string
		signals    []string
		wantPreset string
		wantAgents []string
	}{
		{"frontend", "Update responsive UI", []string{"frontend"}, "frontend", []string{"frontend", "qa", "reviewer"}},
		{"docker", "Update Dockerfile healthcheck", []string{"ops"}, "ops", []string{"release-manager", "security", "qa", "reviewer"}},
		{"helm", "Fix Helm chart values", nil, "ops", []string{"release-manager", "security", "qa", "reviewer"}},
		{"kubernetes", "Fix Kubernetes ingress deployment", nil, "ops", []string{"release-manager", "security", "qa", "reviewer"}},
		{"backend", "Add Go API handler", []string{"backend"}, "backend", []string{"go-backend", "reviewer"}},
		{"docs", "Update README guide", []string{"docs"}, "docs", []string{"docs", "reviewer"}},
		{"security", "Fix CodeQL security finding", []string{"security"}, "security", []string{"security", "reviewer"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recommendOrchestration(tt.task, tt.signals, reg)
			if got.Preset != tt.wantPreset {
				t.Fatalf("Preset = %q, want %q; recommendation=%+v", got.Preset, tt.wantPreset, got)
			}
			for _, want := range tt.wantAgents {
				if !containsString(got.Agents, want) {
					t.Fatalf("Agents = %+v, want %q", got.Agents, want)
				}
			}
		})
	}
}

func TestRecommendRepoSignals_DetectsFrontendAndOpsFiles(t *testing.T) {
	repo := t.TempDir()
	for _, path := range []string{"package.json", "next.config.js", "svelte.config.js", "index.html", "Dockerfile", filepath.Join("charts", "Chart.yaml"), filepath.Join(".github", "workflows", "ci.yml")} {
		full := filepath.Join(repo, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}

	signals := recommendRepoSignals(".", "main")
	for _, want := range []string{"ci", "frontend", "ops"} {
		if !containsString(signals, want) {
			t.Fatalf("signals = %+v, want %q", signals, want)
		}
	}
}

func TestServer_OrchestrateRecommendEndpoint(t *testing.T) {
	s := NewServer(0)
	w := serveRequest(s, "POST", "/api/orchestrate/recommend", []byte(`{"repo":".","baseBranch":"main","task":"Fix GitHub Actions CI failure"}`))
	assertStatus(t, w.Code, http.StatusOK)
	var got orchestrationRecommendation
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Preset != "ci-fix" || len(got.Agents) == 0 || !got.CreatePullRequest {
		t.Fatalf("recommendation = %+v", got)
	}
}

func TestRecommendOrchestration_UsesSpecializedBuiltInAgents(t *testing.T) {
	t.Parallel()

	reg := agent.DefaultRegistry()
	tests := []struct {
		task      string
		want      string
		wantAgent string
	}{
		{"Fix CVE and authz permission issue", "security", "security"},
		{"Prepare release notes and rollback checklist", "release", "release-manager"},
		{"Bump Go module dependencies", "dependency", "dependency-updater"},
		{"Add smoke test and manual verification notes", "qa", "qa"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := recommendOrchestration(tt.task, nil, reg)
			if got.Preset != tt.want {
				t.Fatalf("Preset = %q, want %q; recommendation=%+v", got.Preset, tt.want, got)
			}
			found := false
			for _, name := range got.Agents {
				if name == tt.wantAgent {
					found = true
				}
			}
			if !found {
				t.Fatalf("Agents = %+v, want %q", got.Agents, tt.wantAgent)
			}
		})
	}
}

func TestIssueTriggerControls_LabelAndSlashCommand(t *testing.T) {
	body := `Please handle this.

/agentos run agents=docs,reviewer strategy=parallel create_pr=false close_policy=after_human_approval approval=true`
	got := issueTriggerControls([]string{"agentos:create-pr"}, body)
	if got.Strategy != "parallel" {
		t.Fatalf("Strategy = %q, want parallel", got.Strategy)
	}
	if strings.Join(got.Agents, ",") != "docs,reviewer" {
		t.Fatalf("Agents = %+v, want docs/reviewer", got.Agents)
	}
	if got.CreatePullRequest == nil || *got.CreatePullRequest {
		t.Fatalf("CreatePullRequest = %v, want false", got.CreatePullRequest)
	}
	if got.ClosePolicy != "after_human_approval" || got.RequireApproval == nil || !*got.RequireApproval {
		t.Fatalf("approval controls = %+v, want after_human_approval/true", got)
	}
}

func TestOrchestrationRequestFromIssue_UsesRecommendationAndSource(t *testing.T) {
	req, source, err := orchestrationRequestFromIssue(&orchestrateFromIssueRequest{
		Repo:        "kazyamaz200/agentos",
		BaseBranch:  "main",
		IssueNumber: 203,
		IssueTitle:  "Fix GitHub Actions workflow check failed",
		IssueBody:   "CI is failing on lint.",
		IssueURL:    "https://github.com/kazyamaz200/agentos/issues/203",
		Labels:      []string{"agentos:run", "agentos:create-pr"},
	}, agent.DefaultRegistry())
	if err != nil {
		t.Fatalf("orchestrationRequestFromIssue() error = %v", err)
	}
	if source.Number != 203 || source.URL == "" {
		t.Fatalf("source = %+v", source)
	}
	if req.Repo != "kazyamaz200/agentos" || req.BaseBranch != "main" {
		t.Fatalf("repo/base = %q/%q", req.Repo, req.BaseBranch)
	}
	if !strings.Contains(req.Task, "GitHub Issue #203") || !strings.Contains(req.Task, "CI is failing") {
		t.Fatalf("Task = %q", req.Task)
	}
	if strings.Join(req.Agents, ",") != "ci-fixer,reviewer" {
		t.Fatalf("Agents = %+v, want ci-fixer/reviewer", req.Agents)
	}
	if req.GitHub == nil || !req.GitHub.CreatePullRequest || req.GitHub.BranchName != "agentos/issue-203" {
		t.Fatalf("GitHub = %+v", req.GitHub)
	}
	if source.ClosePolicy != "on_pr_merge" {
		t.Fatalf("ClosePolicy = %q, want on_pr_merge", source.ClosePolicy)
	}
}

func TestFindDuplicateIssueOrchestration_ActiveIssueAndTrigger(t *testing.T) {
	t.Setenv("AGENTOS_HOME", shortTestDir(t))
	now := time.Now().UTC()
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "kazyamaz200/agentos",
		BaseBranch: "main",
		Task:       "issue task",
		Agents:     []string{"go-backend"},
		Strategy:   "sequential",
		Status:     "running",
		GitHub: &orchestrationGitHubState{
			Repo:              "kazyamaz200/agentos",
			SourceIssueNumber: 203,
			SourceTriggerID:   "delivery-1",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := saveOrchestrationRecord(record); err != nil {
		t.Fatal(err)
	}
	if got, ok := findDuplicateIssueOrchestration("kazyamaz200/agentos", &orchestrationSourceIssue{Number: 203}); !ok || got.ID != record.ID {
		t.Fatalf("duplicate by issue = %+v/%v, want %s", got, ok, record.ID)
	}
	if got, ok := findDuplicateIssueOrchestration("kazyamaz200/agentos", &orchestrationSourceIssue{Number: 999, TriggerID: "delivery-1"}); !ok || got.ID != record.ID {
		t.Fatalf("duplicate by trigger = %+v/%v, want %s", got, ok, record.ID)
	}
}

func TestSourceIssueCommentBodies(t *testing.T) {
	t.Setenv("AGENTOS_PUBLIC_URL", "https://agentos.example.com")
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "Fix CI",
		Agents:     []string{"ci-fixer", "reviewer"},
		Strategy:   "parallel",
		Status:     "completed",
		Summary:    "CI fixed.",
		GitHub: &orchestrationGitHubState{
			Repo:           "owner/repo",
			PullRequestURL: "https://github.com/owner/repo/pull/2",
		},
	}
	start := sourceIssueStartCommentBody(record)
	if !strings.Contains(start, "AgentOS orchestration started") ||
		!strings.Contains(start, "https://agentos.example.com/#orchestrates/run-0123456789abcdef") ||
		!strings.Contains(start, "ci-fixer, reviewer") {
		t.Fatalf("start comment = %q", start)
	}
	final := sourceIssueFinalCommentBody(record)
	if !strings.Contains(final, "AgentOS orchestration finished") ||
		!strings.Contains(final, "Status: completed") ||
		!strings.Contains(final, "https://github.com/owner/repo/pull/2") ||
		!strings.Contains(final, "CI fixed.") {
		t.Fatalf("final comment = %q", final)
	}
}

func TestSourceIssueClosePolicyAllows(t *testing.T) {
	record := &orchestrationRecord{
		Status: "completed",
		Results: []orchestrator.SubtaskResult{{
			Success: true,
			QualityGate: &orchestrator.QualityGateStatus{
				Passed: true,
			},
		}},
		GitHub: &orchestrationGitHubState{ClosePolicy: "on_quality_gate_pass"},
	}
	if !sourceIssueClosePolicyAllows(record) {
		t.Fatal("on_quality_gate_pass should allow close when results and gates passed")
	}
	record.Results[0].QualityGate.Passed = false
	if sourceIssueClosePolicyAllows(record) {
		t.Fatal("on_quality_gate_pass should not allow close when quality gate failed")
	}
	record.Results[0].QualityGate.Passed = true
	record.GitHub.ClosePolicy = "after_human_approval"
	record.GitHub.ApprovalStatus = "pending"
	if sourceIssueClosePolicyAllows(record) {
		t.Fatal("pending approval should not allow close")
	}
	record.GitHub.ApprovalStatus = "approved"
	if !sourceIssueClosePolicyAllows(record) {
		t.Fatal("approved human approval should allow close")
	}
	record.GitHub.ClosePolicy = "never"
	if sourceIssueClosePolicyAllows(record) {
		t.Fatal("never should not allow close")
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
	if len(record.Events) != 0 {
		t.Fatalf("applySubtaskEvent should not append timeline directly: %+v", record.Events)
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

func TestAppendTimelineForSubtaskEvent(t *testing.T) {
	record := &orchestrationRecord{}
	appendTimelineForSubtaskEvent(record, &orchestrator.SubtaskEvent{
		Type:    orchestrator.SubtaskStarted,
		Subtask: orchestrator.Subtask{ID: "step-1", AgentName: "go-backend"},
	})
	appendTimelineForSubtaskEvent(record, &orchestrator.SubtaskEvent{
		Type:    orchestrator.SubtaskCompleted,
		Subtask: orchestrator.Subtask{ID: "step-1", AgentName: "go-backend"},
		Result:  &orchestrator.SubtaskResult{SubtaskID: "step-1", Success: false, Error: "boom"},
	})
	if len(record.Events) != 2 {
		t.Fatalf("events = %+v, want 2", record.Events)
	}
	if record.Events[0].Type != "subtask.started" || record.Events[1].Type != "subtask.completed" {
		t.Fatalf("event types = %+v", record.Events)
	}
	if !strings.Contains(record.Events[1].Message, "boom") {
		t.Fatalf("completion message = %q, want error detail", record.Events[1].Message)
	}
}

func TestServer_CancelOrchestration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGENTOS_HOME", home)
	t.Setenv("AGENTOS_AUTH_REQUIRED", "true")
	t.Setenv("AGENTOS_SESSION_SECRET", "test-secret")
	t.Setenv("AGENTOS_ADMIN_USERS", "admin")
	s := NewServer(0)

	record := &orchestrationRecord{
		ID:        "run-1234567890abcdef",
		Status:    "running",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := saveOrchestrationRecord(record); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.registerActiveOrchestration(record.ID, cancel)

	w := serveRequestAs(s, "POST", "/api/orchestrates/"+record.ID+"/cancel", nil, &authUser{
		Login:     "admin",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	})
	assertStatus(t, w.Code, http.StatusOK)
	if ctx.Err() != context.Canceled {
		t.Fatalf("context err = %v, want canceled", ctx.Err())
	}
	updated, err := readOrchestrationRecord(record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "canceled" || len(updated.Events) != 2 || updated.Events[0].Type != "cancel.requested" || updated.Events[1].Type != "canceled" {
		t.Fatalf("updated record = %+v", updated)
	}
}

func TestServer_StopCanceledOrchestrationPreservesTerminalRecord(t *testing.T) {
	home := t.TempDir()
	t.Setenv("AGENTOS_HOME", home)
	s := NewServer(0)

	id := "run-1234567890abcdef"
	s.registerActiveOrchestration(id, func() {})
	if !s.cancelActiveOrchestration(id) {
		t.Fatal("cancel active orchestration failed")
	}

	diskRecord := &orchestrationRecord{
		ID:        id,
		Status:    "canceled",
		Error:     "canceled",
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	appendOrchestrationEvent(diskRecord, "cancel.requested", "", "Cancel requested")
	appendOrchestrationEvent(diskRecord, "canceled", "", "Orchestration canceled")
	if err := saveOrchestrationRecord(diskRecord); err != nil {
		t.Fatal(err)
	}

	staleRecord := &orchestrationRecord{
		ID:        id,
		Status:    "running",
		CreatedAt: diskRecord.CreatedAt,
		UpdatedAt: diskRecord.CreatedAt,
	}
	appendOrchestrationEvent(staleRecord, "planning.finished", "", "Planning finished with 1 subtasks")
	if !s.stopCanceledOrchestration(staleRecord, "Orchestration canceled") {
		t.Fatal("stopCanceledOrchestration returned false")
	}

	updated, err := readOrchestrationRecord(id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "canceled" || len(updated.Events) != 2 {
		t.Fatalf("updated record = %+v", updated)
	}
	if updated.Events[0].Type != "cancel.requested" || updated.Events[1].Type != "canceled" {
		t.Fatalf("events = %+v", updated.Events)
	}
}
