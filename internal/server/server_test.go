package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hakobune8/arun/internal/agent"
	"github.com/hakobune8/arun/internal/apphome"
	arungh "github.com/hakobune8/arun/internal/github"
	"github.com/hakobune8/arun/internal/guideline"
	"github.com/hakobune8/arun/internal/memory"
	"github.com/hakobune8/arun/internal/orchestrator"
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
	assertArrayLen(t, w.Body.Bytes(), 15)
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

func TestServer_AgentsEndpoint_LocalizesBuiltInsForJapaneseUI(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/agents?uiLanguage=ja", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var agents []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &agents); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	found := false
	for _, a := range agents {
		if a["Name"] == "go-backend" {
			found = true
			description, _ := a["Description"].(string)
			if !strings.Contains(description, "既存構成") || !strings.Contains(description, "テスト") {
				t.Fatalf("localized go-backend description = %q", description)
			}
			break
		}
	}
	if !found {
		t.Fatal("go-backend agent not found in list")
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
	t.Setenv("ARUN_HOME", t.TempDir())
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
	t.Setenv("ARUN_HOME", t.TempDir())
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
	t.Setenv("ARUN_HOME", t.TempDir())
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
	t.Setenv("ARUN_HOME", t.TempDir())
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

func TestRepositoryContextSearch_ScopesSourcesByRepository(t *testing.T) {
	t.Setenv("ARUN_HOME", t.TempDir())
	ctx := context.Background()
	memStore, err := repositoryMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := memStore.Save(ctx, &memory.RepositoryEntry{
		Repo:    "owner/repo",
		Branch:  "main",
		Type:    "validation",
		Content: "Run repository scoped search tests.",
		Status:  memory.RepositoryMemoryApproved,
	}); err != nil {
		t.Fatal(err)
	}
	if err := memStore.Save(ctx, &memory.RepositoryEntry{
		Repo:    "other/repo",
		Branch:  "main",
		Type:    "validation",
		Content: "Run repository scoped search tests.",
		Status:  memory.RepositoryMemoryApproved,
	}); err != nil {
		t.Fatal(err)
	}
	glStore, err := repositoryGuidelineStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := glStore.Save(ctx, &guideline.RepositoryGuideline{
		Repo:    "owner/repo",
		Branch:  "main",
		Title:   "Search guideline",
		Content: "Keep context search scoped to the selected repository.",
		Status:  guideline.RepositoryGuidelineActive,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := saveOrchestrationRecord(&orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "Improve repository scoped search",
		Status:     "completed",
		CreatedAt:  now,
		UpdatedAt:  now,
		Plan: &orchestrator.TaskPlan{Subtasks: []orchestrator.Subtask{{
			ID:          "step-1",
			AgentName:   "go-backend",
			Description: "Implement repository scoped context search",
		}}},
	}); err != nil {
		t.Fatal(err)
	}

	results, err := repositoryContextSearch(ctx, repositoryContextSearchQuery{
		Repo:   "owner/repo",
		Branch: "main",
		Query:  "repository scoped search",
		Limit:  20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 3 {
		t.Fatalf("results = %+v, want memory, guideline, and run/artifact results", results)
	}
	for _, result := range results {
		if result.Repo != "owner/repo" {
			t.Fatalf("result repo = %q, want owner/repo: %+v", result.Repo, result)
		}
	}

	memoryOnly, err := repositoryContextSearch(ctx, repositoryContextSearchQuery{
		Repo:   "owner/repo",
		Branch: "main",
		Query:  "repository scoped search",
		Source: "memory",
		Limit:  20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(memoryOnly) != 1 || memoryOnly[0].Source != "memory" {
		t.Fatalf("memoryOnly = %+v, want one memory result", memoryOnly)
	}
}

func TestServer_SearchEndpoint_RepositoryContext(t *testing.T) {
	t.Setenv("ARUN_HOME", t.TempDir())
	s := NewServer(0)
	store, err := repositoryMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), &memory.RepositoryEntry{
		Repo:    "owner/repo",
		Branch:  "main",
		Type:    "architecture",
		Content: "Repository context search endpoint result.",
		Status:  memory.RepositoryMemoryApproved,
	}); err != nil {
		t.Fatal(err)
	}
	w := serveRequest(s, "GET", "/api/search?repo=owner/repo&baseBranch=main&source=memory&q=context", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var results []repositoryContextSearchResult
	if err := json.Unmarshal(w.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Source != "memory" || results[0].Repo != "owner/repo" {
		t.Fatalf("results = %+v, want scoped memory result", results)
	}
}

func TestRepositoryContextSearch_LiveGitHubSourcesRedactAndProvenance(t *testing.T) {
	t.Setenv("ARUN_HOME", t.TempDir())
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/owner/repo/issues":
			_, _ = w.Write([]byte(`[{"number":7,"title":"Investigate failed run","body":"token=ghp_123456789012345678901234567890123456","state":"open","html_url":"https://github.com/owner/repo/issues/7","created_at":"2026-07-01T00:00:00Z"}]`))
		case "/repos/owner/repo/pulls":
			_, _ = w.Write([]byte(`[{"number":9,"title":"Fix report","state":"open","html_url":"https://github.com/owner/repo/pull/9","head":{"ref":"fix"},"base":{"ref":"main"}}]`))
		case "/repos/owner/repo/commits/main/check-runs":
			_, _ = w.Write([]byte(`{"check_runs":[{"id":11,"name":"lint","status":"completed","conclusion":"failure","html_url":"https://github.com/owner/repo/runs/11","output":{"title":"lint failed","summary":"secret=ghp_123456789012345678901234567890123456"}}]}`))
		case "/repos/owner/repo/actions/runs":
			_, _ = w.Write([]byte(`{"workflow_runs":[{"id":42,"name":"CI","display_title":"CI failure report","status":"completed","conclusion":"failure","html_url":"https://github.com/owner/repo/actions/runs/42","head_branch":"main","head_sha":"abc123","created_at":"2026-07-01T00:00:00Z","updated_at":"2026-07-01T00:01:00Z"}]}`))
		case "/repos/owner/repo/actions/runs/42/logs":
			_, _ = w.Write([]byte("build failed with github_token=ghp_123456789012345678901234567890123456\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	t.Setenv("GITHUB_API_URL", api.URL)

	results, err := repositoryContextSearch(context.Background(), repositoryContextSearchQuery{
		Repo:   "owner/repo",
		Branch: "main",
		Query:  "failed",
		Source: "github",
		Limit:  20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 3 {
		t.Fatalf("results = %+v, want live GitHub issue/check/workflow evidence", results)
	}
	body, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "ghp_123456789012345678901234567890123456") {
		t.Fatalf("GitHub evidence leaked token: %s", string(body))
	}
	var sawIssue, sawCheck, sawWorkflow bool
	for _, result := range results {
		if result.Source != "github" || result.Repo != "owner/repo" || result.Branch != "main" || result.URL == "" {
			t.Fatalf("bad provenance result: %+v", result)
		}
		switch result.Metadata["type"] {
		case "issue":
			sawIssue = true
		case "check_run":
			sawCheck = true
		case "workflow_run":
			sawWorkflow = true
		}
	}
	if !sawIssue || !sawCheck || !sawWorkflow {
		t.Fatalf("saw issue=%v check=%v workflow=%v in %+v", sawIssue, sawCheck, sawWorkflow, results)
	}
}

func TestRepositoryContextSearch_KubernetesLogsRedact(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake kubectl shell script is POSIX-only")
	}
	t.Setenv("ARUN_HOME", t.TempDir())
	dir := t.TempDir()
	kubectl := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(kubectl, []byte("#!/bin/sh\necho 'arun failed token=ghp_123456789012345678901234567890123456'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(kubectl, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARUN_KUBECTL", kubectl)
	t.Setenv("ARUN_KUBECONFIG", filepath.Join(dir, "config"))
	t.Setenv("ARUN_KUBERNETES_NAMESPACE", "arun")
	t.Setenv("ARUN_KUBERNETES_SELECTOR", "app.kubernetes.io/name=arun")

	results, err := repositoryContextSearch(context.Background(), repositoryContextSearchQuery{
		Repo:   "owner/repo",
		Branch: "main",
		Query:  "failed",
		Source: "kubernetes",
		Limit:  20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Source != "kubernetes" {
		t.Fatalf("results = %+v, want kubernetes logs", results)
	}
	if strings.Contains(results[0].Content, "ghp_123456789012345678901234567890123456") {
		t.Fatalf("kubernetes logs leaked token: %+v", results[0])
	}
	if results[0].Metadata["namespace"] != "arun" || results[0].Metadata["selector"] != "app.kubernetes.io/name=arun" {
		t.Fatalf("missing kubernetes provenance: %+v", results[0].Metadata)
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
	t.Setenv("ARUN_HOME", home)

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
	testGitHubEndpoint(t, "/api/github/issues?repo=hakobune8/arun")
}

func TestServer_GitHub_Pulls_ValidRepo(t *testing.T) {
	t.Parallel()
	testGitHubEndpoint(t, "/api/github/pulls?repo=hakobune8/arun")
}

func TestServer_GitHub_Checks_ValidRepo(t *testing.T) {
	t.Parallel()
	testGitHubEndpoint(t, "/api/github/checks?repo=hakobune8/arun")
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

func TestServer_GitHub_Repositories(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/installation/repositories" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"repositories":[{"id":1,"name":"repo","full_name":"owner/repo","private":true,"html_url":"https://github.com/owner/repo","default_branch":"main"}]}`))
	}))
	defer api.Close()
	t.Setenv("GITHUB_API_URL", api.URL)

	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/github/repositories", nil)
	assertStatus(t, w.Code, http.StatusOK)

	var repos []arungh.RepositorySummary
	if err := json.Unmarshal(w.Body.Bytes(), &repos); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(repos) != 1 || repos[0].FullName != "owner/repo" || repos[0].DefaultBranch != "main" {
		t.Fatalf("unexpected repositories: %+v", repos)
	}
}

func TestServer_GitHub_RepositoriesUsesOAuthToken(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/installation/repositories":
			t.Fatalf("OAuth-backed repository picker should not call installation repositories")
		case "/user/repos":
			if got := r.Header.Get("Authorization"); got != "Bearer oauth-token" {
				t.Fatalf("Authorization = %q, want OAuth token", got)
			}
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Query().Get("page") {
			case "1":
				repos := make([]arungh.RepositorySummary, 100)
				for i := range repos {
					repos[i] = arungh.RepositorySummary{ID: int64(i + 1), Name: fmt.Sprintf("repo-%03d", i+1), FullName: fmt.Sprintf("owner/repo-%03d", i+1), DefaultBranch: "main"}
				}
				_ = json.NewEncoder(w).Encode(repos)
			case "2":
				_, _ = w.Write([]byte(`[{"id":200,"name":"private-repo","full_name":"owner/private-repo","private":true,"html_url":"https://github.com/owner/private-repo","default_branch":"main"}]`))
			default:
				_, _ = w.Write([]byte(`[]`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()
	t.Setenv("GITHUB_API_URL", api.URL)
	t.Setenv("ARUN_AUTH_REQUIRED", "true")
	t.Setenv("ARUN_SESSION_SECRET", "test-secret")

	s := NewServer(0)
	w := serveRequestAs(s, "GET", "/api/github/repositories", nil, &authUser{
		Login:       "alice",
		AccessToken: "oauth-token",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	})
	assertStatus(t, w.Code, http.StatusOK)

	var repos []arungh.RepositorySummary
	if err := json.Unmarshal(w.Body.Bytes(), &repos); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(repos) != 101 || repos[100].FullName != "owner/private-repo" || !repos[100].Private {
		t.Fatalf("unexpected repositories: %+v", repos)
	}
}

func TestServer_AuthSessionDoesNotExposeAccessToken(t *testing.T) {
	t.Setenv("ARUN_AUTH_REQUIRED", "true")
	t.Setenv("ARUN_SESSION_SECRET", "test-secret")
	s := NewServer(0)
	w := serveRequestAs(s, "GET", "/api/auth/session", nil, &authUser{
		Login:       "alice",
		AccessToken: "secret-token",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	})
	assertStatus(t, w.Code, http.StatusOK)
	if strings.Contains(w.Body.String(), "secret-token") || strings.Contains(w.Body.String(), "accessToken") {
		t.Fatalf("session leaked access token: %s", w.Body.String())
	}
}

func TestServer_FetchGitHubUserRetriesUnauthorizedBearerWithTokenScheme(t *testing.T) {
	var calls int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			if got := r.Header.Get("Authorization"); got != "Bearer oauth-token" {
				t.Fatalf("first Authorization = %q, want bearer token", got)
			}
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		case 2:
			if got := r.Header.Get("Authorization"); got != "token oauth-token" {
				t.Fatalf("retry Authorization = %q, want token scheme", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"login":"alice","name":"Alice","avatar_url":"https://example.com/a.png","html_url":"https://github.com/alice"}`))
		default:
			t.Fatalf("unexpected extra call %d", calls)
		}
	}))
	defer api.Close()

	s := &Server{auth: authConfig{UserURL: api.URL}}
	user, err := s.fetchGitHubUser(context.Background(), "oauth-token")
	if err != nil {
		t.Fatalf("fetchGitHubUser: %v", err)
	}
	if user.Login != "alice" {
		t.Fatalf("login = %q, want alice", user.Login)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestServer_GitHub_UnknownResource(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/github/unknown?repo=hakobune8/arun", nil)
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

func TestGitCloneEnvWithToken_UsesRequestToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token")
	env := gitCloneEnvWithToken([]string{"clone", "https://github.com/owner/repo.git", "/tmp/repo"}, "oauth-token")
	var encoded string
	for _, item := range env {
		encoded = strings.TrimPrefix(item, "GIT_CONFIG_VALUE_0=AUTHORIZATION: basic ")
		if encoded != item {
			break
		}
	}
	if encoded == "" {
		t.Fatal("missing git basic auth value")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode auth value: %v", err)
	}
	if string(decoded) != "x-access-token:oauth-token" {
		t.Fatalf("decoded auth = %q, want request OAuth token", string(decoded))
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
		"hakobune8/arun":                             "https://github.com/hakobune8/arun.git",
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
	t.Setenv("ARUN_HOME", shortTestDir(t))
	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/orchestrates", nil)
	assertStatus(t, w.Code, http.StatusOK)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("body = %s, want []", w.Body.String())
	}
}

func TestServer_Orchestrates_ReturnsLightweightSummaries(t *testing.T) {
	t.Setenv("ARUN_HOME", shortTestDir(t))
	now := time.Now().UTC().Truncate(time.Second)
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "large task",
		Agents:     []string{"go-backend"},
		Status:     "running",
		Plan: &orchestrator.TaskPlan{Subtasks: []orchestrator.Subtask{{
			ID:          "step-1",
			Description: strings.Repeat("plan payload", 100),
		}}},
		Subtasks: []orchestrationSubtaskState{{
			ID:     "step-1",
			Status: "running",
		}},
		Results: []orchestrator.SubtaskResult{{
			SubtaskID: "step-1",
			Output:    strings.Repeat("result payload", 100),
		}},
		Events: []orchestrationEvent{{
			Timestamp: now,
			Type:      "subtask.completed",
			Message:   strings.Repeat("event payload", 100),
		}},
		Summary:   strings.Repeat("summary payload", 100),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := saveOrchestrationRecord(record); err != nil {
		t.Fatalf("saveOrchestrationRecord() error = %v", err)
	}

	s := NewServer(0)
	w := serveRequest(s, "GET", "/api/orchestrates", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var records []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &records); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	got := records[0]
	for _, key := range []string{"plan", "subtasks", "results", "events", "summary"} {
		if _, ok := got[key]; ok {
			t.Fatalf("list response includes heavyweight key %q: %s", key, w.Body.String())
		}
	}
	if got["id"] != record.ID || got["status"] != record.Status || got["task"] != record.Task {
		t.Fatalf("summary = %+v, want id/status/task", got)
	}
}

func TestOrchestrationRecordStore_RoundTrip(t *testing.T) {
	t.Setenv("ARUN_HOME", shortTestDir(t))
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
	if len(templates) < 9 {
		t.Fatalf("templates = %d, want at least 9", len(templates))
	}
	if templates[0].ID == "" || templates[0].TaskTemplate == "" || len(templates[0].Agents) == 0 {
		t.Fatalf("template missing required fields: %+v", templates[0])
	}
	var scrum *scenarioTemplate
	for i := range templates {
		if templates[i].ID == "three-sprint-agile-scrum" {
			scrum = &templates[i]
			break
		}
	}
	if scrum == nil {
		t.Fatalf("three-sprint-agile-scrum template not found: %+v", templates)
		return
	}
	if scrum.CreatePullRequest || scrum.Strategy != "sequential" || !strings.Contains(scrum.TaskTemplate, "Sprint 3") {
		t.Fatalf("scrum template defaults = %+v", scrum)
	}
	if strings.Contains(scrum.TaskTemplate, "Japanese stakeholder report") {
		t.Fatalf("scrum template should not force a Japanese stakeholder report")
	}
	if scrum.Limits.MaxDuration != "45m" || scrum.Limits.MaxSubtasks != 18 || scrum.Limits.MaxConcurrentRepoRun != 1 {
		t.Fatalf("scrum limits = %+v, want 45m/18/1", scrum.Limits)
	}
	for _, want := range []string{"analyst", "release-manager", "reviewer", "qa", "reporter"} {
		if !containsString(scrum.Agents, want) {
			t.Fatalf("scrum agents = %+v, want %s", scrum.Agents, want)
		}
	}

	var heavy *scenarioTemplate
	for i := range templates {
		if templates[i].ID == "implementation-heavy-scrum" {
			heavy = &templates[i]
			break
		}
	}
	if heavy == nil {
		t.Fatalf("implementation-heavy-scrum template not found: %+v", templates)
		return
	}
	if !heavy.CreateIssue || !heavy.CreatePullRequest || heavy.Strategy != "sequential" || !strings.Contains(heavy.TaskTemplate, "build-first") {
		t.Fatalf("implementation-heavy-scrum defaults = %+v", heavy)
	}
	if strings.Contains(heavy.TaskTemplate, "Japanese stakeholder report") {
		t.Fatalf("implementation-heavy-scrum template should not force a Japanese stakeholder report")
	}
	for _, want := range []string{"Go HTTP server", "Helm chart", "Kubernetes manifests", "GitHub Actions CI", "Quality bar", "Acceptance criteria", "repository layout", "duplicated documentation", "product-centered", "source of truth", "Product coherence status"} {
		if !strings.Contains(heavy.TaskTemplate, want) {
			t.Fatalf("implementation-heavy-scrum task missing %q", want)
		}
	}
	if heavy.Limits.MaxDuration != "180m" || heavy.Limits.MaxSubtasks != 30 || heavy.Limits.MaxConcurrentRepoRun != 1 {
		t.Fatalf("implementation-heavy-scrum limits = %+v, want 180m/30/1", heavy.Limits)
	}
	for _, want := range []string{"analyst", "go-backend", "frontend", "docs", "qa", "reviewer", "release-manager", "docker", "helm", "kubernetes", "devops"} {
		if !containsString(heavy.Agents, want) {
			t.Fatalf("implementation-heavy-scrum agents = %+v, want %s", heavy.Agents, want)
		}
	}
}

func TestServer_OrchestrateTemplates_LocalizesBuiltInsForJapaneseUI(t *testing.T) {
	s := NewServer(0)
	w := serveRequest(s, "POST", "/api/orchestrate/templates", []byte(`{"repo":"","baseBranch":"main","uiLanguage":"ja"}`))
	assertStatus(t, w.Code, http.StatusOK)
	var templates []scenarioTemplate
	if err := json.Unmarshal(w.Body.Bytes(), &templates); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var heavy *scenarioTemplate
	for i := range templates {
		if templates[i].ID == "implementation-heavy-scrum" {
			heavy = &templates[i]
			break
		}
	}
	if heavy == nil {
		t.Fatalf("implementation-heavy-scrum template not found: %+v", templates)
	}
	if heavy.Name != "実装重視 Scrum" || heavy.OutputLanguage != "ja" {
		t.Fatalf("localized heavy template = %+v", heavy)
	}
	if !strings.Contains(heavy.Description, "sandbox リポジトリ") {
		t.Fatalf("localized heavy description = %q", heavy.Description)
	}
	for _, want := range []string{"Quality bar", "acceptance criteria", "Fresh checkout", "repository layout", "重複 documentation", "product-centered", "source of truth", "Product coherence status"} {
		if !strings.Contains(heavy.TaskTemplate, want) {
			t.Fatalf("localized implementation-heavy-scrum task missing %q", want)
		}
	}
	if !strings.Contains(heavy.TaskTemplate, "{{repo}} の {{baseBranch}} 上で") || !strings.Contains(heavy.TaskTemplate, "出力言語: 日本語。") {
		t.Fatalf("localized heavy task missing language instruction: %q", heavy.TaskTemplate)
	}
	if strings.Contains(heavy.TaskTemplate, "Run an implementation-heavy agile scrum workflow") {
		t.Fatalf("localized heavy task should not keep English leading sentence: %q", heavy.TaskTemplate)
	}
}

func TestServer_OrchestrateTemplates_ReturnsBuiltInsWhenRepositoryUnavailable(t *testing.T) {
	t.Setenv("ARUN_AUTH_REQUIRED", "true")
	t.Setenv("ARUN_SESSION_SECRET", "test-secret")
	s := NewServer(0)
	w := serveRequestAs(s, "POST", "/api/orchestrate/templates", []byte(`{"repo":"not-a-valid-repo","baseBranch":"main"}`), &authUser{
		Login:       "alice",
		AccessToken: "oauth-token",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	})
	assertStatus(t, w.Code, http.StatusOK)

	var templates []scenarioTemplate
	if err := json.Unmarshal(w.Body.Bytes(), &templates); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !containsScenarioTemplate(templates, "three-sprint-agile-scrum") {
		t.Fatalf("templates missing built-in scrum template: %+v", templates)
	}
}

func TestImplementationHeavyScrumPlan_UsesSprintStageWorkflow(t *testing.T) {
	record := &orchestrationRecord{
		Task:     "Build an invaders game",
		Scenario: &scenarioTemplateSelection{ID: "implementation-heavy-scrum"},
	}

	plan := implementationHeavyScrumPlan(record)
	if plan == nil {
		t.Fatal("plan is nil")
		return
	}
	if len(plan.Subtasks) != 25 {
		t.Fatalf("subtasks = %d, want 25", len(plan.Subtasks))
	}
	agents := make([]string, 0, len(plan.Subtasks))
	byID := map[string]orchestrator.Subtask{}
	for _, subtask := range plan.Subtasks {
		agents = append(agents, subtask.AgentName)
		byID[subtask.ID] = subtask
	}
	for _, want := range []string{"analyst", "go-backend", "frontend", "qa", "docker", "helm", "kubernetes", "devops", "docs", "reviewer", "release-manager"} {
		if !containsString(agents, want) {
			t.Fatalf("plan agents = %+v, want %s", agents, want)
		}
	}
	if got := byID["sprint-1-adjust-plan"].Deps; len(got) != 1 || got[0] != "sprint-1-qa" {
		t.Fatalf("sprint-1-adjust-plan deps = %+v, want sprint-1-qa", got)
	}
	if got := byID["sprint-1-backend-fix"].Deps; len(got) != 1 || got[0] != "sprint-1-adjust-plan" {
		t.Fatalf("sprint-1-backend-fix deps = %+v, want sprint-1-adjust-plan", got)
	}
	if got := byID["sprint-2-plan"].Deps; len(got) != 1 || got[0] != "sprint-1-report" {
		t.Fatalf("sprint-2-plan deps = %+v, want sprint-1-report", got)
	}
	if got := byID["sprint-2-backend"].Deps; len(got) != 1 || got[0] != "sprint-2-plan" {
		t.Fatalf("sprint-2-backend deps = %+v, want sprint-2-plan", got)
	}
	if got := byID["sprint-2-docker"].Deps; len(got) != 1 || got[0] != "sprint-2-backend" {
		t.Fatalf("sprint-2-docker deps = %+v, want sprint-2-backend", got)
	}
	if !strings.Contains(byID["sprint-1-adjust-plan"].Description, "Sprint 1 QA") {
		t.Fatalf("sprint-1-adjust-plan description = %q, want QA evidence handoff", byID["sprint-1-adjust-plan"].Description)
	}
	if !strings.Contains(byID["sprint-1-plan"].Description, "product concept") || !strings.Contains(byID["sprint-1-plan"].Description, "differentiating mechanic") || !strings.Contains(byID["sprint-1-plan"].Description, "docs/product-brief.md as the single source-of-truth product brief") {
		t.Fatalf("sprint-1-plan description = %q, want product/design gate", byID["sprint-1-plan"].Description)
	}
	if !strings.Contains(byID["sprint-1-plan"].Description, "do not create product/design.md") ||
		!strings.Contains(byID["sprint-1-plan"].Description, "not only in documentation") {
		t.Fatalf("sprint-1-plan description = %q, want duplicate brief and implemented mechanic guard", byID["sprint-1-plan"].Description)
	}
	if !strings.Contains(byID["sprint-1-qa"].Description, "Compare docs/product-brief.md against README") ||
		!strings.Contains(byID["sprint-1-qa"].Description, "duplicate product brief files") {
		t.Fatalf("sprint-1-qa description = %q, want product coherence QA", byID["sprint-1-qa"].Description)
	}
	if !strings.Contains(byID["sprint-2-docker"].Description, "container root path returns the same primary UI") {
		t.Fatalf("sprint-2-docker description = %q, want container UI parity guidance", byID["sprint-2-docker"].Description)
	}
	if !strings.Contains(byID["sprint-2-qa"].Description, "verify `/healthz` and `/` from the container") ||
		!strings.Contains(byID["sprint-2-qa"].Description, "runtime image copies all files needed for the primary UI") {
		t.Fatalf("sprint-2-qa description = %q, want container smoke gate", byID["sprint-2-qa"].Description)
	}
	if !strings.Contains(byID["sprint-3-docs"].Description, "README H1 must be the product name") {
		t.Fatalf("sprint-3-docs description = %q, want README H1 product gate", byID["sprint-3-docs"].Description)
	}
	if !strings.Contains(byID["sprint-3-review"].Description, "accidental binary/workspace artifacts") {
		t.Fatalf("sprint-3-review description = %q, want artifact hygiene check", byID["sprint-3-review"].Description)
	}
	if !strings.Contains(byID["sprint-3-review"].Description, "product concept drift") {
		t.Fatalf("sprint-3-review description = %q, want concept drift review", byID["sprint-3-review"].Description)
	}
	if sprint, ok := scrumSprintCheckpoint("sprint-3-report"); !ok || sprint != 3 {
		t.Fatalf("sprint-3-report checkpoint = %d/%v, want 3/true", sprint, ok)
	}
}

func TestCommitScrumSprintCheckpoint_CreatesThreeCheckpointCommits(t *testing.T) {
	repo := t.TempDir()
	runGitTestCommand(t, repo, "init")
	record := &orchestrationRecord{
		ID:       "run-test",
		RepoPath: repo,
		Scenario: &scenarioTemplateSelection{ID: "implementation-heavy-scrum"},
	}

	for _, id := range []string{"sprint-1-report", "sprint-2-report", "sprint-3-report"} {
		event := &orchestrator.SubtaskEvent{
			Type:    orchestrator.SubtaskCompleted,
			Subtask: orchestrator.Subtask{ID: id},
			Result:  &orchestrator.SubtaskResult{Success: true},
		}
		if err := commitScrumSprintCheckpoint(record, event); err != nil {
			t.Fatalf("commit checkpoint %s: %v", id, err)
		}
	}

	out := runGitTestCommand(t, repo, "rev-list", "--count", "HEAD")
	if strings.TrimSpace(out) != "3" {
		t.Fatalf("commit count = %q, want 3", strings.TrimSpace(out))
	}
	if len(record.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(record.Events))
	}
}

func TestShouldPublishScrumSprintCheckpoint_OnlyAfterSuccessfulCompletion(t *testing.T) {
	record := &orchestrationRecord{
		ID:       "run-test",
		Scenario: &scenarioTemplateSelection{ID: "implementation-heavy-scrum"},
	}
	started := &orchestrator.SubtaskEvent{
		Type:    orchestrator.SubtaskStarted,
		Subtask: orchestrator.Subtask{ID: "sprint-1-report"},
	}
	if sprint, ok := shouldPublishScrumSprintCheckpoint(record, started); ok || sprint != 0 {
		t.Fatalf("started checkpoint publish = %d/%v, want 0/false", sprint, ok)
	}
	failed := &orchestrator.SubtaskEvent{
		Type:    orchestrator.SubtaskCompleted,
		Subtask: orchestrator.Subtask{ID: "sprint-1-report"},
		Result:  &orchestrator.SubtaskResult{Success: false},
	}
	if sprint, ok := shouldPublishScrumSprintCheckpoint(record, failed); ok || sprint != 0 {
		t.Fatalf("failed checkpoint publish = %d/%v, want 0/false", sprint, ok)
	}
	completed := &orchestrator.SubtaskEvent{
		Type:    orchestrator.SubtaskCompleted,
		Subtask: orchestrator.Subtask{ID: "sprint-2-report"},
		Result:  &orchestrator.SubtaskResult{Success: true},
	}
	if sprint, ok := shouldPublishScrumSprintCheckpoint(record, completed); !ok || sprint != 2 {
		t.Fatalf("completed checkpoint publish = %d/%v, want 2/true", sprint, ok)
	}
}

func TestCommitScrumSprintCheckpoint_ScrubsGeneratedArtifacts(t *testing.T) {
	repo := t.TempDir()
	runGitTestCommand(t, repo, "init")
	binaryName := "20260704T032604-run-fd7ee96cb4117060-hakobune8-arun-test"
	if err := os.WriteFile(filepath.Join(repo, binaryName), []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01}, 0o600); err != nil {
		t.Fatal(err)
	}
	readme := "# Product\n\nUseful overview.\n\n## Scenario\n\nParent task:\nmake a game\n\nOperating mode: build-first\n\nQuality bar:\n- good\n\nExpected output:\n- files\n"
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(readme), 0o600); err != nil {
		t.Fatal(err)
	}
	record := &orchestrationRecord{
		ID:       "run-test",
		RepoPath: repo,
		Scenario: &scenarioTemplateSelection{ID: "implementation-heavy-scrum"},
	}

	event := &orchestrator.SubtaskEvent{
		Type:    orchestrator.SubtaskCompleted,
		Subtask: orchestrator.Subtask{ID: "sprint-3-report"},
		Result:  &orchestrator.SubtaskResult{Success: true},
	}
	if err := commitScrumSprintCheckpoint(record, event); err != nil {
		t.Fatalf("commit checkpoint: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, binaryName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary artifact stat err = %v, want not exist", err)
	}
	cleaned, err := os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cleaned), "Parent task:") || strings.Contains(string(cleaned), "Quality bar:") {
		t.Fatalf("README still contains prompt block:\n%s", cleaned)
	}
	if !strings.Contains(string(cleaned), "Useful overview.") {
		t.Fatalf("README lost product content:\n%s", cleaned)
	}
	show := runGitTestCommand(t, repo, "show", "--name-only", "--format=", "HEAD")
	if strings.Contains(show, binaryName) {
		t.Fatalf("checkpoint committed binary artifact:\n%s", show)
	}
	if len(record.Events) != 2 {
		t.Fatalf("events = %d, want hygiene plus commit", len(record.Events))
	}
}

func TestRecoverWorkflowScopePushFailure_MovesWorkflowToFollowUp(t *testing.T) {
	repo := t.TempDir()
	runGitTestCommand(t, repo, "init")
	workflowDir := filepath.Join(repo, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflow := "name: CI\n\non:\n  pull_request:\n\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./...\n"
	if err := os.WriteFile(filepath.Join(workflowDir, "ci.yml"), []byte(workflow), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "config", "user.email", "arun@example.invalid")
	runGitTestCommand(t, repo, "config", "user.name", "ARUN")
	runGitTestCommand(t, repo, "add", ".")
	runGitTestCommand(t, repo, "commit", "-m", "ARUN run-test sprint 3 checkpoint")
	record := &orchestrationRecord{ID: "run-test", RepoPath: repo}
	pushErr := fmt.Errorf("git push: remote rejected .github/workflows/ci.yml without `workflow` scope")

	if err := recoverWorkflowScopePushFailure(record, pushErr); err != nil {
		t.Fatalf("recover workflow scope push failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workflowDir, "ci.yml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workflow file stat err = %v, want not exist", err)
	}
	doc, err := os.ReadFile(filepath.Join(repo, "docs", "arun-generated-workflows.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), ".github/workflows/ci.yml") || !strings.Contains(string(doc), "go test ./...") {
		t.Fatalf("follow-up doc missing workflow content:\n%s", doc)
	}
	show := runGitTestCommand(t, repo, "show", "--name-only", "--format=", "HEAD")
	if strings.Contains(show, ".github/workflows/ci.yml") {
		t.Fatalf("recovery commit includes workflow file:\n%s", show)
	}
	if !strings.Contains(show, "docs/arun-generated-workflows.md") {
		t.Fatalf("recovery commit missing follow-up doc:\n%s", show)
	}
	count := strings.TrimSpace(runGitTestCommand(t, repo, "rev-list", "--count", "HEAD"))
	if count != "1" {
		t.Fatalf("commit count = %s, want recovery to amend checkpoint commit", count)
	}
	if len(record.Events) != 1 || record.Events[0].Type != "workflow_scope.recovery" {
		t.Fatalf("events = %+v, want workflow scope recovery event", record.Events)
	}
}

func TestCreatePullRequestForOrchestration_MarksPublishErrorStatus(t *testing.T) {
	record := &orchestrationRecord{
		ID:       "run-test",
		RepoPath: t.TempDir(),
		Status:   "completed",
		GitHub: &orchestrationGitHubState{
			Repo:              "not a repo",
			CreatePullRequest: true,
		},
	}
	(&Server{}).createPullRequestForOrchestration(record)
	if record.Status != "completed_with_publish_error" {
		t.Fatalf("status = %q, want completed_with_publish_error", record.Status)
	}
	if !strings.Contains(record.GitHub.Error, "invalid GitHub repository") {
		t.Fatalf("github error = %q, want invalid repository", record.GitHub.Error)
	}
}

func TestPrepareOrchestrationGitHub_ForcesImplementationHeavyArtifacts(t *testing.T) {
	got, err := prepareOrchestrationGitHub("run-test", &orchestrateRequest{
		Repo:     "owner/repo",
		Task:     "Build product",
		Scenario: &scenarioTemplateSelection{ID: "implementation-heavy-scrum"},
		GitHub: &orchestrateGitHubRequest{
			CreateIssue:       false,
			CreatePullRequest: false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.CreateIssue || !got.CreatePullRequest {
		t.Fatalf("github state = %+v, want forced issue and PR", got)
	}
	if got.BranchName != "arun/run-test" || got.PRBase != "main" {
		t.Fatalf("github state = %+v, want default branch and PR base", got)
	}
}

func TestPrepareOrchestrationGitHub_ForcesImplementationHeavyArtifactsWithoutGitHubRequest(t *testing.T) {
	got, err := prepareOrchestrationGitHub("run-test", &orchestrateRequest{
		Repo:     "owner/repo",
		Task:     "Build product",
		Scenario: &scenarioTemplateSelection{ID: "implementation-heavy-scrum"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.CreateIssue || !got.CreatePullRequest {
		t.Fatalf("github state = %+v, want forced issue and PR", got)
	}
}

func TestGitPushHeadRefreshesRemoteTrackingRef(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	runGitTestCommand(t, t.TempDir(), "init", "--bare", remote)

	repo := t.TempDir()
	runGitTestCommand(t, repo, "init")
	runGitTestCommand(t, repo, "config", "user.email", "arun@example.invalid")
	runGitTestCommand(t, repo, "config", "user.name", "ARUN")
	runGitTestCommand(t, repo, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "add", ".")
	runGitTestCommand(t, repo, "commit", "-m", "one")

	const branch = "arun/run-test"
	if err := gitPushHead(repo, "", branch); err != nil {
		t.Fatalf("first gitPushHead: %v", err)
	}
	firstHead := strings.TrimSpace(runGitTestCommand(t, repo, "rev-parse", "HEAD"))
	firstTracking := strings.TrimSpace(runGitTestCommand(t, repo, "rev-parse", "refs/remotes/origin/"+branch))
	if firstTracking != firstHead {
		t.Fatalf("tracking ref after first push = %s, want %s", firstTracking, firstHead)
	}

	runGitTestCommand(t, repo, "update-ref", "-d", "refs/remotes/origin/"+branch)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "add", ".")
	runGitTestCommand(t, repo, "commit", "-m", "two")

	if err := gitPushHead(repo, "", branch); err != nil {
		t.Fatalf("second gitPushHead with missing tracking ref: %v", err)
	}
	secondHead := strings.TrimSpace(runGitTestCommand(t, repo, "rev-parse", "HEAD"))
	secondTracking := strings.TrimSpace(runGitTestCommand(t, repo, "rev-parse", "refs/remotes/origin/"+branch))
	if secondTracking != secondHead {
		t.Fatalf("tracking ref after second push = %s, want %s", secondTracking, secondHead)
	}
	remoteHead := strings.TrimSpace(runGitTestCommand(t, repo, "ls-remote", "origin", "refs/heads/"+branch))
	if !strings.HasPrefix(remoteHead, secondHead+"\t") {
		t.Fatalf("remote head = %q, want %s", remoteHead, secondHead)
	}
}

func TestGitPushHeadForceRefreshesRewrittenRemoteTrackingRef(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	runGitTestCommand(t, t.TempDir(), "init", "--bare", remote)

	repo := t.TempDir()
	runGitTestCommand(t, repo, "init")
	runGitTestCommand(t, repo, "config", "user.email", "arun@example.invalid")
	runGitTestCommand(t, repo, "config", "user.name", "ARUN")
	runGitTestCommand(t, repo, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "add", ".")
	runGitTestCommand(t, repo, "commit", "-m", "one")

	const branch = "arun/run-test"
	if err := gitPushHead(repo, "", branch); err != nil {
		t.Fatalf("first gitPushHead: %v", err)
	}

	peer := t.TempDir()
	runGitTestCommand(t, peer, "clone", remote, ".")
	runGitTestCommand(t, peer, "config", "user.email", "arun@example.invalid")
	runGitTestCommand(t, peer, "config", "user.name", "ARUN")
	runGitTestCommand(t, peer, "checkout", "--orphan", "rewrite")
	if err := os.WriteFile(filepath.Join(peer, "README.md"), []byte("remote rewrite\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, peer, "add", ".")
	runGitTestCommand(t, peer, "commit", "-m", "remote rewrite")
	runGitTestCommand(t, peer, "push", "--force", "origin", "HEAD:refs/heads/"+branch)
	rewrittenRemote := strings.TrimSpace(runGitTestCommand(t, peer, "rev-parse", "HEAD"))

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("local checkpoint\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTestCommand(t, repo, "add", ".")
	runGitTestCommand(t, repo, "commit", "-m", "local checkpoint")
	if err := gitPushHead(repo, "", branch); err != nil {
		t.Fatalf("gitPushHead after remote rewrite: %v", err)
	}

	head := strings.TrimSpace(runGitTestCommand(t, repo, "rev-parse", "HEAD"))
	tracking := strings.TrimSpace(runGitTestCommand(t, repo, "rev-parse", "refs/remotes/origin/"+branch))
	if tracking != head {
		t.Fatalf("tracking ref after rewritten push = %s, want %s", tracking, head)
	}
	remoteHead := strings.TrimSpace(runGitTestCommand(t, repo, "ls-remote", "origin", "refs/heads/"+branch))
	if !strings.HasPrefix(remoteHead, head+"\t") {
		t.Fatalf("remote head = %q, want %s after replacing rewritten remote %s", remoteHead, head, rewrittenRemote)
	}
}

func runGitTestCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out)
}

func containsScenarioTemplate(templates []scenarioTemplate, id string) bool {
	for i := range templates {
		if templates[i].ID == id {
			return true
		}
	}
	return false
}

func TestServer_ResolveOrchestrationStagePresets_UsesRecommendedPresets(t *testing.T) {
	t.Setenv("ARUN_LLM_DEFAULT_PRESET", "staips")
	t.Setenv("ARUN_LLM_PRESETS", `[
		{"id":"staips","name":"STAIPS","provider":"litellm","baseUrl":"http://litellm:4000","model":"default"},
		{"id":"planning","name":"Planning","provider":"litellm","baseUrl":"http://litellm:4000","model":"planner"},
		{"id":"coding","name":"Coding","provider":"litellm","baseUrl":"http://litellm:4000","model":"coder"},
		{"id":"review","name":"Review","provider":"litellm","baseUrl":"http://litellm:4000","model":"reviewer"},
		{"id":"smoke","name":"Smoke","provider":"litellm","baseUrl":"http://litellm:4000","model":"smoke"},
		{"id":"reporting","name":"Reporting","provider":"litellm","baseUrl":"http://litellm:4000","model":"reporter"}
	]`)
	s := NewServer(0)
	routing := s.resolveOrchestrationStagePresets([]string{"go-backend", "reviewer", "qa", "analyst", "reporter"}, "staips")

	if !routing.enabled {
		t.Fatal("routing disabled")
	}
	want := map[string]string{
		"planning":         "planning",
		"agent:go-backend": "coding",
		"agent:reviewer":   "review",
		"agent:qa":         "smoke",
		"agent:analyst":    "planning",
		"agent:reporter":   "reporting",
	}
	if len(routing.records) != 6 {
		t.Fatalf("records = %+v, want 6 entries", routing.records)
	}
	for _, rec := range routing.records {
		if rec.Fallback {
			t.Fatalf("unexpected fallback: %+v", rec)
		}
		key := rec.Stage
		if rec.Agent != "" {
			key = "agent:" + rec.Agent
		}
		if want[key] != rec.PresetID {
			t.Fatalf("%s preset = %q, want %q; records=%+v", key, rec.PresetID, want[key], routing.records)
		}
	}
}

func TestServer_ResolveOrchestrationStagePresets_FallsBackToSelectedPreset(t *testing.T) {
	t.Setenv("ARUN_LLM_DEFAULT_PRESET", "staips")
	t.Setenv("ARUN_LLM_PRESETS", `[{"id":"staips","name":"STAIPS","provider":"litellm","baseUrl":"http://litellm:4000","model":"default"}]`)
	s := NewServer(0)
	routing := s.resolveOrchestrationStagePresets([]string{"go-backend", "reviewer"}, "staips")

	if len(routing.records) != 3 {
		t.Fatalf("records = %+v, want planning plus two agents", routing.records)
	}
	for _, rec := range routing.records {
		if rec.PresetID != "staips" || !rec.Fallback || rec.Reason == "" {
			t.Fatalf("fallback record = %+v, want staips fallback with reason", rec)
		}
	}
}

func TestLoadRepositoryScenarioTemplates_LoadsValidTemplates(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".arun", "scenarios")
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
	dir := filepath.Join(repo, ".arun", "scenarios")
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
	dir := filepath.Join(repo, ".arun", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "frontend.yaml"), []byte(`apiVersion: arun.io/v1
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
	if got.BranchName != "arun/run-0123456789abcdef" {
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

func TestNormalizeGovernanceLimits_DefaultsAndValidation(t *testing.T) {
	got, err := normalizeGovernanceLimits(governanceLimits{})
	if err != nil {
		t.Fatalf("normalizeGovernanceLimits() error = %v", err)
	}
	if got.MaxDuration != "30m0s" || got.MaxSubtasks != 12 || got.MaxConcurrentRepoRun != 1 {
		t.Fatalf("limits = %+v, want defaults", got)
	}

	_, err = normalizeGovernanceLimits(governanceLimits{MaxDuration: "0s"})
	if err == nil || !strings.Contains(err.Error(), "positive duration") {
		t.Fatalf("error = %v, want positive duration validation", err)
	}
}

func TestEnforceGovernancePlan_RejectsTooManySubtasks(t *testing.T) {
	record := &orchestrationRecord{Limits: governanceLimits{MaxSubtasks: 1}}
	plan := &orchestrator.TaskPlan{Subtasks: []orchestrator.Subtask{
		{ID: "step-1", AgentName: "docs"},
		{ID: "step-2", AgentName: "reviewer"},
	}}
	err := enforceGovernancePlan(record, plan)
	if err == nil || !strings.Contains(err.Error(), "subtask limit exceeded") {
		t.Fatalf("error = %v, want subtask limit exceeded", err)
	}
}

func TestServer_EnforceGovernanceBeforeStartRejectsRepoConcurrency(t *testing.T) {
	t.Setenv("ARUN_HOME", shortTestDir(t))
	s := NewServer(0)
	now := time.Now().UTC()
	active := &orchestrationRecord{
		ID:         "run-1111111111111111",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "active",
		Status:     "running",
		CreatedAt:  now.Add(-time.Minute),
		UpdatedAt:  now,
	}
	if err := saveOrchestrationRecord(active); err != nil {
		t.Fatal(err)
	}

	req := &orchestrateRequest{Repo: "https://github.com/owner/repo.git"}
	err := s.enforceGovernanceBeforeStart(req, governanceLimits{MaxConcurrentRepoRun: 1})
	if err == nil || !strings.Contains(err.Error(), "repo concurrency limit exceeded") {
		t.Fatalf("error = %v, want repo concurrency rejection", err)
	}
}

func TestPrepareSchedule_NormalizesGovernanceLimits(t *testing.T) {
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	schedule := &scheduleDefinition{
		ID:                "schedule-1111111111111111",
		Name:              "test",
		Repo:              ".",
		Task:              "report",
		Agents:            []string{"reporter"},
		Strategy:          "sequential",
		Schedule:          scheduleSpec{Type: "interval", Interval: "1h", Timezone: "UTC"},
		ConcurrencyPolicy: schedulePolicyForbid,
		Limits:            governanceLimits{MaxDuration: "5m", MaxSubtasks: 2},
	}
	if err := prepareSchedule(schedule, now); err != nil {
		t.Fatalf("prepareSchedule() error = %v", err)
	}
	req := schedule.orchestrateRequest()
	if req.Limits.MaxDuration != "5m" || req.Limits.MaxSubtasks != 2 || req.Limits.MaxConcurrentRepoRun != 1 {
		t.Fatalf("request limits = %+v", req.Limits)
	}
}

func TestStorageCleanup_ArchivesOldOrchestrationAndArtifacts(t *testing.T) {
	home := shortTestDir(t)
	t.Setenv("ARUN_HOME", home)
	now := time.Now().UTC()
	newRecord := &orchestrationRecord{
		ID:         "run-1111111111111111",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "new",
		Status:     "completed",
		CreatedAt:  now.Add(-2 * time.Hour),
		UpdatedAt:  now.Add(-90 * time.Minute),
	}
	oldRecord := &orchestrationRecord{
		ID:         "run-2222222222222222",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "old",
		Status:     "completed",
		CreatedAt:  now.Add(-4 * time.Hour),
		UpdatedAt:  now.Add(-3 * time.Hour),
	}
	if err := saveOrchestrationRecord(newRecord); err != nil {
		t.Fatal(err)
	}
	if err := saveOrchestrationRecord(oldRecord); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(apphome.RunsDir(), oldRecord.ID+"-step-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "summary.md"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := now.Add(-3 * time.Hour)
	if err := os.Chtimes(runDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	result, err := runStorageCleanup(context.Background(), storageCleanupRequest{
		Policy: &storagePolicy{
			OrchestrationRetention: "1h",
			RunArtifactRetention:   "1h",
			WorkspaceRetention:     "0",
			MemoryRetention:        "0",
			GuidelineRetention:     "0",
			KeepLastOrchestrations: 1,
			ArchiveBeforeDelete:    true,
		},
	})
	if err != nil {
		t.Fatalf("runStorageCleanup() error = %v", err)
	}
	if result.Summary.Archived != 2 {
		t.Fatalf("summary = %+v, want 2 archived", result.Summary)
	}
	if _, err := readOrchestrationRecord(oldRecord.ID); err == nil {
		t.Fatal("old orchestration still exists")
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("run artifacts stat error = %v, want not exist", err)
	}
	if count := entryCount(filepath.Join(home, "archive", "orchestrates")); count != 1 {
		t.Fatalf("archived orchestrations = %d, want 1", count)
	}
	if count := dirEntryCount(filepath.Join(home, "archive", "runs")); count != 1 {
		t.Fatalf("archived runs = %d, want 1", count)
	}
}

func TestStorageCleanup_SkipsGitHubLinkedOrchestrationByDefault(t *testing.T) {
	t.Setenv("ARUN_HOME", shortTestDir(t))
	now := time.Now().UTC()
	if err := saveOrchestrationRecord(&orchestrationRecord{
		ID:         "run-1111111111111111",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "new",
		Status:     "completed",
		CreatedAt:  now.Add(-2 * time.Hour),
		UpdatedAt:  now.Add(-90 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	linked := &orchestrationRecord{
		ID:         "run-2222222222222222",
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "linked",
		Status:     "completed",
		GitHub:     &orchestrationGitHubState{IssueURL: "https://github.com/owner/repo/issues/1"},
		CreatedAt:  now.Add(-4 * time.Hour),
		UpdatedAt:  now.Add(-3 * time.Hour),
	}
	if err := saveOrchestrationRecord(linked); err != nil {
		t.Fatal(err)
	}

	result, err := runStorageCleanup(context.Background(), storageCleanupRequest{
		DryRun: true,
		Policy: &storagePolicy{
			OrchestrationRetention: "1h",
			RunArtifactRetention:   "0",
			WorkspaceRetention:     "0",
			MemoryRetention:        "0",
			GuidelineRetention:     "0",
			KeepLastOrchestrations: 1,
			ArchiveBeforeDelete:    true,
		},
	})
	if err != nil {
		t.Fatalf("runStorageCleanup() error = %v", err)
	}
	if result.Summary.Skipped != 1 || len(result.Items) != 1 || !result.Items[0].Skipped {
		t.Fatalf("result = %+v, want skipped linked record", result)
	}
	if !strings.Contains(result.Items[0].Reason, "GitHub-linked") {
		t.Fatalf("reason = %q, want GitHub-linked", result.Items[0].Reason)
	}
}

func entryCount(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	return len(entries)
}

func TestStorageCleanup_ArchivesOldRepositoryMemory(t *testing.T) {
	t.Setenv("ARUN_HOME", shortTestDir(t))
	old := time.Now().UTC().Add(-2 * time.Hour)
	data, err := json.Marshal([]memory.RepositoryEntry{
		{
			ID:        "repo-mem-1",
			Repo:      "owner/repo",
			Branch:    "main",
			Type:      "note",
			Content:   "stale",
			Status:    memory.RepositoryMemoryApproved,
			CreatedAt: old,
			UpdatedAt: old,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(repositoryMemoryPath(), data, 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := runStorageCleanup(context.Background(), storageCleanupRequest{
		Policy: &storagePolicy{
			Repo:                     "owner/repo",
			BaseBranch:               "main",
			OrchestrationRetention:   "0",
			RunArtifactRetention:     "0",
			WorkspaceRetention:       "0",
			MemoryRetention:          "1h",
			GuidelineRetention:       "0",
			KeepLastOrchestrations:   1,
			ArchiveBeforeDelete:      true,
			AllowLinkedGitHubCleanup: false,
		},
	})
	if err != nil {
		t.Fatalf("runStorageCleanup() error = %v", err)
	}
	if result.Summary.Archived != 1 {
		t.Fatalf("summary = %+v, want 1 archived", result.Summary)
	}
	store, err := repositoryMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.Get(context.Background(), "repo-mem-1")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Status != memory.RepositoryMemoryArchived {
		t.Fatalf("Status = %q, want archived", entry.Status)
	}
}

func TestOrchestrationRecordStore_PreservesGitHubArtifacts(t *testing.T) {
	t.Setenv("ARUN_HOME", shortTestDir(t))
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
			BranchName:            "arun/run-0123456789abcdef",
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
			BranchName:    "arun/run-0123456789abcdef",
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
	if !strings.Contains(issue, "## タスク") || !strings.Contains(issue, "ARUN Orchestrate により作成されました") {
		t.Fatalf("japanese issue body = %q", issue)
	}
	pr = orchestrationPRBody(record)
	if !strings.Contains(pr, "## 概要") || !strings.Contains(pr, "Done") {
		t.Fatalf("japanese PR body = %q", pr)
	}
}

func TestArtifactTemplates_PRBodyIsShortenedForReadability(t *testing.T) {
	record := &orchestrationRecord{
		ID:             "run-0123456789abcdef",
		BaseBranch:     "main",
		OutputLanguage: "ja",
		Agents:         []string{"release-manager"},
		Strategy:       "sequential",
		GitHub: &orchestrationGitHubState{
			Repo:       "owner/repo",
			BranchName: "arun/run-0123456789abcdef",
			PRTemplate: "default",
			PRBase:     "main",
			IssueURL:   "https://github.com/owner/repo/issues/1",
		},
		Summary: strings.Repeat("長いサマリーです。", 10000),
	}

	body := orchestrationPRBody(record)
	if len([]byte(body)) > githubPullRequestBodyReadableBytes {
		t.Fatalf("PR body bytes = %d, want <= %d", len([]byte(body)), githubPullRequestBodyReadableBytes)
	}
	if !strings.Contains(body, "読みやすさを保つため") && !strings.Contains(body, "概要は短縮されています") {
		start := len(body) - 400
		if start < 0 {
			start = 0
		}
		t.Fatalf("PR body missing readability notice: %q", body[start:])
	}
	if !strings.Contains(body, "run-0123456789abcdef") {
		t.Fatalf("PR body missing run metadata")
	}
}

func TestArtifactTemplates_RepositoryConfigFallback(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".arun"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".arun", "config.yaml"), []byte(`outputLanguage: ja
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

func TestOrchestrationRecord_DoesNotPersistGitHubToken(t *testing.T) {
	record := &orchestrationRecord{
		ID:          "run-0123456789abcdef",
		Repo:        "owner/repo",
		BaseBranch:  "main",
		Status:      "completed",
		GitHubToken: "secret-oauth-token",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret-oauth-token") || strings.Contains(string(data), "GitHubToken") {
		t.Fatalf("record JSON leaked GitHub token: %s", data)
	}
}

func TestPublishOrchestrationBranch_CommitsAndPushes(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "remote.git")
	if err := runTestGit("", "init", "--bare", remote); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "work")
	if err := runTestGit("", "clone", remote, repo); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	record := &orchestrationRecord{
		ID:          "run-0123456789abcdef",
		RepoPath:    repo,
		GitHubToken: "oauth-token",
		GitHub: &orchestrationGitHubState{
			Repo:       "owner/repo",
			BranchName: "arun/run-0123456789abcdef",
			PRBase:     "main",
		},
	}
	if err := publishOrchestrationBranch(record); err != nil {
		t.Fatalf("publishOrchestrationBranch() error = %v", err)
	}
	if err := runTestGit(repo, "ls-remote", "--exit-code", "origin", "refs/heads/main"); err != nil {
		t.Fatalf("base branch was not pushed: %v", err)
	}
	if err := runTestGit(repo, "ls-remote", "--exit-code", "origin", "refs/heads/arun/run-0123456789abcdef"); err != nil {
		t.Fatalf("branch was not pushed: %v", err)
	}
	base, err := runTestGitOutput(remote, "rev-parse", "refs/heads/main")
	if err != nil {
		t.Fatal(err)
	}
	headParent, err := runTestGitOutput(remote, "rev-parse", "refs/heads/arun/run-0123456789abcdef^")
	if err != nil {
		t.Fatal(err)
	}
	if headParent != base {
		t.Fatalf("head parent = %s, want base %s", headParent, base)
	}
}

func TestNormalizeGitHubArtifactTitle_UsesFirstTaskLine(t *testing.T) {
	task := "Run an implementation-heavy agile scrum workflow for hakobune8/invaders on main.\n\nOperating mode: build-first."
	got := normalizeGitHubArtifactTitle("", task, "fallback")
	want := "Run an implementation-heavy agile scrum workflow for hakobune8/invaders on main."
	if got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
}

func TestNormalizeGitHubArtifactTitle_TruncatesExplicitTitle(t *testing.T) {
	got := normalizeGitHubArtifactTitle(strings.Repeat("a", 300), "task", "fallback")
	if len([]rune(got)) != 256 {
		t.Fatalf("title length = %d, want 256", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("title = %q, want ellipsis suffix", got)
	}
}

func runTestGit(dir string, args ...string) error {
	_, err := runTestGitCombinedOutput(dir, args...)
	return err
}

func runTestGitOutput(dir string, args ...string) (string, error) {
	out, err := runTestGitCombinedOutput(dir, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runTestGitCombinedOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func TestReadOrchestrationRecord_RejectsInvalidID(t *testing.T) {
	t.Setenv("ARUN_HOME", shortTestDir(t))
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
	if !strings.Contains(body, "ARUN") {
		t.Error("index.html does not contain 'ARUN'")
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
	t.Setenv("ARUN_AUTH_REQUIRED", "true")
	t.Setenv("ARUN_SESSION_SECRET", "test-secret")
	s := NewServer(0)

	protected := []string{
		"/api/runs",
		"/api/search?q=test",
		"/api/github/issues?repo=owner/repo",
		"/api/orchestrates",
		"/api/schedules",
		"/api/schedules/templates",
		"/api/notifications",
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
	t.Setenv("ARUN_HOME", home)

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
	t.Setenv("ARUN_HOME", home)
	t.Setenv("ARUN_AUTH_REQUIRED", "true")
	t.Setenv("ARUN_SESSION_SECRET", "test-secret")
	t.Setenv("ARUN_ADMIN_USERS", "admin")
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
	t.Setenv("ARUN_HOME", home)
	t.Setenv("ARUN_AUTH_REQUIRED", "true")
	t.Setenv("ARUN_SESSION_SECRET", "test-secret")
	t.Setenv("ARUN_ADMIN_USERS", "admin")
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

func TestServer_ScheduleCRUDPauseResume(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARUN_HOME", home)
	s := NewServer(0)

	body := []byte(`{
		"name":"Nightly report",
		"repo":".",
		"baseBranch":"main",
		"task":"Create a repository health report",
		"templateId":"weekly-repository-health-report",
		"agents":["reporter"],
		"strategy":"sequential",
		"schedule":{"type":"interval","interval":"1h","timezone":"UTC"},
		"concurrencyPolicy":"forbid"
	}`)
	w := serveRequest(s, "POST", "/api/schedules", body)
	assertStatus(t, w.Code, http.StatusOK)
	var created scheduleDefinition
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if !isValidScheduleID(created.ID) || created.Status != scheduleStatusActive || created.NextRunAt.IsZero() {
		t.Fatalf("created schedule = %+v", created)
	}
	if created.TemplateID != "weekly-repository-health-report" {
		t.Fatalf("TemplateID = %q", created.TemplateID)
	}

	w = serveRequest(s, "POST", "/api/schedules/"+created.ID+"/pause", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var paused scheduleDefinition
	if err := json.Unmarshal(w.Body.Bytes(), &paused); err != nil {
		t.Fatal(err)
	}
	if paused.Status != scheduleStatusPaused {
		t.Fatalf("paused status = %q", paused.Status)
	}

	w = serveRequest(s, "POST", "/api/schedules/"+created.ID+"/resume", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var resumed scheduleDefinition
	if err := json.Unmarshal(w.Body.Bytes(), &resumed); err != nil {
		t.Fatal(err)
	}
	if resumed.Status != scheduleStatusActive || resumed.NextRunAt.IsZero() {
		t.Fatalf("resumed schedule = %+v", resumed)
	}

	w = serveRequest(s, "GET", "/api/schedules", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var schedules []scheduleDefinition
	if err := json.Unmarshal(w.Body.Bytes(), &schedules); err != nil {
		t.Fatal(err)
	}
	if len(schedules) != 1 || schedules[0].ID != created.ID {
		t.Fatalf("schedules = %+v", schedules)
	}

	w = serveRequest(s, "DELETE", "/api/schedules/"+created.ID, nil)
	assertStatus(t, w.Code, http.StatusNoContent)
	w = serveRequest(s, "GET", "/api/schedules", nil)
	assertStatus(t, w.Code, http.StatusOK)
	if err := json.Unmarshal(w.Body.Bytes(), &schedules); err != nil {
		t.Fatal(err)
	}
	if len(schedules) != 0 {
		t.Fatalf("schedules after delete = %+v, want none", schedules)
	}
}

func TestServer_ScheduleTemplates(t *testing.T) {
	t.Setenv("ARUN_HOME", t.TempDir())
	s := NewServer(0)

	w := serveRequest(s, "GET", "/api/schedules/templates", nil)
	assertStatus(t, w.Code, http.StatusOK)
	var templates []scheduledWorkflowTemplate
	if err := json.Unmarshal(w.Body.Bytes(), &templates); err != nil {
		t.Fatal(err)
	}
	if len(templates) < 3 {
		t.Fatalf("templates = %d, want at least 3", len(templates))
	}
	byID := map[string]scheduledWorkflowTemplate{}
	for _, template := range templates {
		byID[template.ID] = template
		if len(template.Agents) == 0 || len(template.ExpectedOutputs) == 0 || template.Schedule.Type == "" {
			t.Fatalf("incomplete template: %+v", template)
		}
	}
	for _, id := range []string{"daily-failed-run-report", "weekly-repository-health-report", "weekly-security-triage"} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing template %s in %+v", id, byID)
		}
	}
}

func TestScheduleNextRunCronTimezone(t *testing.T) {
	t.Setenv("ARUN_HOME", t.TempDir())
	schedule := &scheduleDefinition{
		Name:      "weekday cron",
		Repo:      ".",
		Task:      "report",
		Agents:    []string{"reporter"},
		Strategy:  "sequential",
		Schedule:  scheduleSpec{Type: "cron", Cron: "30 9 * * *", Timezone: "Asia/Tokyo"},
		CreatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		Status:    scheduleStatusActive,
		NextRunAt: time.Time{},
	}
	next, err := nextScheduleRun(schedule, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 7, 1, 0, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next = %s, want %s", next, want)
	}
}

func TestServer_RunDueSchedulesSkipsActiveForbidConcurrency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARUN_HOME", home)
	s := NewServer(0)

	now := time.Date(2026, 7, 1, 1, 0, 0, 0, time.UTC)
	activeRun := &orchestrationRecord{
		ID:         "run-1111111111111111",
		Actor:      "system",
		Repo:       ".",
		BaseBranch: "main",
		Task:       "active",
		Status:     "running",
		CreatedAt:  now.Add(-time.Hour),
		UpdatedAt:  now.Add(-time.Minute),
	}
	if err := saveOrchestrationRecord(activeRun); err != nil {
		t.Fatal(err)
	}
	schedule := &scheduleDefinition{
		ID:                "schedule-2222222222222222",
		Actor:             "system",
		Name:              "due",
		Status:            scheduleStatusActive,
		Repo:              ".",
		BaseBranch:        "main",
		Task:              "report",
		Agents:            []string{"reporter"},
		Strategy:          "sequential",
		Schedule:          scheduleSpec{Type: "interval", Interval: "1h", Timezone: "UTC"},
		ConcurrencyPolicy: schedulePolicyForbid,
		NextRunAt:         now.Add(-time.Minute),
		LastRunID:         activeRun.ID,
		LastRunStatus:     activeRun.Status,
		CreatedAt:         now.Add(-2 * time.Hour),
		UpdatedAt:         now.Add(-time.Hour),
	}
	if err := saveSchedule(schedule); err != nil {
		t.Fatal(err)
	}

	s.runDueSchedules(now, "scheduled")

	reloaded, err := readSchedule(schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Executions) != 1 || reloaded.Executions[0].Status != "skipped" {
		t.Fatalf("executions = %+v", reloaded.Executions)
	}
	if !reloaded.NextRunAt.After(now) {
		t.Fatalf("next run = %s, want after %s", reloaded.NextRunAt, now)
	}
}

func TestServer_ScheduledRunNotificationWebhookRetriesAndStoresHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARUN_HOME", home)
	t.Setenv("ARUN_PUBLIC_URL", "https://arun.example.com")
	s := NewServer(0)

	attempts := 0
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var notification notificationRecord
		if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
			t.Fatalf("decode notification: %v", err)
		}
		if notification.Trigger != notificationTriggerFailed || notification.RunID != "run-3333333333333333" {
			t.Fatalf("notification = %+v", notification)
		}
		if attempts < 3 {
			http.Error(w, "temporary failure", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hook.Close()

	schedule := &scheduleDefinition{
		ID:                "schedule-2222222222222222",
		Actor:             "system",
		Name:              "Failure alerts",
		Status:            scheduleStatusActive,
		Repo:              "owner/repo",
		BaseBranch:        "main",
		Task:              "report",
		Agents:            []string{"reporter"},
		Strategy:          "sequential",
		Schedule:          scheduleSpec{Type: "interval", Interval: "1h", Timezone: "UTC"},
		ConcurrencyPolicy: schedulePolicyForbid,
		Notification: scheduleNotification{
			Enabled:      true,
			Triggers:     []string{notificationTriggerFailed},
			Destinations: []string{notificationDestinationInbox, notificationDestinationWebhook},
			WebhookURL:   hook.URL,
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := saveSchedule(schedule); err != nil {
		t.Fatal(err)
	}
	record := &orchestrationRecord{
		ID:         "run-3333333333333333",
		ScheduleID: schedule.ID,
		Repo:       "owner/repo",
		BaseBranch: "main",
		Task:       "report",
		Status:     "failed",
		Error:      "execute: boom",
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	s.notifyScheduledRun(record)

	if attempts != 3 {
		t.Fatalf("webhook attempts = %d, want 3", attempts)
	}
	notifications, err := listNotificationRecords(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 {
		t.Fatalf("notifications = %+v", notifications)
	}
	got := notifications[0]
	if got.Trigger != notificationTriggerFailed || got.ScheduleID != schedule.ID || got.RunURL != "https://arun.example.com/#orchestrates/run-3333333333333333" {
		t.Fatalf("notification = %+v", got)
	}
	if len(got.Deliveries) != 2 || got.Deliveries[0].Status != notificationDeliverySuccess || got.Deliveries[1].Status != notificationDeliverySuccess || got.Deliveries[1].Attempts != 3 {
		t.Fatalf("deliveries = %+v", got.Deliveries)
	}

	s.notifyScheduledRun(record)
	notifications, err = listNotificationRecords(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 {
		t.Fatalf("duplicate notifications = %+v", notifications)
	}
}

func TestServer_ScheduleNotificationFailureDeliveryHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ARUN_HOME", home)
	s := NewServer(0)

	schedule := &scheduleDefinition{
		ID:     "schedule-2222222222222222",
		Name:   "Failure alerts",
		Repo:   "owner/repo",
		Status: scheduleStatusActive,
		Notification: scheduleNotification{
			Enabled:      true,
			Triggers:     []string{notificationTriggerFailed},
			Destinations: []string{notificationDestinationWebhook},
		},
	}
	execution := schedule.newExecution(time.Now().UTC(), notificationTriggerFailed, "scheduled", "missing webhook")

	s.notifyScheduleExecution(schedule, &execution)

	notifications, err := listNotificationRecords(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 1 {
		t.Fatalf("notifications = %+v", notifications)
	}
	if len(notifications[0].Deliveries) != 1 || notifications[0].Deliveries[0].Status != notificationDeliveryFailure || !strings.Contains(notifications[0].Deliveries[0].Error, "webhook URL") {
		t.Fatalf("deliveries = %+v", notifications[0].Deliveries)
	}

	w := serveRequest(s, "DELETE", "/api/notifications/"+notifications[0].ID, nil)
	assertStatus(t, w.Code, http.StatusNoContent)
	notifications, err = listNotificationRecords(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifications) != 0 {
		t.Fatalf("notifications after delete = %+v, want none", notifications)
	}
}

func TestServer_LLMSettingsDoesNotExposeSecret(t *testing.T) {
	t.Setenv("LITELLM_BASE_URL", "http://litellm.test")
	t.Setenv("LITELLM_API_KEY", "secret-key")
	t.Setenv("ARUN_MODEL_CODER", "coder-test")
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
		{"docker", "Update Dockerfile healthcheck", []string{"ops"}, "ops", []string{"devops", "docker", "helm", "kubernetes", "release-manager", "security", "qa", "reviewer"}},
		{"helm", "Fix Helm chart values", nil, "ops", []string{"devops", "docker", "helm", "kubernetes", "release-manager", "security", "qa", "reviewer"}},
		{"kubernetes", "Fix Kubernetes ingress deployment", nil, "ops", []string{"devops", "docker", "helm", "kubernetes", "release-manager", "security", "qa", "reviewer"}},
		{"ops-over-security-signal", "Fix Helm chart values and Kubernetes ingress deployment", []string{"security"}, "ops", []string{"devops", "docker", "helm", "kubernetes"}},
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

/arun run agents=docs,reviewer strategy=parallel create_pr=false close_policy=after_human_approval approval=true`
	got := issueTriggerControls([]string{"arun:create-pr"}, body)
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
		Repo:        "hakobune8/arun",
		BaseBranch:  "main",
		IssueNumber: 203,
		IssueTitle:  "Fix GitHub Actions workflow check failed",
		IssueBody:   "CI is failing on lint.",
		IssueURL:    "https://github.com/hakobune8/arun/issues/203",
		Labels:      []string{"arun:run", "arun:create-pr"},
	}, agent.DefaultRegistry())
	if err != nil {
		t.Fatalf("orchestrationRequestFromIssue() error = %v", err)
	}
	if source.Number != 203 || source.URL == "" {
		t.Fatalf("source = %+v", source)
	}
	if req.Repo != "hakobune8/arun" || req.BaseBranch != "main" {
		t.Fatalf("repo/base = %q/%q", req.Repo, req.BaseBranch)
	}
	if !strings.Contains(req.Task, "GitHub Issue #203") || !strings.Contains(req.Task, "CI is failing") {
		t.Fatalf("Task = %q", req.Task)
	}
	if strings.Join(req.Agents, ",") != "ci-fixer,reviewer" {
		t.Fatalf("Agents = %+v, want ci-fixer/reviewer", req.Agents)
	}
	if req.GitHub == nil || !req.GitHub.CreatePullRequest || req.GitHub.BranchName != "arun/issue-203" {
		t.Fatalf("GitHub = %+v", req.GitHub)
	}
	if source.ClosePolicy != "on_pr_merge" {
		t.Fatalf("ClosePolicy = %q, want on_pr_merge", source.ClosePolicy)
	}
}

func TestFindDuplicateIssueOrchestration_ActiveIssueAndTrigger(t *testing.T) {
	t.Setenv("ARUN_HOME", shortTestDir(t))
	now := time.Now().UTC()
	record := &orchestrationRecord{
		ID:         "run-0123456789abcdef",
		Repo:       "hakobune8/arun",
		BaseBranch: "main",
		Task:       "issue task",
		Agents:     []string{"go-backend"},
		Strategy:   "sequential",
		Status:     "running",
		GitHub: &orchestrationGitHubState{
			Repo:              "hakobune8/arun",
			SourceIssueNumber: 203,
			SourceTriggerID:   "delivery-1",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := saveOrchestrationRecord(record); err != nil {
		t.Fatal(err)
	}
	if got, ok := findDuplicateIssueOrchestration("hakobune8/arun", &orchestrationSourceIssue{Number: 203}); !ok || got.ID != record.ID {
		t.Fatalf("duplicate by issue = %+v/%v, want %s", got, ok, record.ID)
	}
	if got, ok := findDuplicateIssueOrchestration("hakobune8/arun", &orchestrationSourceIssue{Number: 999, TriggerID: "delivery-1"}); !ok || got.ID != record.ID {
		t.Fatalf("duplicate by trigger = %+v/%v, want %s", got, ok, record.ID)
	}
}

func TestSourceIssueCommentBodies(t *testing.T) {
	t.Setenv("ARUN_PUBLIC_URL", "https://arun.example.com")
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
	if !strings.Contains(start, "ARUN orchestration started") ||
		!strings.Contains(start, "https://arun.example.com/#orchestrates/run-0123456789abcdef") ||
		!strings.Contains(start, "ci-fixer, reviewer") {
		t.Fatalf("start comment = %q", start)
	}
	final := sourceIssueFinalCommentBody(record)
	if !strings.Contains(final, "ARUN orchestration finished") ||
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
	t.Setenv("ARUN_HOME", home)
	t.Setenv("ARUN_AUTH_REQUIRED", "true")
	t.Setenv("ARUN_SESSION_SECRET", "test-secret")
	t.Setenv("ARUN_ADMIN_USERS", "admin")
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
	t.Setenv("ARUN_HOME", home)
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
