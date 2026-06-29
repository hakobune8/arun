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

package server

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/apphome"
	"github.com/kazyamaz200/agentos/internal/embedding"
	agentosgh "github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/orchestrator"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/search"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/kazyamaz200/agentos/internal/vector"
)

//go:embed static
var staticFS embed.FS

var runIDPattern = regexp.MustCompile(`^run-[0-9a-f]{16}$`)

// Server serves the AgentOS web UI and API endpoints.
type Server struct {
	port        int
	server      *http.Server
	search      *search.Service
	agentReg    *agent.Registry
	llmClient   llm.LLMClient
	sandbox     sandbox.Sandbox
	runtimeCfg  *runtime.Config
	auth        authConfig
	llmSettings llmSettings
}

// NewServer creates a new Server listening on the given port.
func NewServer(port int) *Server {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	svc := search.NewService(vs, emb)

	llmCfg := llm.DefaultConfig()
	llmClient := llm.NewLiteLLMClient(llmCfg)
	authCfg := loadAuthConfig()
	llmSettings := loadLLMSettings()

	mux := http.NewServeMux()
	s := &Server{
		port:        port,
		search:      svc,
		agentReg:    agent.DefaultRegistry(),
		llmClient:   llmClient,
		sandbox:     sandbox.NewLocalSandbox("."),
		runtimeCfg:  &runtime.Config{Verbose: false},
		auth:        authCfg,
		llmSettings: llmSettings,
	}

	mux.HandleFunc("/api/auth/session", s.handleAuthSession)
	mux.HandleFunc("/auth/login", s.handleAuthLogin)
	mux.HandleFunc("/auth/callback", s.handleAuthCallback)
	mux.HandleFunc("/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/settings/llm", s.handleLLMSettings)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunDetail)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/github/", s.handleGitHub)
	mux.HandleFunc("/api/orchestrate", s.handleOrchestrate)
	mux.HandleFunc("/api/orchestrates", s.handleOrchestrates)
	mux.HandleFunc("/api/orchestrates/", s.handleOrchestrateDetail)

	staticSub, err := fs.Sub(staticFS, "static")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(staticSub)))
	}

	s.server = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           corsMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// Start starts the HTTP server and blocks until Shutdown is called.
func (s *Server) Start() error {
	slog.Info("AgentOS Web UI starting", "port", s.port)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// --- Health ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // best-effort
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// --- Agents ---

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	agents := s.agentReg.List()
	_ = json.NewEncoder(w).Encode(agents) //nolint:errcheck // best-effort
}

// --- Settings ---

func (s *Server) handleLLMSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.llmSettings) //nolint:errcheck // best-effort
}

// --- Runs ---

type createRunRequest struct {
	Agent       string `json:"agent"`
	Task        string `json:"task"`
	Description string `json:"description"`
	Repo        string `json:"repo"`
	LLMPreset   string `json:"llmPreset"`
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listRuns(w, r)
	case http.MethodPost:
		s.createRun(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	runsDir := apphome.RunsDir()

	entries, err := os.ReadDir(runsDir)
	if err != nil {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{}) //nolint:errcheck // empty list
		return
	}

	var runs []map[string]interface{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		run := map[string]interface{}{
			"id": entry.Name(),
		}
		stateFile := filepath.Join(runsDir, entry.Name(), "run_state.json")
		if data, err := os.ReadFile(stateFile); err == nil {
			var state map[string]interface{}
			if json.Unmarshal(data, &state) == nil {
				run["state"] = state
			}
		}
		runs = append(runs, run)
	}

	if runs == nil {
		runs = []map[string]interface{}{}
	}
	_ = json.NewEncoder(w).Encode(runs) //nolint:errcheck // best-effort
}

