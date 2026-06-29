package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewServer_ReturnsServer(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	if s == nil {
		t.Fatal("NewServer returned nil")
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
	w := serveRequest(s, "GET", "/api/runs/nonexistent", nil)
	assertStatus(t, w.Code, http.StatusOK) // returns empty artifacts, not error
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if id, ok := resp["id"]; !ok || id != "nonexistent" {
		t.Errorf("id = %v, want nonexistent", id)
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

func TestResolveOrchestrateRepo_LocalPath(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()

	got, err := resolveOrchestrateRepo(repo, "")
	if err != nil {
		t.Fatalf("resolveOrchestrateRepo() error = %v", err)
	}
	if got != repo {
		t.Fatalf("repo = %q, want %q", got, repo)
	}
}

func TestResolveOrchestrateRepo_RemoteFileClone(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	t.Setenv("AGENTOS_HOME", shortTestDir(t))

	source := filepath.Join(root, "source")
	remote := filepath.Join(root, "remote.git")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, source, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("# scenario\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitCommand(t, source, "add", "README.md")
	runGitCommand(t, source, "-c", "user.name=AgentOS", "-c", "user.email=agentos@example.local", "commit", "-m", "init")
	runGitCommand(t, root, "clone", "--bare", source, remote)

	got, err := resolveOrchestrateRepo("file://"+remote, "main")
	if err != nil {
		t.Fatalf("resolveOrchestrateRepo() error = %v", err)
	}
	if !strings.Contains(got, filepath.Join("workspaces", "orchestrate")) {
		t.Fatalf("repo = %q, want cloned workspace under AGENTOS_HOME", got)
	}
	if _, err := os.Stat(filepath.Join(got, "README.md")); err != nil {
		t.Fatalf("cloned README.md missing: %v", err)
	}
}

func TestResolveOrchestrateRepo_EmptyRemoteFallsBackWithoutBranch(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	t.Setenv("AGENTOS_HOME", shortTestDir(t))

	remote := filepath.Join(root, "empty.git")
	runGitCommand(t, root, "init", "--bare", remote)

	got, err := resolveOrchestrateRepo("file://"+remote, "main")
	if err != nil {
		t.Fatalf("resolveOrchestrateRepo() error = %v", err)
	}
	if !strings.Contains(got, filepath.Join("workspaces", "orchestrate")) {
		t.Fatalf("repo = %q, want cloned workspace under AGENTOS_HOME", got)
	}
	if _, err := os.Stat(filepath.Join(got, ".git")); err != nil {
		t.Fatalf("cloned .git missing: %v", err)
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
		"kazyamaz200/agentos":               "https://github.com/kazyamaz200/agentos.git",
		"https://github.com/owner/repo.git": "https://github.com/owner/repo.git",
		"git@github.com:owner/repo.git":     "git@github.com:owner/repo.git",
		"file:///tmp/repo.git":              "file:///tmp/repo.git",
		"/workspace/scenario-repo":          "",
		"relative-repo":                     "",
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
	if !strings.Contains(body, "Dashboard") {
		t.Error("index.html does not contain 'Dashboard'")
	}
}

func TestServer_IndexHTML_HasAllNavLinks(t *testing.T) {
	t.Parallel()
	s := NewServer(0)
	w := serveRequest(s, "GET", "/", nil)
	body := w.Body.String()
	links := []string{"Dashboard", "Runs", "Agents", "Search", "GitHub", "Orchestrate", "New Run"}
	for _, link := range links {
		if !strings.Contains(body, link) {
			t.Errorf("index.html missing nav link: %s", link)
		}
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

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func runGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
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
