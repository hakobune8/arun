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
	"bytes"
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"text/template"
	"time"

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/apphome"
	"github.com/kazyamaz200/agentos/internal/embedding"
	agentosgh "github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/orchestrator"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/safety"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/search"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/kazyamaz200/agentos/internal/vector"
	"gopkg.in/yaml.v3"
)

//go:embed static
var staticFS embed.FS

var runIDPattern = regexp.MustCompile(`^run-[0-9a-f]{16}$`)
var githubRepoPathPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)
var gitRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)

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
	activeMu    sync.Mutex
	activeRuns  map[string]context.CancelFunc
	canceledRun map[string]bool
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
		activeRuns:  map[string]context.CancelFunc{},
		canceledRun: map[string]bool{},
	}

	mux.HandleFunc("/api/auth/session", s.handleAuthSession)
	mux.HandleFunc("/auth/login", s.handleAuthLogin)
	mux.HandleFunc("/auth/callback", s.handleAuthCallback)
	mux.HandleFunc("/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/settings/llm", s.handleLLMSettings)
	mux.HandleFunc("/api/audit", s.handleAudit)
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

// --- Audit ---

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	if !s.requireAutomationPermission(w, r, user, "audit.read", "audit", "", "") {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	events, err := listAuditEvents(100)
	if err != nil {
		http.Error(w, "list audit: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(events) //nolint:errcheck // best-effort response
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
	if !isValidRunID(id) {
		http.Error(w, "invalid run id", http.StatusBadRequest)
		return
	}

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
		path, err := runArtifactPath(runDir, name)
		if err != nil {
			continue
		}
		if data, err := os.ReadFile(path); err == nil {
			result["artifacts"].(map[string]string)[name] = safety.NewRedactor().RedactString(string(data))
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
	Agents         []string                  `json:"agents"`
	Repo           string                    `json:"repo"`
	BaseBranch     string                    `json:"baseBranch"`
	Task           string                    `json:"task"`
	Strategy       string                    `json:"strategy"`
	LLMPreset      string                    `json:"llmPreset"`
	OutputLanguage string                    `json:"outputLanguage,omitempty"`
	GitHub         *orchestrateGitHubRequest `json:"github,omitempty"`
}

type orchestrateGitHubRequest struct {
	CreateIssue       bool   `json:"createIssue"`
	CreatePullRequest bool   `json:"createPullRequest"`
	BranchName        string `json:"branchName"`
	PRBase            string `json:"prBase"`
	IssueTitle        string `json:"issueTitle"`
	PRTitle           string `json:"prTitle"`
	IssueTemplate     string `json:"issueTemplate,omitempty"`
	PRTemplate        string `json:"prTemplate,omitempty"`
}

type orchestrationRecord struct {
	ID             string                       `json:"id"`
	Actor          string                       `json:"actor,omitempty"`
	Repo           string                       `json:"repo"`
	RepoPath       string                       `json:"repoPath,omitempty"`
	BaseBranch     string                       `json:"baseBranch"`
	Task           string                       `json:"task"`
	Agents         []string                     `json:"agents"`
	Strategy       string                       `json:"strategy"`
	LLMPreset      string                       `json:"llmPreset"`
	OutputLanguage string                       `json:"outputLanguage,omitempty"`
	Status         string                       `json:"status"`
	Error          string                       `json:"error,omitempty"`
	Plan           *orchestrator.TaskPlan       `json:"plan,omitempty"`
	Subtasks       []orchestrationSubtaskState  `json:"subtasks,omitempty"`
	Results        []orchestrator.SubtaskResult `json:"results,omitempty"`
	Events         []orchestrationEvent         `json:"events,omitempty"`
	Summary        string                       `json:"summary,omitempty"`
	GitHub         *orchestrationGitHubState    `json:"github,omitempty"`
	CreatedAt      time.Time                    `json:"createdAt"`
	UpdatedAt      time.Time                    `json:"updatedAt"`
}

type orchestrationEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	SubtaskID string    `json:"subtaskId,omitempty"`
	Message   string    `json:"message"`
}

type orchestrationGitHubState struct {
	Repo              string `json:"repo"`
	BranchName        string `json:"branchName,omitempty"`
	IssueTitle        string `json:"issueTitle,omitempty"`
	IssueTemplate     string `json:"issueTemplate,omitempty"`
	IssueURL          string `json:"issueUrl,omitempty"`
	IssueNumber       int    `json:"issueNumber,omitempty"`
	PRTitle           string `json:"prTitle,omitempty"`
	PRTemplate        string `json:"prTemplate,omitempty"`
	PRBase            string `json:"prBase,omitempty"`
	PullRequestURL    string `json:"pullRequestUrl,omitempty"`
	PullRequestNumber int    `json:"pullRequestNumber,omitempty"`
	Error             string `json:"error,omitempty"`
	CreateIssue       bool   `json:"createIssue,omitempty"`
	CreatePullRequest bool   `json:"createPullRequest,omitempty"`
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
	user, ok := s.requireAuth(w, r)
	if !ok {
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
	if !s.requireAutomationPermission(w, r, user, "orchestrate.create", "orchestration", req.Repo, "") {
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
	artifactConfig := loadArtifactConfig(repoPath)
	applyArtifactConfig(&req, artifactConfig)

	agents := make(map[string]runtime.Agent)
	for _, name := range req.Agents {
		a, err := s.agentReg.Create(name, llmClient)
		if err != nil {
			http.Error(w, "lookup agent "+name+": "+err.Error(), http.StatusBadRequest)
			return
		}
		agents[name] = a
	}

	id := generateID()
	githubState, err := prepareOrchestrationGitHub(id, &req)
	if err != nil {
		http.Error(w, "github: "+err.Error(), http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	record := &orchestrationRecord{
		ID:             id,
		Actor:          actorLogin(user),
		Repo:           req.Repo,
		RepoPath:       repoPath,
		BaseBranch:     req.BaseBranch,
		Task:           req.Task,
		Agents:         req.Agents,
		Strategy:       req.Strategy,
		LLMPreset:      presetID,
		OutputLanguage: normalizeOutputLanguage(req.OutputLanguage),
		Status:         "planning",
		GitHub:         githubState,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	appendOrchestrationEvent(record, "created", "", "Orchestration created")
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
	runCtx, cancelRun := context.WithCancel(context.Background())
	s.registerActiveOrchestration(record.ID, cancelRun)
	defer s.unregisterActiveOrchestration(record.ID)
	defer cancelRun()

	cfg := &runtime.Config{Verbose: false}
	orch := orchestrator.NewOrchestrator(llmClient, sandbox.NewLocalSandbox(record.RepoPath), agents, cfg)
	orch.SetBaseBranch(record.BaseBranch)
	orch.SetRunID(record.ID)

	if record.Strategy == "parallel" {
		orch.SetStrategy(orchestrator.StrategyParallel)
	}

	s.createTrackingIssue(record)
	appendOrchestrationEvent(record, "planning.started", "", "Planning started")

	planCtx, cancelPlan := context.WithTimeout(runCtx, orchestratePlanTimeout())
	defer cancelPlan()
	plan, err := orch.Plan(planCtx, record.Task)
	if err != nil {
		if errors.Is(planCtx.Err(), context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
			s.stopCanceledOrchestration(record, "Orchestration canceled during planning")
			return
		} else {
			record.Status = "failed"
			record.Error = "plan: " + err.Error()
			appendOrchestrationEvent(record, "planning.failed", "", record.Error)
		}
		record.UpdatedAt = time.Now().UTC()
		if saveErr := saveOrchestrationRecord(record); saveErr != nil {
			slog.Warn("save orchestration failed", "id", record.ID, "error", saveErr)
		}
		s.auditOrchestrationOutcome(record, auditOutcomeFailure, record.Error)
		return
	}
	if s.stopCanceledOrchestration(record, "Orchestration canceled") {
		return
	}
	appendOrchestrationEvent(record, "planning.finished", "", fmt.Sprintf("Planning finished with %d subtasks", len(plan.Subtasks)))
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
		if s.isOrchestrationCanceled(record.ID) {
			return
		}
		applySubtaskEvent(record, &event)
		appendTimelineForSubtaskEvent(record, &event)
		record.UpdatedAt = time.Now().UTC()
		if err := saveOrchestrationRecord(record); err != nil {
			slog.Warn("save orchestration failed", "id", record.ID, "error", err)
		}
	}

	results, err := orch.ExecuteWithObserver(runCtx, plan, observer)
	if s.isOrchestrationCanceled(record.ID) && err == nil {
		err = context.Canceled
	}
	if err != nil {
		mu.Lock()
		defer mu.Unlock()
		if errors.Is(runCtx.Err(), context.Canceled) {
			s.stopCanceledOrchestration(record, "Orchestration canceled")
			return
		} else {
			record.Status = "failed"
			record.Error = "execute: " + err.Error()
			appendOrchestrationEvent(record, "execute.failed", "", record.Error)
		}
		record.Results = results
		record.UpdatedAt = time.Now().UTC()
		if saveErr := saveOrchestrationRecord(record); saveErr != nil {
			slog.Warn("save orchestration failed", "id", record.ID, "error", saveErr)
		}
		s.auditOrchestrationOutcome(record, auditOutcomeFailure, record.Error)
		return
	}

	summary := orch.MergeResults(results)
	mu.Lock()
	defer mu.Unlock()
	if s.stopCanceledOrchestration(record, "Orchestration canceled") {
		return
	}
	record.Results = results
	record.Summary = summary
	record.Status = "completed"
	appendOrchestrationEvent(record, "completed", "", "Orchestration completed")
	record.UpdatedAt = time.Now().UTC()
	if err := saveOrchestrationRecord(record); err != nil {
		slog.Warn("save orchestration failed", "id", record.ID, "error", err)
	}
	s.auditOrchestrationOutcome(record, auditOutcomeSuccess, "")
	s.createPullRequestForOrchestration(record)
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
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	if strings.HasSuffix(r.URL.Path, "/cancel") {
		s.handleOrchestrateCancel(w, r, user)
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

func (s *Server) handleOrchestrateCancel(w http.ResponseWriter, r *http.Request, user *authUser) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	id := filepath.Base(filepath.Dir(r.URL.Path))
	if !s.requireAutomationPermission(w, r, user, "orchestrate.cancel", "orchestration/"+id, "", id) {
		return
	}
	record, err := readOrchestrationRecord(id)
	if err != nil {
		http.Error(w, "orchestration not found: "+id, http.StatusNotFound)
		return
	}
	if record.Status != "planning" && record.Status != "running" {
		http.Error(w, "orchestration is not running", http.StatusConflict)
		return
	}
	cancelRun, ok := s.prepareCancelActiveOrchestration(id)
	if !ok {
		http.Error(w, "orchestration is not active on this server", http.StatusConflict)
		return
	}
	record.Status = "canceled"
	record.Error = "canceled"
	appendOrchestrationEvent(record, "cancel.requested", "", "Cancel requested")
	appendOrchestrationEvent(record, "canceled", "", "Orchestration canceled")
	record.UpdatedAt = time.Now().UTC()
	if err := saveOrchestrationRecord(record); err != nil {
		cancelRun()
		http.Error(w, "save orchestration: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cancelRun()
	_ = json.NewEncoder(w).Encode(record) //nolint:errcheck // best-effort response
}

func (s *Server) registerActiveOrchestration(id string, cancel context.CancelFunc) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	s.activeRuns[id] = cancel
}

func (s *Server) unregisterActiveOrchestration(id string) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	delete(s.activeRuns, id)
	delete(s.canceledRun, id)
}

func (s *Server) cancelActiveOrchestration(id string) bool {
	cancel, ok := s.prepareCancelActiveOrchestration(id)
	if !ok {
		return false
	}
	cancel()
	return true
}

func (s *Server) prepareCancelActiveOrchestration(id string) (context.CancelFunc, bool) {
	s.activeMu.Lock()
	cancel := s.activeRuns[id]
	if cancel != nil {
		s.canceledRun[id] = true
	}
	s.activeMu.Unlock()
	if cancel == nil {
		return nil, false
	}
	return cancel, true
}

func (s *Server) isOrchestrationCanceled(id string) bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return s.canceledRun[id]
}

func (s *Server) stopCanceledOrchestration(record *orchestrationRecord, message string) bool {
	if !s.isOrchestrationCanceled(record.ID) {
		return false
	}
	if latest, err := readOrchestrationRecord(record.ID); err == nil && latest.Status == "canceled" {
		s.auditOrchestrationOutcome(latest, auditOutcomeFailure, latest.Error)
		return true
	}
	record.Status = "canceled"
	record.Error = "canceled"
	appendOrchestrationEvent(record, "canceled", "", message)
	record.UpdatedAt = time.Now().UTC()
	if err := saveOrchestrationRecord(record); err != nil {
		slog.Warn("save orchestration failed", "id", record.ID, "error", err)
	}
	s.auditOrchestrationOutcome(record, auditOutcomeFailure, record.Error)
	return true
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

func appendTimelineForSubtaskEvent(record *orchestrationRecord, event *orchestrator.SubtaskEvent) {
	switch event.Type {
	case orchestrator.SubtaskStarted:
		appendOrchestrationEvent(record, "subtask.started", event.Subtask.ID, fmt.Sprintf("%s started", event.Subtask.AgentName))
	case orchestrator.SubtaskCompleted:
		message := "Subtask completed"
		if event.Result != nil && !event.Result.Success {
			message = "Subtask failed"
			if event.Result.Error != "" {
				message += ": " + event.Result.Error
			}
		}
		appendOrchestrationEvent(record, "subtask.completed", event.Subtask.ID, message)
	}
}

func appendOrchestrationEvent(record *orchestrationRecord, eventType, subtaskID, message string) {
	if record == nil {
		return
	}
	record.Events = append(record.Events, orchestrationEvent{
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		SubtaskID: subtaskID,
		Message:   safety.NewRedactor().RedactString(message),
	})
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

func orchestratePlanTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("AGENTOS_ORCHESTRATE_PLAN_TIMEOUT"))
	if raw == "" {
		return 90 * time.Second
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil || timeout <= 0 {
		return 90 * time.Second
	}
	return timeout
}

func resolveOrchestrateRepo(repo, baseBranch string) (string, error) {
	if repo == "" {
		repo = "."
	}
	if err := validateGitRef(defaultBaseBranch(baseBranch)); err != nil {
		return "", err
	}

	if cloneURL, ok := normalizeRemoteRepo(repo); ok {
		return cloneRemoteRepo(cloneURL, defaultBaseBranch(baseBranch))
	}
	if repo != "." {
		return "", fmt.Errorf("repo must be a GitHub HTTPS URL, owner/repo, or .")
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve current repo path: %w", err)
	}

	info, err := os.Stat(wd)
	if err != nil {
		return "", fmt.Errorf("repo does not exist: %s", wd)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repo is not a directory: %s", wd)
	}
	return wd, nil
}

func orchestrationsDir() string {
	return filepath.Join(apphome.Dir(), "orchestrates")
}

func saveOrchestrationRecord(record *orchestrationRecord) error {
	if err := os.MkdirAll(orchestrationsDir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(orchestrationsDir(), record.ID+".json")
	data, err := json.MarshalIndent(safety.NewRedactor().RedactValue(record), "", "  ")
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

func runArtifactPath(runDir, artifact string) (string, error) {
	if strings.Contains(artifact, string(filepath.Separator)) || artifact == "." || artifact == ".." {
		return "", fmt.Errorf("invalid artifact name")
	}
	cleanDir, err := filepath.Abs(runDir)
	if err != nil {
		return "", err
	}
	path := filepath.Join(cleanDir, artifact)
	if filepath.Dir(path) != cleanDir {
		return "", fmt.Errorf("artifact escapes run directory")
	}
	return path, nil
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

	if strings.HasPrefix(repo, "https://") {
		u, err := url.Parse(repo)
		if err != nil || u.User != nil || !strings.EqualFold(u.Host, "github.com") || u.RawQuery != "" || u.Fragment != "" {
			return "", false
		}
		path := strings.Trim(strings.TrimSuffix(u.EscapedPath(), ".git"), "/")
		if !githubRepoPathPattern.MatchString(path) {
			return "", false
		}
		return "https://github.com/" + path + ".git", true
	}
	if strings.Count(repo, "/") == 1 && !strings.HasPrefix(repo, ".") {
		repo = strings.TrimSuffix(repo, ".git")
		if githubRepoPathPattern.MatchString(repo) {
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
	// Inputs are constrained to HTTPS github.com owner/repo URLs and validated refs before args are built.
	// codeql[go/command-injection]
	cmd := exec.Command("git", args...)
	cmd.Env = gitCloneEnv(args)
	return cmd.CombinedOutput()
}

func gitCloneArgs(cloneURL, baseBranch, dest string) []string {
	args := []string{"clone", "--depth=1"}
	if baseBranch != "" {
		args = append(args, "--branch", baseBranch)
	}
	args = append(args, "--", cloneURL, dest)
	return args
}

func validateGitRef(ref string) error {
	if ref == "" {
		return nil
	}
	if !gitRefPattern.MatchString(ref) || strings.Contains(ref, "..") || strings.Contains(ref, "@{") || strings.HasSuffix(ref, ".") || strings.HasSuffix(ref, "/") {
		return fmt.Errorf("invalid git ref: %s", ref)
	}
	return nil
}

func gitCloneEnv(args []string) []string {
	env := os.Environ()
	token, err := agentosgh.TokenFromEnv(context.Background())
	if err != nil || token == "" || !cloneArgsUseGitHubHTTPS(args) {
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

func prepareOrchestrationGitHub(id string, req *orchestrateRequest) (*orchestrationGitHubState, error) {
	if req == nil || req.GitHub == nil || (!req.GitHub.CreateIssue && !req.GitHub.CreatePullRequest) {
		return nil, nil
	}
	_, _, full, ok := githubRepoForAPI(req.Repo)
	if !ok {
		return nil, fmt.Errorf("repository must be a GitHub HTTPS URL or owner/repo when GitHub artifacts are enabled")
	}

	branch := strings.TrimSpace(req.GitHub.BranchName)
	if branch == "" {
		branch = "agentos/" + id
	}
	if err := validateGitRef(branch); err != nil {
		return nil, err
	}

	prBase := defaultBaseBranch(req.GitHub.PRBase)
	if err := validateGitRef(prBase); err != nil {
		return nil, err
	}

	issueTitle := strings.TrimSpace(req.GitHub.IssueTitle)
	if issueTitle == "" {
		issueTitle = strings.TrimSpace(req.Task)
	}
	prTitle := strings.TrimSpace(req.GitHub.PRTitle)
	if prTitle == "" {
		prTitle = strings.TrimSpace(req.Task)
	}
	issueTemplate := normalizeArtifactTemplateID(req.GitHub.IssueTemplate)
	prTemplate := normalizeArtifactTemplateID(req.GitHub.PRTemplate)

	return &orchestrationGitHubState{
		Repo:              full,
		BranchName:        branch,
		IssueTitle:        issueTitle,
		IssueTemplate:     issueTemplate,
		PRTitle:           prTitle,
		PRTemplate:        prTemplate,
		PRBase:            prBase,
		CreateIssue:       req.GitHub.CreateIssue,
		CreatePullRequest: req.GitHub.CreatePullRequest,
	}, nil
}

func actorLogin(user *authUser) string {
	if user == nil || user.Login == "" {
		return "system"
	}
	return user.Login
}

func (s *Server) auditOrchestrationOutcome(record *orchestrationRecord, outcome auditOutcome, message string) {
	if record == nil {
		return
	}
	_ = appendAuditEvent(&auditEvent{ //nolint:errcheck // best-effort audit
		Actor:   record.Actor,
		Action:  "orchestrate.run",
		Target:  "orchestration/" + record.ID,
		Repo:    record.Repo,
		RunID:   record.ID,
		Outcome: outcome,
		Message: message,
	})
}

func (s *Server) createTrackingIssue(record *orchestrationRecord) {
	if record == nil || record.GitHub == nil || !record.GitHub.CreateIssue || record.GitHub.IssueURL != "" {
		return
	}
	owner, name, _, ok := githubRepoForAPI(record.GitHub.Repo)
	if !ok {
		record.GitHub.Error = "create issue: invalid GitHub repository"
		record.UpdatedAt = time.Now().UTC()
		_ = saveOrchestrationRecord(record)
		s.auditGitHubArtifact(record, "github.issue.create", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	client := agentosgh.NewClient(owner, name)
	issue, err := client.CreateIssue(agentosgh.CreateIssueRequest{
		Title: record.GitHub.IssueTitle,
		Body:  orchestrationIssueBody(record),
		Labels: []string{
			"agentos",
		},
	})
	if err != nil {
		record.GitHub.Error = "create issue: " + err.Error()
		record.UpdatedAt = time.Now().UTC()
		_ = saveOrchestrationRecord(record)
		s.auditGitHubArtifact(record, "github.issue.create", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	record.GitHub.IssueNumber = issue.Number
	record.GitHub.IssueURL = issue.HTMLURL
	record.GitHub.Error = ""
	record.UpdatedAt = time.Now().UTC()
	_ = saveOrchestrationRecord(record)
	s.auditGitHubArtifact(record, "github.issue.create", auditOutcomeSuccess, issue.HTMLURL)
}

func (s *Server) createPullRequestForOrchestration(record *orchestrationRecord) {
	if record == nil || record.GitHub == nil || !record.GitHub.CreatePullRequest || record.GitHub.PullRequestURL != "" {
		return
	}
	owner, name, _, ok := githubRepoForAPI(record.GitHub.Repo)
	if !ok {
		record.GitHub.Error = "create pull request: invalid GitHub repository"
		record.UpdatedAt = time.Now().UTC()
		_ = saveOrchestrationRecord(record)
		s.auditGitHubArtifact(record, "github.pull_request.create", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	client := agentosgh.NewClient(owner, name)
	pr, err := client.CreatePR(agentosgh.CreatePRRequest{
		Title: record.GitHub.PRTitle,
		Body:  orchestrationPRBody(record),
		Head:  record.GitHub.BranchName,
		Base:  record.GitHub.PRBase,
	})
	if err != nil {
		record.GitHub.Error = "create pull request: " + err.Error()
		record.UpdatedAt = time.Now().UTC()
		_ = saveOrchestrationRecord(record)
		s.auditGitHubArtifact(record, "github.pull_request.create", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	record.GitHub.PullRequestNumber = pr.Number
	record.GitHub.PullRequestURL = pr.HTMLURL
	record.GitHub.Error = ""
	record.UpdatedAt = time.Now().UTC()
	_ = saveOrchestrationRecord(record)
	s.auditGitHubArtifact(record, "github.pull_request.create", auditOutcomeSuccess, pr.HTMLURL)
}

func (s *Server) auditGitHubArtifact(record *orchestrationRecord, action string, outcome auditOutcome, message string) {
	if record == nil || record.GitHub == nil {
		return
	}
	_ = appendAuditEvent(&auditEvent{ //nolint:errcheck // best-effort audit
		Actor:   record.Actor,
		Action:  action,
		Target:  record.GitHub.Repo,
		Repo:    record.GitHub.Repo,
		RunID:   record.ID,
		Outcome: outcome,
		Message: message,
	})
}

type artifactConfig struct {
	OutputLanguage string `yaml:"outputLanguage"`
	Templates      struct {
		Issue struct {
			Body string `yaml:"body"`
		} `yaml:"issue"`
		PullRequest struct {
			Body string `yaml:"body"`
		} `yaml:"pullRequest"`
	} `yaml:"templates"`
}

type artifactTemplateData struct {
	RunID        string
	Repository   string
	BaseBranch   string
	TargetBranch string
	PRBase       string
	Strategy     string
	Agents       string
	Task         string
	Summary      string
	IssueURL     string
}

func loadArtifactConfig(repoPath string) artifactConfig {
	var cfg artifactConfig
	if repoPath == "" {
		return cfg
	}
	raw, err := os.ReadFile(filepath.Join(repoPath, ".agentos", "config.yaml"))
	if err != nil {
		return cfg
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		slog.Warn("parse .agentos/config.yaml failed", "repoPath", repoPath, "error", err)
	}
	cfg.OutputLanguage = normalizeOutputLanguage(cfg.OutputLanguage)
	return cfg
}

func applyArtifactConfig(req *orchestrateRequest, cfg artifactConfig) {
	if req == nil {
		return
	}
	if normalizeOutputLanguage(req.OutputLanguage) == "" {
		req.OutputLanguage = cfg.OutputLanguage
	}
	req.OutputLanguage = normalizeOutputLanguage(req.OutputLanguage)
	if req.GitHub == nil {
		return
	}
	if strings.TrimSpace(req.GitHub.IssueTemplate) == "" && cfg.Templates.Issue.Body != "" {
		req.GitHub.IssueTemplate = "repository"
	}
	if strings.TrimSpace(req.GitHub.PRTemplate) == "" && cfg.Templates.PullRequest.Body != "" {
		req.GitHub.PRTemplate = "repository"
	}
}

func normalizeOutputLanguage(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "", "default":
		return ""
	case "ja", "japanese", "日本語":
		return "ja"
	case "en", "english":
		return "en"
	default:
		return ""
	}
}

func artifactLanguage(record *orchestrationRecord) string {
	if record == nil {
		return "en"
	}
	if language := normalizeOutputLanguage(record.OutputLanguage); language != "" {
		return language
	}
	return "en"
}

func normalizeArtifactTemplateID(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "", "default":
		return "default"
	case "repository", "repo":
		return "repository"
	default:
		return "default"
	}
}

func orchestrationIssueBody(record *orchestrationRecord) string {
	return renderArtifactBody(record, "issue")
}

func orchestrationPRBody(record *orchestrationRecord) string {
	return renderArtifactBody(record, "pull_request")
}

func renderArtifactBody(record *orchestrationRecord, artifact string) string {
	if record == nil {
		return ""
	}
	body := artifactTemplate(record, artifact)
	data := artifactTemplateData{
		RunID:        record.ID,
		Repository:   "",
		BaseBranch:   record.BaseBranch,
		TargetBranch: "",
		PRBase:       "",
		Strategy:     record.Strategy,
		Agents:       strings.Join(record.Agents, ", "),
		Task:         record.Task,
		Summary:      record.Summary,
		IssueURL:     "",
	}
	if record.GitHub != nil {
		data.Repository = record.GitHub.Repo
		data.TargetBranch = record.GitHub.BranchName
		data.PRBase = record.GitHub.PRBase
		data.IssueURL = record.GitHub.IssueURL
	}
	rendered, err := renderTextTemplate(body, &data)
	if err != nil {
		slog.Warn("render artifact template failed", "artifact", artifact, "run", record.ID, "error", err)
		rendered, _ = renderTextTemplate(defaultArtifactTemplate(artifact, artifactLanguage(record)), &data)
	}
	return safety.NewRedactor().RedactString(rendered)
}

func artifactTemplate(record *orchestrationRecord, artifact string) string {
	language := artifactLanguage(record)
	templateID := "default"
	if record != nil && record.GitHub != nil {
		if artifact == "issue" {
			templateID = normalizeArtifactTemplateID(record.GitHub.IssueTemplate)
		} else {
			templateID = normalizeArtifactTemplateID(record.GitHub.PRTemplate)
		}
	}
	if templateID == "repository" && record != nil {
		cfg := loadArtifactConfig(record.RepoPath)
		if artifact == "issue" && cfg.Templates.Issue.Body != "" {
			return cfg.Templates.Issue.Body
		}
		if artifact == "pull_request" && cfg.Templates.PullRequest.Body != "" {
			return cfg.Templates.PullRequest.Body
		}
	}
	return defaultArtifactTemplate(artifact, language)
}

func defaultArtifactTemplate(artifact, language string) string {
	if artifact == "issue" {
		if language == "ja" {
			return "AgentOS Orchestrate により作成されました。\n\n" +
				"- Run: `{{.RunID}}`\n" +
				"- Repository: `{{.Repository}}`\n" +
				"- Base branch: `{{.BaseBranch}}`\n" +
				"- Target branch: `{{.TargetBranch}}`\n" +
				"- Strategy: `{{.Strategy}}`\n" +
				"- Agents: `{{.Agents}}`\n\n" +
				"## タスク\n\n{{.Task}}\n"
		}
		return "Created by AgentOS Orchestrate.\n\n" +
			"- Run: `{{.RunID}}`\n" +
			"- Repository: `{{.Repository}}`\n" +
			"- Base branch: `{{.BaseBranch}}`\n" +
			"- Target branch: `{{.TargetBranch}}`\n" +
			"- Strategy: `{{.Strategy}}`\n" +
			"- Agents: `{{.Agents}}`\n\n" +
			"## Task\n\n{{.Task}}\n"
	}
	if language == "ja" {
		return "AgentOS Orchestrate により作成されました。\n\n" +
			"{{if .IssueURL}}Tracking issue: {{.IssueURL}}\n\n{{end}}" +
			"- Run: `{{.RunID}}`\n" +
			"- Base branch: `{{.PRBase}}`\n" +
			"- Agents: `{{.Agents}}`\n\n" +
			"{{if .Summary}}## 概要\n\n{{.Summary}}\n{{end}}"
	}
	return "Created by AgentOS Orchestrate.\n\n" +
		"{{if .IssueURL}}Tracking issue: {{.IssueURL}}\n\n{{end}}" +
		"- Run: `{{.RunID}}`\n" +
		"- Base branch: `{{.PRBase}}`\n" +
		"- Agents: `{{.Agents}}`\n\n" +
		"{{if .Summary}}## Summary\n\n{{.Summary}}\n{{end}}"
}

func renderTextTemplate(body string, data *artifactTemplateData) (string, error) {
	tpl, err := template.New("artifact").Option("missingkey=zero").Parse(body)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func githubRepoForAPI(repo string) (owner, name, full string, ok bool) {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", "", "", false
	}
	if strings.HasPrefix(repo, "https://") {
		u, err := url.Parse(repo)
		if err != nil || !strings.EqualFold(u.Host, "github.com") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", "", "", false
		}
		repo = strings.Trim(strings.TrimSuffix(u.EscapedPath(), ".git"), "/")
	}
	repo = strings.TrimSuffix(repo, ".git")
	if !githubRepoPathPattern.MatchString(repo) {
		return "", "", "", false
	}
	parts := strings.SplitN(repo, "/", 2)
	return parts[0], parts[1], repo, true
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