func (s *Server) createRun(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req createRunRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Agent == "" || req.Task == "" {
		http.Error(w, "agent and task are required", http.StatusBadRequest)
		return
	}

	llmClient, presetID, err := s.llmClientForPreset(req.LLMPreset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	agt, err := s.agentReg.Create(req.Agent, llmClient)
	if err != nil {
		http.Error(w, "lookup agent: "+err.Error(), http.StatusBadRequest)
		return
	}

	id := generateID()
	repo := req.Repo
	if repo == "" {
		repo = "."
	}

	tk := &task.Task{
		ID:          id,
		Type:        "issue_to_patch",
		Repo:        repo,
		BaseBranch:  "main",
		Branch:      "agentos/" + id,
		Title:       req.Task,
		Description: req.Description + "\n\nLLM preset: " + presetID,
	}

	sb := sandbox.NewLocalSandbox(repo)
	cfg := &runtime.Config{Verbose: false}
	prof := &profile.Profile{Name: req.Agent}

	rt := runtime.NewRuntime(llmClient, prof, sb, cfg, agt)

	go func() {
		if err := rt.Run(context.Background(), tk); err != nil {
			slog.Warn("async run failed", "id", id, "error", err)
		}
	}()

	_ = json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // best-effort
		"id":        id,
		"status":    "started",
		"llmPreset": presetID,
	})
}

// --- Run Detail ---

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	id := filepath.Base(r.URL.Path)

	runDir := filepath.Join(apphome.RunsDir(), id)

	artifacts := []string{
		"task.yaml", "profile.yaml", "plan.json",
		"summary.md", "pr_body.md", "diff.patch",
		"test.log", "lint.log", "run_state.json",
	}

	result := map[string]interface{}{
		"id":        id,
		"artifacts": map[string]string{},
	}

	for _, name := range artifacts {
		path := filepath.Join(runDir, name)
		if data, err := os.ReadFile(path); err == nil {
			result["artifacts"].(map[string]string)[name] = string(data)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result) //nolint:errcheck // best-effort
}

// --- Search ---

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query param q required", http.StatusBadRequest)
		return
	}

	source := r.URL.Query().Get("source")
	searchType := search.TypeAll
	switch source {
	case "memory":
		searchType = search.TypeMemory
	case "guideline":
		searchType = search.TypeGuideline
	case "pr":
		searchType = search.TypePR
	}

	results, err := s.search.Search(r.Context(), query, searchType, 50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results) //nolint:errcheck // best-effort
}

// --- GitHub ---

func (s *Server) handleGitHub(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	repo := r.URL.Query().Get("repo")
	if repo == "" {
		http.Error(w, "repo query param required", http.StatusBadRequest)
		return
	}
	parts := splitRepo(repo)
	if len(parts) != 2 {
		http.Error(w, "repo must be owner/name", http.StatusBadRequest)
		return
	}
	client := agentosgh.NewClient(parts[0], parts[1])

	path := r.URL.Path[len("/api/github/"):]
	switch path {
	case "issues":
		state := r.URL.Query().Get("state")
		if state == "" {
			state = "open"
		}
		issues, err := client.ListIssues(state)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if issues == nil {
			issues = []agentosgh.Issue{}
		}
		_ = json.NewEncoder(w).Encode(issues) //nolint:errcheck // best-effort response

	case "pulls":
		state := r.URL.Query().Get("state")
		prs, err := client.ListPRs(state)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if prs == nil {
			prs = []agentosgh.PullRequest{}
		}
		_ = json.NewEncoder(w).Encode(prs) //nolint:errcheck // best-effort response

	case "checks":
		ref := r.URL.Query().Get("ref")
		if ref == "" {
			ref = "main"
		}
		runs, err := client.GetCheckRuns(ref)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if runs == nil {
			runs = []agentosgh.CheckRun{}
		}
		_ = json.NewEncoder(w).Encode(runs) //nolint:errcheck // best-effort response

	default:
		http.Error(w, "unknown github resource: "+path, http.StatusNotFound)
	}
}

// --- Orchestrate ---

type orchestrateRequest struct {
	Agents     []string `json:"agents"`
	Repo       string   `json:"repo"`
	BaseBranch string   `json:"baseBranch"`
	Task       string   `json:"task"`
	Strategy   string   `json:"strategy"`
	LLMPreset  string   `json:"llmPreset"`
}

type orchestrationRecord struct {
	ID         string                       `json:"id"`
	Repo       string                       `json:"repo"`
	RepoPath   string                       `json:"repoPath,omitempty"`
	BaseBranch string                       `json:"baseBranch"`
	Task       string                       `json:"task"`
	Agents     []string                     `json:"agents"`
	Strategy   string                       `json:"strategy"`
	LLMPreset  string                       `json:"llmPreset"`
	Status     string                       `json:"status"`
	Error      string                       `json:"error,omitempty"`
	Plan       *orchestrator.TaskPlan       `json:"plan,omitempty"`
	Subtasks   []orchestrationSubtaskState  `json:"subtasks,omitempty"`
	Results    []orchestrator.SubtaskResult `json:"results,omitempty"`
	Summary    string                       `json:"summary,omitempty"`
	CreatedAt  time.Time                    `json:"createdAt"`
	UpdatedAt  time.Time                    `json:"updatedAt"`
}

type orchestrationSubtaskState struct {
	ID          string                      `json:"id"`
	Description string                      `json:"description"`
	AgentName   string                      `json:"agent_type"`
	Status      string                      `json:"status"`
	StartedAt   *time.Time                  `json:"startedAt,omitempty"`
	FinishedAt  *time.Time                  `json:"finishedAt,omitempty"`
	Result      *orchestrator.SubtaskResult `json:"result,omitempty"`
}

func (s *Server) handleOrchestrate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req orchestrateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}

	req.Repo = strings.TrimSpace(req.Repo)
	req.BaseBranch = defaultBaseBranch(req.BaseBranch)
	req.Strategy = strings.TrimSpace(req.Strategy)
	if req.Strategy == "" {
		req.Strategy = "sequential"
	}
	if req.Strategy != "sequential" && req.Strategy != "parallel" {
		http.Error(w, "strategy must be sequential or parallel", http.StatusBadRequest)
		return
	}

	if len(req.Agents) == 0 || req.Task == "" {
		http.Error(w, "agents and task are required", http.StatusBadRequest)
		return
	}

	llmClient, presetID, err := s.llmClientForPreset(req.LLMPreset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repoPath, err := resolveOrchestrateRepo(req.Repo, req.BaseBranch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	agents := make(map[string]runtime.Agent)
	for _, name := range req.Agents {
		a, err := s.agentReg.Create(name, llmClient)
		if err != nil {
			http.Error(w, "lookup agent "+name+": "+err.Error(), http.StatusBadRequest)
			return
		}
		agents[name] = a
	}

	now := time.Now().UTC()
	record := &orchestrationRecord{
		ID:         generateID(),
		Repo:       req.Repo,
		RepoPath:   repoPath,
		BaseBranch: req.BaseBranch,
		Task:       req.Task,
		Agents:     req.Agents,
		Strategy:   req.Strategy,
		LLMPreset:  presetID,
		Status:     "planning",
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if record.Repo == "" {
		record.Repo = "."
	}
	if err := saveOrchestrationRecord(record); err != nil {
		http.Error(w, "save orchestration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	go s.runOrchestration(record, agents, llmClient)

	_ = json.NewEncoder(w).Encode(record) //nolint:errcheck // best-effort response
}

func (s *Server) runOrchestration(record *orchestrationRecord, agents map[string]runtime.Agent, llmClient llm.LLMClient) {
	cfg := &runtime.Config{Verbose: false}
	orch := orchestrator.NewOrchestrator(llmClient, sandbox.NewLocalSandbox(record.RepoPath), agents, cfg)
	orch.SetBaseBranch(record.BaseBranch)

	if record.Strategy == "parallel" {
		orch.SetStrategy(orchestrator.StrategyParallel)
	}

	plan, err := orch.Plan(context.Background(), record.Task)
	if err != nil {
		record.Status = "failed"
		record.Error = "plan: " + err.Error()
		record.UpdatedAt = time.Now().UTC()
		if saveErr := saveOrchestrationRecord(record); saveErr != nil {
			slog.Warn("save orchestration failed", "id", record.ID, "error", saveErr)
		}
		return
	}
	record.Plan = plan
	record.Subtasks = makeSubtaskStates(plan)
	record.Status = "running"
	record.UpdatedAt = time.Now().UTC()
	if err := saveOrchestrationRecord(record); err != nil {
		slog.Warn("save orchestration failed", "id", record.ID, "error", err)
	}

	orch.SetSubtaskTimeout(orchestrateSubtaskTimeout())
	var mu sync.Mutex
	observer := func(event orchestrator.SubtaskEvent) {
		mu.Lock()
		defer mu.Unlock()
		applySubtaskEvent(record, &event)
		record.UpdatedAt = time.Now().UTC()
		if err := saveOrchestrationRecord(record); err != nil {
			slog.Warn("save orchestration failed", "id", record.ID, "error", err)
		}
	}

	results, err := orch.ExecuteWithObserver(context.Background(), plan, observer)
	if err != nil {
		mu.Lock()
		defer mu.Unlock()
		record.Status = "failed"
		record.Error = "execute: " + err.Error()
		record.Results = results
		record.UpdatedAt = time.Now().UTC()
		if saveErr := saveOrchestrationRecord(record); saveErr != nil {
			slog.Warn("save orchestration failed", "id", record.ID, "error", saveErr)
		}
		return
	}

	summary := orch.MergeResults(results)
	mu.Lock()
	defer mu.Unlock()
	record.Results = results
	record.Summary = summary
	record.Status = "completed"
	record.UpdatedAt = time.Now().UTC()
	if err := saveOrchestrationRecord(record); err != nil {
		slog.Warn("save orchestration failed", "id", record.ID, "error", err)
	}
}

func (s *Server) handleOrchestrates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	records, err := listOrchestrationRecords()
	if err != nil {
		http.Error(w, "list orchestrations: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(records) //nolint:errcheck // best-effort response
}

func (s *Server) handleOrchestrateDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	id := filepath.Base(r.URL.Path)
	record, err := readOrchestrationRecord(id)
	if err != nil {
		http.Error(w, "orchestration not found: "+id, http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(record) //nolint:errcheck // best-effort response
}

func makeSubtaskStates(plan *orchestrator.TaskPlan) []orchestrationSubtaskState {
	if plan == nil || len(plan.Subtasks) == 0 {
		return []orchestrationSubtaskState{}
	}
	states := make([]orchestrationSubtaskState, 0, len(plan.Subtasks))
	for _, subtask := range plan.Subtasks {
		states = append(states, orchestrationSubtaskState{
			ID:          subtask.ID,
			Description: subtask.Description,
			AgentName:   subtask.AgentName,
			Status:      "pending",
		})
	}
	return states
}

func applySubtaskEvent(record *orchestrationRecord, event *orchestrator.SubtaskEvent) {
	if len(record.Subtasks) == 0 && record.Plan != nil {
		record.Subtasks = makeSubtaskStates(record.Plan)
	}
	for i := range record.Subtasks {
		if record.Subtasks[i].ID != event.Subtask.ID {
			continue
		}
		switch event.Type {
		case orchestrator.SubtaskStarted:
			started := event.Started
			record.Subtasks[i].Status = "running"
			record.Subtasks[i].StartedAt = &started
		case orchestrator.SubtaskCompleted:
			finished := event.Finished
			record.Subtasks[i].FinishedAt = &finished
			record.Subtasks[i].Result = event.Result
			if event.Result != nil && event.Result.Success {
				record.Subtasks[i].Status = "completed"
			} else {
				record.Subtasks[i].Status = "failed"
			}
		}
		return
	}
}

func orchestrateSubtaskTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("AGENTOS_ORCHESTRATE_SUBTASK_TIMEOUT"))
	if raw == "" {
		return 10 * time.Minute
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return 10 * time.Minute
	}
	return timeout
}

func resolveOrchestrateRepo(repo, baseBranch string) (string, error) {
	if repo == "" {
		repo = "."
	}

	if cloneURL, ok := normalizeRemoteRepo(repo); ok {
		return cloneRemoteRepo(cloneURL, defaultBaseBranch(baseBranch))
	}

	abs, err := filepath.Abs(repo)
	if err != nil {
		return "", fmt.Errorf("resolve repo path: %w", err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("repo does not exist: %s", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo is not a directory: %s", abs)
	}
	return abs, nil
}

func orchestrationsDir() string {
	return filepath.Join(apphome.Dir(), "orchestrates")
}

func saveOrchestrationRecord(record *orchestrationRecord) error {
	if err := os.MkdirAll(orchestrationsDir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(orchestrationsDir(), record.ID+".json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readOrchestrationRecord(id string) (*orchestrationRecord, error) {
	if !isValidRunID(id) {
		return nil, fmt.Errorf("invalid orchestration id")
	}
	path := filepath.Join(orchestrationsDir(), id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record orchestrationRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func isValidRunID(id string) bool {
	return runIDPattern.MatchString(id)
}

func listOrchestrationRecords() ([]*orchestrationRecord, error) {
	entries, err := os.ReadDir(orchestrationsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []*orchestrationRecord{}, nil
		}
		return nil, err
	}

	records := make([]*orchestrationRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		record, err := readOrchestrationRecord(id)
		if err != nil {
			slog.Warn("skip unreadable orchestration record", "id", id, "error", err)
			continue
		}
		records = append(records, record)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records, nil
}

func normalizeRemoteRepo(repo string) (string, bool) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", false
	}

	if strings.HasPrefix(repo, "git@") {
		return repo, true
	}
	if strings.HasPrefix(repo, "http://") || strings.HasPrefix(repo, "https://") || strings.HasPrefix(repo, "file://") {
		return repo, true
	}
	if strings.Count(repo, "/") == 1 && !strings.HasPrefix(repo, ".") {
		parts := splitRepo(repo)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return "https://github.com/" + repo + ".git", true
		}
	}
	return "", false
}

func cloneRemoteRepo(cloneURL, baseBranch string) (string, error) {
	root := filepath.Join(apphome.Dir(), "workspaces", "orchestrate")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create workspace root: %w", err)
	}

	dest := filepath.Join(root, fmt.Sprintf("%s-%s-%s", time.Now().UTC().Format("20060102T150405"), generateID(), safeRepoSlug(cloneURL)))
	args := gitCloneArgs(cloneURL, baseBranch, dest)
	out, err := runGitClone(args)
	if err != nil && shouldRetryCloneWithoutBranch(string(out)) {
		_ = os.RemoveAll(dest)
		args = gitCloneArgs(cloneURL, "", dest)
		out, err = runGitClone(args)
	}
	if err != nil {
		return "", fmt.Errorf("clone repo: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return dest, nil
}

func runGitClone(args []string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = gitCloneEnv(args)
	return cmd.CombinedOutput()
}

func gitCloneArgs(cloneURL, baseBranch, dest string) []string {
	args := []string{"clone", "--depth=1"}
	if baseBranch != "" {
		args = append(args, "--branch", baseBranch)
	}
	args = append(args, cloneURL, dest)
	return args
}

func gitCloneEnv(args []string) []string {
	env := os.Environ()
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" || !cloneArgsUseGitHubHTTPS(args) {
		return env
	}

	auth := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return append(env,
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic "+auth,
	)
}

func cloneArgsUseGitHubHTTPS(args []string) bool {
	for _, arg := range args {
		if isGitHubHTTPSRepo(arg) {
			return true
		}
	}
	return false
}

func isGitHubHTTPSRepo(repo string) bool {
	u, err := url.Parse(repo)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, "github.com")
}

func defaultBaseBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "main"
	}
	return branch
}

func shouldRetryCloneWithoutBranch(output string) bool {
	output = strings.ToLower(output)
	return strings.Contains(output, "remote branch") && strings.Contains(output, "not found")
}

func safeRepoSlug(repo string) string {
	slug := repo
	if u, err := url.Parse(repo); err == nil && u.Path != "" {
		slug = strings.Trim(u.Path, "/")
	}
	slug = strings.TrimSuffix(slug, ".git")
	slug = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, slug)
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "repo"
	}
	if len(slug) > 24 {
		return slug[len(slug)-24:]
	}
	return slug
}

// --- Store ---

func newVectorStore() vector.VectorStore {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL != "" {
		return vector.NewQdrantClient()
	}
	return vector.NewLocalStore(apphome.VectorsDir())
}

// --- Helpers ---

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "run-" + hex.EncodeToString(b)
}

func splitRepo(repo string) []string {
	for i := 0; i < len(repo); i++ {
		if repo[i] == '/' {
			return []string{repo[:i], repo[i+1:]}
		}
	}
	return nil
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}
