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
	"github.com/kazyamaz200/agentos/internal/factory"
	agentosgh "github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/guideline"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/memory"
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
var customAgentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
var scenarioVariableNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

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
	mux.HandleFunc("/api/agents/repository", s.handleRepositoryAgents)
	mux.HandleFunc("/api/settings/llm", s.handleLLMSettings)
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunDetail)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/repository-memory", s.handleRepositoryMemory)
	mux.HandleFunc("/api/repository-memory/", s.handleRepositoryMemoryItem)
	mux.HandleFunc("/api/repository-guidelines", s.handleRepositoryGuidelines)
	mux.HandleFunc("/api/repository-guidelines/", s.handleRepositoryGuidelineItem)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/github/", s.handleGitHub)
	mux.HandleFunc("/api/orchestrate/templates", s.handleOrchestrateTemplates)
	mux.HandleFunc("/api/orchestrate/recommend", s.handleOrchestrateRecommend)
	mux.HandleFunc("/api/orchestrate/from-issue", s.handleOrchestrateFromIssue)
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

type repositoryAgentsRequest struct {
	Repo       string `json:"repo"`
	BaseBranch string `json:"baseBranch"`
}

type repositoryAgentsResponse struct {
	Agents []agent.Definition `json:"agents"`
}

func (s *Server) handleRepositoryAgents(w http.ResponseWriter, r *http.Request) {
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
	var req repositoryAgentsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Repo = strings.TrimSpace(req.Repo)
	req.BaseBranch = defaultBaseBranch(req.BaseBranch)
	if !s.requireAutomationPermission(w, r, user, "agents.repository.load", "repository", req.Repo, "") {
		return
	}
	repoPath, err := resolveOrchestrateRepo(req.Repo, req.BaseBranch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defs, err := loadRepositoryAgentDefinitions(repoPath, s.agentReg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(repositoryAgentsResponse{Agents: defs}) //nolint:errcheck // best-effort
}

func (s *Server) handleOrchestrateTemplates(w http.ResponseWriter, r *http.Request) {
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
	var req orchestrateTemplatesRequest
	if len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	req.Repo = strings.TrimSpace(req.Repo)
	req.BaseBranch = defaultBaseBranch(req.BaseBranch)
	if !s.requireAutomationPermission(w, r, user, "orchestrate.templates.list", "repository", req.Repo, "") {
		return
	}

	templates := builtInScenarioTemplates(s.agentReg)
	if req.Repo != "" {
		repoPath, err := resolveOrchestrateRepo(req.Repo, req.BaseBranch)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		repoTemplates, err := loadRepositoryScenarioTemplates(repoPath, s.agentReg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		templates = append(templates, repoTemplates...)
	}
	_ = json.NewEncoder(w).Encode(templates) //nolint:errcheck // best-effort
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
	Agents         []string                   `json:"agents"`
	CustomAgents   []agent.Definition         `json:"customAgents,omitempty"`
	Scenario       *scenarioTemplateSelection `json:"scenarioTemplate,omitempty"`
	Repo           string                     `json:"repo"`
	BaseBranch     string                     `json:"baseBranch"`
	Task           string                     `json:"task"`
	Strategy       string                     `json:"strategy"`
	LLMPreset      string                     `json:"llmPreset"`
	OutputLanguage string                     `json:"outputLanguage,omitempty"`
	GitHub         *orchestrateGitHubRequest  `json:"github,omitempty"`
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

type orchestrateFromIssueRequest struct {
	Repo              string   `json:"repo"`
	BaseBranch        string   `json:"baseBranch"`
	IssueNumber       int      `json:"issueNumber"`
	IssueTitle        string   `json:"issueTitle"`
	IssueBody         string   `json:"issueBody"`
	IssueURL          string   `json:"issueUrl"`
	Labels            []string `json:"labels,omitempty"`
	TriggerID         string   `json:"triggerId,omitempty"`
	OutputLanguage    string   `json:"outputLanguage,omitempty"`
	LLMPreset         string   `json:"llmPreset,omitempty"`
	Agents            []string `json:"agents,omitempty"`
	Strategy          string   `json:"strategy,omitempty"`
	CreatePullRequest *bool    `json:"createPullRequest,omitempty"`
	ClosePolicy       string   `json:"closePolicy,omitempty"`
	RequireApproval   *bool    `json:"requireApproval,omitempty"`
}

type orchestrateRecommendRequest struct {
	Repo       string `json:"repo"`
	BaseBranch string `json:"baseBranch"`
	Task       string `json:"task"`
}

type orchestrateTemplatesRequest struct {
	Repo       string `json:"repo"`
	BaseBranch string `json:"baseBranch"`
}

type scenarioTemplateSelection struct {
	ID     string `json:"id"`
	Name   string `json:"name,omitempty"`
	Source string `json:"source,omitempty"`
}

type scenarioTemplateVariable struct {
	Name        string `json:"name" yaml:"name"`
	Label       string `json:"label,omitempty" yaml:"label,omitempty"`
	Placeholder string `json:"placeholder,omitempty" yaml:"placeholder,omitempty"`
	Default     string `json:"default,omitempty" yaml:"default,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

type scenarioTemplate struct {
	ID                string                     `json:"id" yaml:"id"`
	Name              string                     `json:"name" yaml:"name"`
	Description       string                     `json:"description,omitempty" yaml:"description,omitempty"`
	Source            string                     `json:"source,omitempty" yaml:"source,omitempty"`
	Agents            []string                   `json:"agents" yaml:"agents"`
	Strategy          string                     `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	CreatePullRequest bool                       `json:"createPullRequest,omitempty" yaml:"createPullRequest,omitempty"`
	RequireApproval   bool                       `json:"requireApproval,omitempty" yaml:"requireApproval,omitempty"`
	TaskTemplate      string                     `json:"taskTemplate" yaml:"taskTemplate"`
	Variables         []scenarioTemplateVariable `json:"variables,omitempty" yaml:"variables,omitempty"`
}

type orchestrationRecommendation struct {
	Preset            string   `json:"preset"`
	Confidence        float64  `json:"confidence"`
	Rationale         string   `json:"rationale"`
	Agents            []string `json:"agents"`
	Strategy          string   `json:"strategy"`
	CreatePullRequest bool     `json:"createPullRequest"`
	RequireApproval   bool     `json:"requireApproval"`
}

type orchestrationRecord struct {
	ID                       string                                 `json:"id"`
	Actor                    string                                 `json:"actor,omitempty"`
	Repo                     string                                 `json:"repo"`
	RepoPath                 string                                 `json:"repoPath,omitempty"`
	BaseBranch               string                                 `json:"baseBranch"`
	Task                     string                                 `json:"task"`
	Agents                   []string                               `json:"agents"`
	CustomAgents             []agent.Definition                     `json:"customAgents,omitempty"`
	Scenario                 *scenarioTemplateSelection             `json:"scenarioTemplate,omitempty"`
	Strategy                 string                                 `json:"strategy"`
	LLMPreset                string                                 `json:"llmPreset"`
	OutputLanguage           string                                 `json:"outputLanguage,omitempty"`
	Status                   string                                 `json:"status"`
	Error                    string                                 `json:"error,omitempty"`
	Plan                     *orchestrator.TaskPlan                 `json:"plan,omitempty"`
	Subtasks                 []orchestrationSubtaskState            `json:"subtasks,omitempty"`
	Results                  []orchestrator.SubtaskResult           `json:"results,omitempty"`
	Events                   []orchestrationEvent                   `json:"events,omitempty"`
	Summary                  string                                 `json:"summary,omitempty"`
	MemoryUsed               []memory.RepositoryEntry               `json:"memoryUsed,omitempty"`
	MemoryProposals          []memory.RepositoryEntry               `json:"memoryProposals,omitempty"`
	GuidelinesUsed           []guideline.AppliedRepositoryGuideline `json:"guidelinesUsed,omitempty"`
	MissedRequiredGuidelines []guideline.RepositoryGuideline        `json:"missedRequiredGuidelines,omitempty"`
	GitHub                   *orchestrationGitHubState              `json:"github,omitempty"`
	CreatedAt                time.Time                              `json:"createdAt"`
	UpdatedAt                time.Time                              `json:"updatedAt"`
}

type orchestrationEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	SubtaskID string    `json:"subtaskId,omitempty"`
	Message   string    `json:"message"`
}

type orchestrationGitHubState struct {
	Repo                  string `json:"repo"`
	BranchName            string `json:"branchName,omitempty"`
	IssueTitle            string `json:"issueTitle,omitempty"`
	IssueTemplate         string `json:"issueTemplate,omitempty"`
	IssueURL              string `json:"issueUrl,omitempty"`
	IssueNumber           int    `json:"issueNumber,omitempty"`
	PRTitle               string `json:"prTitle,omitempty"`
	PRTemplate            string `json:"prTemplate,omitempty"`
	PRBase                string `json:"prBase,omitempty"`
	PullRequestURL        string `json:"pullRequestUrl,omitempty"`
	PullRequestNumber     int    `json:"pullRequestNumber,omitempty"`
	Error                 string `json:"error,omitempty"`
	CreateIssue           bool   `json:"createIssue,omitempty"`
	CreatePullRequest     bool   `json:"createPullRequest,omitempty"`
	SourceIssueURL        string `json:"sourceIssueUrl,omitempty"`
	SourceIssueNumber     int    `json:"sourceIssueNumber,omitempty"`
	SourceIssueTitle      string `json:"sourceIssueTitle,omitempty"`
	SourceTriggerID       string `json:"sourceTriggerId,omitempty"`
	SourceStartCommentURL string `json:"sourceStartCommentUrl,omitempty"`
	SourceFinalCommentURL string `json:"sourceFinalCommentUrl,omitempty"`
	ClosePolicy           string `json:"closePolicy,omitempty"`
	ApprovalStatus        string `json:"approvalStatus,omitempty"`
	ApprovalActor         string `json:"approvalActor,omitempty"`
	ApprovalReason        string `json:"approvalReason,omitempty"`
	ApprovedAt            string `json:"approvedAt,omitempty"`
	SourceIssueClosed     bool   `json:"sourceIssueClosed,omitempty"`
	SourceIssueClosedAt   string `json:"sourceIssueClosedAt,omitempty"`
}

type orchestrationSourceIssue struct {
	Repo        string
	Number      int
	Title       string
	URL         string
	TriggerID   string
	ClosePolicy string
}

type orchestrationApprovalRequest struct {
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
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

func (s *Server) handleOrchestrateRecommend(w http.ResponseWriter, r *http.Request) {
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
	var req orchestrateRecommendRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Repo = strings.TrimSpace(req.Repo)
	req.BaseBranch = defaultBaseBranch(req.BaseBranch)
	req.Task = strings.TrimSpace(req.Task)
	if req.Task == "" {
		http.Error(w, "task is required", http.StatusBadRequest)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "orchestrate.recommend", "orchestration", req.Repo, "") {
		return
	}

	recommendation := recommendOrchestration(req.Task, recommendRepoSignals(req.Repo, req.BaseBranch), s.agentReg)
	_ = json.NewEncoder(w).Encode(recommendation) //nolint:errcheck // best-effort response
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

	s.startOrchestration(w, r, user, &req, nil)
}

func (s *Server) handleOrchestrateFromIssue(w http.ResponseWriter, r *http.Request) {
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
	var importReq orchestrateFromIssueRequest
	if err := json.Unmarshal(body, &importReq); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req, source, err := orchestrationRequestFromIssue(&importReq, s.agentReg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !s.auth.userCanAutomate(user) {
		s.requireAutomationPermission(w, r, user, "orchestrate.create", "orchestration", req.Repo, "")
		return
	}
	if existing, ok := findDuplicateIssueOrchestration(req.Repo, source); ok {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(existing) //nolint:errcheck // best-effort response
		return
	}
	s.startOrchestration(w, r, user, req, source)
}

func (s *Server) startOrchestration(w http.ResponseWriter, r *http.Request, user *authUser, req *orchestrateRequest, source *orchestrationSourceIssue) {
	if req == nil {
		http.Error(w, "request is required", http.StatusBadRequest)
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
	applyArtifactConfig(req, artifactConfig)
	scenarioSelection := resolveScenarioTemplateSelection(req.Scenario, repoPath, s.agentReg)

	agents := make(map[string]runtime.Agent)
	customAgents, err := validateCustomAgentDefinitions(req.CustomAgents, s.agentReg)
	if err != nil {
		http.Error(w, "custom agents: "+err.Error(), http.StatusBadRequest)
		return
	}
	customByName := make(map[string]agent.Definition, len(customAgents))
	for i := range customAgents {
		def := customAgents[i]
		customByName[def.Metadata.Name] = def
	}
	for _, name := range req.Agents {
		if def, ok := customByName[name]; ok {
			agents[name] = agent.NewBaseAgent(def.Metadata.Name, llmClient)
		} else {
			a, err := s.agentReg.Create(name, llmClient)
			if err != nil {
				http.Error(w, "lookup agent "+name+": "+err.Error(), http.StatusBadRequest)
				return
			}
			agents[name] = a
		}
	}

	id := generateID()
	githubState, err := prepareOrchestrationGitHub(id, req)
	if err != nil {
		http.Error(w, "github: "+err.Error(), http.StatusBadRequest)
		return
	}
	githubState = attachSourceIssue(githubState, source)

	now := time.Now().UTC()
	record := &orchestrationRecord{
		ID:             id,
		Actor:          actorLogin(user),
		Repo:           req.Repo,
		RepoPath:       repoPath,
		BaseBranch:     req.BaseBranch,
		Task:           req.Task,
		Agents:         req.Agents,
		CustomAgents:   selectedCustomAgentDefinitions(req.Agents, customByName),
		Scenario:       scenarioSelection,
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
	orch.SetAgentMetadata(orchestrationAgentMetadata(record, agents, s.agentReg), orchestrationAgentProfiles(record, agents))
	orch.SetBaseBranch(record.BaseBranch)
	orch.SetRunID(record.ID)

	if record.Strategy == "parallel" {
		orch.SetStrategy(orchestrator.StrategyParallel)
	}

	s.createTrackingIssue(record)
	s.postSourceIssueStartComment(record)
	appendOrchestrationEvent(record, "planning.started", "", "Planning started")
	record.MemoryUsed = repositoryMemoryForPlanning(runCtx, record)
	if len(record.MemoryUsed) > 0 {
		appendOrchestrationEvent(record, "memory.loaded", "", fmt.Sprintf("Loaded %d repository memory entries", len(record.MemoryUsed)))
	}
	loadRepositoryGuidelinesForRecord(runCtx, record)
	repositoryGuidelines := repositoryGuidelinesForPlanning(runCtx, record, "")
	if len(repositoryGuidelines) > 0 {
		appendOrchestrationEvent(record, "guidelines.loaded", "", fmt.Sprintf("Loaded %d repository guidelines", len(repositoryGuidelines)))
	}

	planCtx, cancelPlan := context.WithTimeout(runCtx, orchestratePlanTimeout())
	defer cancelPlan()
	planningTask := taskWithRepositoryMemory(record.Task, record.MemoryUsed)
	planningTask = taskWithRepositoryGuidelines(planningTask, repositoryGuidelines)
	plan, err := orch.Plan(planCtx, planningTask)
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
		s.postSourceIssueFinalComment(record)
		return
	}
	if s.stopCanceledOrchestration(record, "Orchestration canceled") {
		return
	}
	record.GuidelinesUsed = applyRepositoryGuidelinesToPlan(plan, repositoryGuidelines)
	record.MissedRequiredGuidelines = missedRequiredGuidelines(repositoryGuidelines, record.GuidelinesUsed)
	if len(record.MissedRequiredGuidelines) > 0 {
		appendOrchestrationEvent(record, "guidelines.required_missed", "", fmt.Sprintf("%d required guidelines were not attached to subtasks", len(record.MissedRequiredGuidelines)))
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
		s.postSourceIssueFinalComment(record)
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
	record.MemoryProposals = proposeRepositoryMemory(context.Background(), record, results)
	if len(record.MemoryProposals) > 0 {
		appendOrchestrationEvent(record, "memory.proposed", "", fmt.Sprintf("Proposed %d repository memory updates", len(record.MemoryProposals)))
	}
	record.Status = "completed"
	if record.GitHub != nil && record.GitHub.ClosePolicy == "after_human_approval" && record.GitHub.ApprovalStatus == "pending" {
		record.Status = "pending_approval"
		appendOrchestrationEvent(record, "approval.pending", "", "Human approval is required before closing the source Issue")
	}
	appendOrchestrationEvent(record, "completed", "", "Orchestration completed")
	record.UpdatedAt = time.Now().UTC()
	if err := saveOrchestrationRecord(record); err != nil {
		slog.Warn("save orchestration failed", "id", record.ID, "error", err)
	}
	s.auditOrchestrationOutcome(record, auditOutcomeSuccess, "")
	s.createPullRequestForOrchestration(record)
	s.postSourceIssueFinalComment(record)
	s.closeSourceIssueIfPolicyAllows(record)
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
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	if strings.HasSuffix(r.URL.Path, "/cancel") {
		s.handleOrchestrateCancel(w, r, user)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/approval") {
		s.handleOrchestrateApproval(w, r, user)
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

func (s *Server) handleOrchestrateApproval(w http.ResponseWriter, r *http.Request, user *authUser) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	id := filepath.Base(filepath.Dir(r.URL.Path))
	if !s.requireAutomationPermission(w, r, user, "orchestrate.approval", "orchestration/"+id, "", id) {
		return
	}
	var req orchestrationApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	if req.Action != "approve" && req.Action != "reject" {
		http.Error(w, "action must be approve or reject", http.StatusBadRequest)
		return
	}
	record, err := readOrchestrationRecord(id)
	if err != nil {
		http.Error(w, "orchestration not found: "+id, http.StatusNotFound)
		return
	}
	if record.GitHub == nil || record.GitHub.ApprovalStatus == "" {
		http.Error(w, "orchestration does not require approval", http.StatusConflict)
		return
	}
	if record.GitHub.ApprovalStatus == "approved" || record.GitHub.ApprovalStatus == "rejected" {
		http.Error(w, "approval is already resolved", http.StatusConflict)
		return
	}

	now := time.Now().UTC()
	record.GitHub.ApprovalActor = actorLogin(user)
	record.GitHub.ApprovalReason = strings.TrimSpace(req.Reason)
	record.GitHub.ApprovedAt = now.Format(time.RFC3339)
	if req.Action == "approve" {
		record.GitHub.ApprovalStatus = "approved"
		appendOrchestrationEvent(record, "approval.approved", "", "Approval granted")
		if record.Status == "pending_approval" {
			record.Status = "completed"
		}
		s.closeSourceIssueIfPolicyAllows(record)
	} else {
		record.GitHub.ApprovalStatus = "rejected"
		record.Status = "approval_rejected"
		appendOrchestrationEvent(record, "approval.rejected", "", approvalRejectionMessage(record.GitHub.ApprovalReason))
	}
	record.UpdatedAt = now
	if err := saveOrchestrationRecord(record); err != nil {
		http.Error(w, "save orchestration: "+err.Error(), http.StatusInternalServerError)
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
	s.postSourceIssueFinalComment(record)
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
		s.postSourceIssueFinalComment(latest)
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
	s.postSourceIssueFinalComment(record)
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

func orchestrationRequestFromIssue(importReq *orchestrateFromIssueRequest, reg *agent.Registry) (*orchestrateRequest, *orchestrationSourceIssue, error) {
	if importReq == nil {
		return nil, nil, fmt.Errorf("request is required")
	}
	repo := strings.TrimSpace(importReq.Repo)
	if _, _, full, ok := githubRepoForAPI(repo); ok {
		repo = full
	} else {
		return nil, nil, fmt.Errorf("repo must be a GitHub HTTPS URL or owner/repo")
	}
	if importReq.IssueNumber <= 0 {
		return nil, nil, fmt.Errorf("issueNumber is required")
	}
	title := strings.TrimSpace(importReq.IssueTitle)
	if title == "" {
		return nil, nil, fmt.Errorf("issueTitle is required")
	}

	taskText := issueOrchestrationTask(importReq)
	rec := recommendOrchestration(taskText, nil, reg)
	controls := issueTriggerControls(importReq.Labels, importReq.IssueBody)

	agents := sanitizeAgentNames(importReq.Agents)
	if len(agents) == 0 {
		agents = sanitizeAgentNames(controls.Agents)
	}
	if len(agents) == 0 {
		agents = rec.Agents
	}
	strategy := strings.TrimSpace(importReq.Strategy)
	if strategy == "" {
		strategy = controls.Strategy
	}
	if strategy == "" {
		strategy = rec.Strategy
	}
	createPullRequest := rec.CreatePullRequest
	if controls.CreatePullRequest != nil {
		createPullRequest = *controls.CreatePullRequest
	}
	if importReq.CreatePullRequest != nil {
		createPullRequest = *importReq.CreatePullRequest
	}
	closePolicy := normalizeClosePolicy(importReq.ClosePolicy)
	if closePolicy == "" {
		closePolicy = controls.ClosePolicy
	}
	if closePolicy == "" {
		closePolicy = defaultClosePolicy(&rec, createPullRequest)
	}
	requireApproval := closePolicy == "after_human_approval"
	if controls.RequireApproval != nil {
		requireApproval = *controls.RequireApproval
	}
	if importReq.RequireApproval != nil {
		requireApproval = *importReq.RequireApproval
	}
	if requireApproval {
		closePolicy = "after_human_approval"
	}

	req := &orchestrateRequest{
		Agents:         agents,
		Repo:           repo,
		BaseBranch:     defaultBaseBranch(importReq.BaseBranch),
		Task:           taskText,
		Strategy:       strategy,
		LLMPreset:      strings.TrimSpace(importReq.LLMPreset),
		OutputLanguage: strings.TrimSpace(importReq.OutputLanguage),
		GitHub: &orchestrateGitHubRequest{
			CreatePullRequest: createPullRequest,
			BranchName:        fmt.Sprintf("agentos/issue-%d", importReq.IssueNumber),
			PRBase:            defaultBaseBranch(importReq.BaseBranch),
			PRTitle:           title,
		},
	}
	source := &orchestrationSourceIssue{
		Repo:        repo,
		Number:      importReq.IssueNumber,
		Title:       title,
		URL:         strings.TrimSpace(importReq.IssueURL),
		TriggerID:   strings.TrimSpace(importReq.TriggerID),
		ClosePolicy: closePolicy,
	}
	return req, source, nil
}

type issueTriggerOptions struct {
	Agents            []string
	Strategy          string
	CreatePullRequest *bool
	ClosePolicy       string
	RequireApproval   *bool
}

func issueTriggerControls(labels []string, text string) issueTriggerOptions {
	var opts issueTriggerOptions
	for _, label := range labels {
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "agentos:create-pr":
			value := true
			opts.CreatePullRequest = &value
		case "agentos:report-only":
			value := false
			opts.CreatePullRequest = &value
		case "agentos:parallel":
			opts.Strategy = "parallel"
		case "agentos:sequential":
			opts.Strategy = "sequential"
		case "agentos:close-never":
			opts.ClosePolicy = "never"
		case "agentos:close-on-quality-gate-pass":
			opts.ClosePolicy = "on_quality_gate_pass"
		case "agentos:close-on-pr-merge":
			opts.ClosePolicy = "on_pr_merge"
		case "agentos:approval-required":
			value := true
			opts.RequireApproval = &value
		}
	}
	if command, ok := parseAgentOSRunCommand(text); ok {
		if command.Strategy != "" {
			opts.Strategy = command.Strategy
		}
		if len(command.Agents) > 0 {
			opts.Agents = command.Agents
		}
		if command.CreatePullRequest != nil {
			opts.CreatePullRequest = command.CreatePullRequest
		}
		if command.ClosePolicy != "" {
			opts.ClosePolicy = command.ClosePolicy
		}
		if command.RequireApproval != nil {
			opts.RequireApproval = command.RequireApproval
		}
	}
	return opts
}

func parseAgentOSRunCommand(text string) (issueTriggerOptions, bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "/agentos run") {
			continue
		}
		var opts issueTriggerOptions
		for _, field := range strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "/agentos run"))) {
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "agents":
				opts.Agents = sanitizeAgentNames(strings.Split(value, ","))
			case "strategy":
				switch strings.ToLower(strings.TrimSpace(value)) {
				case "parallel", "sequential":
					opts.Strategy = strings.ToLower(strings.TrimSpace(value))
				}
			case "create_pr", "createpullrequest":
				enabled := strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
				opts.CreatePullRequest = &enabled
			case "close_policy", "closepolicy":
				if policy := normalizeClosePolicy(value); policy != "" {
					opts.ClosePolicy = policy
				}
			case "approval", "require_approval", "requireapproval":
				enabled := strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
				opts.RequireApproval = &enabled
			}
		}
		return opts, true
	}
	return issueTriggerOptions{}, false
}

func normalizeClosePolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", "default":
		return ""
	case "never":
		return "never"
	case "on_pr_merge", "on-pr-merge", "pr_merge":
		return "on_pr_merge"
	case "on_quality_gate_pass", "on-quality-gate-pass", "quality_gate_pass":
		return "on_quality_gate_pass"
	case "after_human_approval", "after-human-approval", "human_approval":
		return "after_human_approval"
	default:
		return ""
	}
}

func defaultClosePolicy(rec *orchestrationRecommendation, createPullRequest bool) string {
	if rec != nil && rec.RequireApproval {
		return "after_human_approval"
	}
	if createPullRequest {
		return "on_pr_merge"
	}
	return "never"
}

func sanitizeAgentNames(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		name := strings.TrimSpace(value)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func issueOrchestrationTask(importReq *orchestrateFromIssueRequest) string {
	if importReq == nil {
		return ""
	}
	labels := sanitizeAgentNames(importReq.Labels)
	var b strings.Builder
	fmt.Fprintf(&b, "Address GitHub Issue #%d: %s\n\n", importReq.IssueNumber, strings.TrimSpace(importReq.IssueTitle))
	if url := strings.TrimSpace(importReq.IssueURL); url != "" {
		fmt.Fprintf(&b, "Source issue: %s\n\n", url)
	}
	body := strings.TrimSpace(importReq.IssueBody)
	if body != "" {
		fmt.Fprintf(&b, "Issue body:\n%s\n\n", body)
	}
	if len(labels) > 0 {
		fmt.Fprintf(&b, "Labels: %s\n", strings.Join(labels, ", "))
	}
	return strings.TrimSpace(b.String())
}

func attachSourceIssue(state *orchestrationGitHubState, source *orchestrationSourceIssue) *orchestrationGitHubState {
	if source == nil {
		return state
	}
	if state == nil {
		state = &orchestrationGitHubState{Repo: source.Repo}
	}
	if state.Repo == "" {
		state.Repo = source.Repo
	}
	state.SourceIssueNumber = source.Number
	state.SourceIssueTitle = source.Title
	state.SourceIssueURL = source.URL
	state.SourceTriggerID = source.TriggerID
	state.ClosePolicy = normalizeClosePolicy(source.ClosePolicy)
	if state.ClosePolicy == "" {
		state.ClosePolicy = "never"
	}
	if state.ClosePolicy == "after_human_approval" && state.ApprovalStatus == "" {
		state.ApprovalStatus = "pending"
	}
	if state.IssueNumber == 0 && state.IssueURL == "" {
		state.IssueNumber = source.Number
		state.IssueTitle = source.Title
		state.IssueURL = source.URL
	}
	return state
}

func findDuplicateIssueOrchestration(repo string, source *orchestrationSourceIssue) (*orchestrationRecord, bool) {
	if source == nil {
		return nil, false
	}
	records, err := listOrchestrationRecords()
	if err != nil {
		return nil, false
	}
	for _, record := range records {
		if record == nil || record.GitHub == nil {
			continue
		}
		if source.TriggerID != "" && record.GitHub.SourceTriggerID == source.TriggerID {
			return record, true
		}
		if record.GitHub.SourceIssueNumber == source.Number && sameRepo(record.GitHub.Repo, repo) && orchestrationInProgress(record.Status) {
			return record, true
		}
	}
	return nil, false
}

func sameRepo(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func orchestrationInProgress(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "planning", "running":
		return true
	default:
		return false
	}
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

func (s *Server) postSourceIssueStartComment(record *orchestrationRecord) {
	if record == nil || record.GitHub == nil || record.GitHub.SourceIssueNumber == 0 || record.GitHub.SourceStartCommentURL != "" {
		return
	}
	comment, err := s.createSourceIssueComment(record, sourceIssueStartCommentBody(record))
	if err != nil {
		record.GitHub.Error = "comment source issue: " + err.Error()
		record.UpdatedAt = time.Now().UTC()
		_ = saveOrchestrationRecord(record)
		s.auditGitHubArtifact(record, "github.issue.comment", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	record.GitHub.SourceStartCommentURL = comment.HTMLURL
	record.GitHub.Error = ""
	record.UpdatedAt = time.Now().UTC()
	_ = saveOrchestrationRecord(record)
	s.auditGitHubArtifact(record, "github.issue.comment", auditOutcomeSuccess, comment.HTMLURL)
}

func (s *Server) postSourceIssueFinalComment(record *orchestrationRecord) {
	if record == nil || record.GitHub == nil || record.GitHub.SourceIssueNumber == 0 || record.GitHub.SourceFinalCommentURL != "" {
		return
	}
	if orchestrationInProgress(record.Status) {
		return
	}
	comment, err := s.createSourceIssueComment(record, sourceIssueFinalCommentBody(record))
	if err != nil {
		record.GitHub.Error = "comment source issue: " + err.Error()
		record.UpdatedAt = time.Now().UTC()
		_ = saveOrchestrationRecord(record)
		s.auditGitHubArtifact(record, "github.issue.comment", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	record.GitHub.SourceFinalCommentURL = comment.HTMLURL
	record.GitHub.Error = ""
	record.UpdatedAt = time.Now().UTC()
	_ = saveOrchestrationRecord(record)
	s.auditGitHubArtifact(record, "github.issue.comment", auditOutcomeSuccess, comment.HTMLURL)
}

func (s *Server) createSourceIssueComment(record *orchestrationRecord, body string) (*agentosgh.IssueComment, error) {
	if record == nil || record.GitHub == nil {
		return nil, fmt.Errorf("missing GitHub source issue")
	}
	owner, name, _, ok := githubRepoForAPI(record.GitHub.Repo)
	if !ok {
		return nil, fmt.Errorf("invalid GitHub repository")
	}
	if record.GitHub.SourceIssueNumber <= 0 {
		return nil, fmt.Errorf("missing source issue number")
	}
	client := agentosgh.NewClient(owner, name)
	return client.CreateIssueComment(record.GitHub.SourceIssueNumber, agentosgh.CreateIssueCommentRequest{Body: body})
}

func (s *Server) closeSourceIssueIfPolicyAllows(record *orchestrationRecord) {
	if record == nil || record.GitHub == nil || record.GitHub.SourceIssueNumber == 0 || record.GitHub.SourceIssueClosed {
		return
	}
	if !sourceIssueClosePolicyAllows(record) {
		return
	}
	owner, name, _, ok := githubRepoForAPI(record.GitHub.Repo)
	if !ok {
		record.GitHub.Error = "close source issue: invalid GitHub repository"
		s.auditGitHubArtifact(record, "github.issue.close", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	client := agentosgh.NewClient(owner, name)
	if _, err := client.CloseIssue(record.GitHub.SourceIssueNumber); err != nil {
		record.GitHub.Error = "close source issue: " + err.Error()
		s.auditGitHubArtifact(record, "github.issue.close", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	now := time.Now().UTC()
	record.GitHub.SourceIssueClosed = true
	record.GitHub.SourceIssueClosedAt = now.Format(time.RFC3339)
	record.GitHub.Error = ""
	appendOrchestrationEvent(record, "github.issue.closed", "", "Source Issue closed")
	record.UpdatedAt = now
	s.auditGitHubArtifact(record, "github.issue.close", auditOutcomeSuccess, record.GitHub.SourceIssueURL)
}

func sourceIssueClosePolicyAllows(record *orchestrationRecord) bool {
	if record == nil || record.GitHub == nil || record.Status != "completed" {
		return false
	}
	switch normalizeClosePolicy(record.GitHub.ClosePolicy) {
	case "on_quality_gate_pass":
		return orchestrationQualityGatePassed(record)
	case "after_human_approval":
		return record.GitHub.ApprovalStatus == "approved"
	default:
		return false
	}
}

func orchestrationQualityGatePassed(record *orchestrationRecord) bool {
	if record == nil || len(record.Results) == 0 {
		return false
	}
	for _, result := range record.Results {
		if !result.Success {
			return false
		}
		if result.QualityGate != nil && !result.QualityGate.Passed {
			return false
		}
	}
	return true
}

func approvalRejectionMessage(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "Approval rejected"
	}
	return "Approval rejected: " + reason
}

func sourceIssueStartCommentBody(record *orchestrationRecord) string {
	runRef := orchestrationRunReference(record)
	return strings.TrimSpace(fmt.Sprintf(`AgentOS orchestration started.

- Run: %s
- Status: %s
- Repository: %s
- Base branch: %s
- Strategy: %s
- Agents: %s

Task:
%s`, runRef, record.Status, record.Repo, record.BaseBranch, record.Strategy, strings.Join(record.Agents, ", "), record.Task))
}

func sourceIssueFinalCommentBody(record *orchestrationRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "AgentOS orchestration finished.\n\n")
	fmt.Fprintf(&b, "- Run: %s\n", orchestrationRunReference(record))
	fmt.Fprintf(&b, "- Status: %s\n", strings.TrimSpace(record.Status))
	if record.GitHub != nil && record.GitHub.PullRequestURL != "" {
		fmt.Fprintf(&b, "- Pull request: %s\n", record.GitHub.PullRequestURL)
	}
	if record.Error != "" {
		fmt.Fprintf(&b, "- Error: %s\n", record.Error)
	}
	if record.Summary != "" {
		fmt.Fprintf(&b, "\nSummary:\n%s\n", record.Summary)
	}
	return strings.TrimSpace(b.String())
}

func orchestrationRunReference(record *orchestrationRecord) string {
	if record == nil {
		return ""
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("AGENTOS_PUBLIC_URL")), "/")
	if base == "" {
		base = publicURLFromOAuthCallback()
	}
	if base == "" {
		return record.ID
	}
	return fmt.Sprintf("%s/#orchestrates/%s", base, record.ID)
}

func publicURLFromOAuthCallback() string {
	callback := strings.TrimSpace(os.Getenv("GITHUB_OAUTH_CALLBACK_URL"))
	if callback == "" {
		return ""
	}
	u, err := url.Parse(callback)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
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

func builtInScenarioTemplates(registry *agent.Registry) []scenarioTemplate {
	templates := []scenarioTemplate{
		{
			ID:                "go-http-service-bootstrap",
			Name:              "Go HTTP Service Bootstrap",
			Description:       "Create or extend a Go HTTP service while preserving repository layout.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "go-backend", "docs", "qa", "reviewer"),
			Strategy:          "sequential",
			CreatePullRequest: true,
			TaskTemplate: `Bootstrap or extend a Go HTTP service in {{repo}} on {{baseBranch}}.

Package or module focus: {{packageName}}
Endpoints or handlers: {{endpoints}}

Preserve the existing repository layout and conventions. Add or update tests, document how to run the service, and summarize validation.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
				{Name: "packageName", Label: "Package or module", Placeholder: "internal/server"},
				{Name: "endpoints", Label: "Endpoints", Placeholder: "GET /health, POST /items"},
			},
		},
		{
			ID:                "bug-fix-with-tests",
			Name:              "Bug Fix With Tests",
			Description:       "Fix a defect, add regression coverage, and review the result.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "go-backend", "qa", "reviewer"),
			Strategy:          "sequential",
			CreatePullRequest: true,
			TaskTemplate: `Fix the bug in {{repo}} on {{baseBranch}}.

Bug or issue: {{targetIssue}}
Expected behavior: {{expectedBehavior}}
Relevant files or components: {{scope}}

Add focused regression tests, keep the change minimal, and include validation results.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
				{Name: "targetIssue", Label: "Bug or issue", Placeholder: "Issue URL, number, or description", Required: true},
				{Name: "expectedBehavior", Label: "Expected behavior", Placeholder: "What should happen"},
				{Name: "scope", Label: "Files or components", Placeholder: "internal/foo, cmd/bar"},
			},
		},
		{
			ID:                "documentation-only-update",
			Name:              "Documentation-Only Update",
			Description:       "Update README or docs without code changes unless needed for examples.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "docs", "reviewer"),
			Strategy:          "sequential",
			CreatePullRequest: true,
			TaskTemplate: `Update documentation in {{repo}} on {{baseBranch}}.

Documentation target: {{docTarget}}
Audience or use case: {{audience}}
Required details: {{details}}

Match existing documentation style. Keep commands copy-pasteable and avoid unrelated code changes.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
				{Name: "docTarget", Label: "Doc target", Placeholder: "README.md, docs/deployment.md", Required: true},
				{Name: "audience", Label: "Audience", Placeholder: "operators, contributors, API users"},
				{Name: "details", Label: "Required details", Placeholder: "configuration, examples, troubleshooting"},
			},
		},
		{
			ID:                "ci-failure-fixer",
			Name:              "CI Failure Fixer",
			Description:       "Diagnose and fix failing CI with local validation.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "ci-fixer", "qa", "reviewer"),
			Strategy:          "sequential",
			CreatePullRequest: true,
			TaskTemplate: `Fix CI failures for {{repo}} on {{baseBranch}}.

Workflow or check: {{workflow}}
Failure URL or log excerpt: {{failure}}

Preserve existing workflow intent, mirror CI validation locally where practical, and summarize the root cause.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
				{Name: "workflow", Label: "Workflow or check", Placeholder: "CI / lint"},
				{Name: "failure", Label: "Failure detail", Placeholder: "Actions URL or error excerpt", Required: true},
			},
		},
		{
			ID:                "security-remediation",
			Name:              "Security Remediation",
			Description:       "Address security or code-scanning findings with validation notes.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "security", "go-backend", "qa", "reviewer"),
			Strategy:          "sequential",
			CreatePullRequest: true,
			RequireApproval:   true,
			TaskTemplate: `Remediate the security finding in {{repo}} on {{baseBranch}}.

Finding: {{finding}}
Affected area: {{scope}}
Required constraints: {{constraints}}

Prefer narrow defensive fixes, add tests or manual verification notes, and document residual risk.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
				{Name: "finding", Label: "Finding", Placeholder: "CodeQL alert, dependency advisory, or description", Required: true},
				{Name: "scope", Label: "Affected area", Placeholder: "auth/session/dependencies"},
				{Name: "constraints", Label: "Constraints", Placeholder: "No dependency upgrades beyond patch releases"},
			},
		},
		{
			ID:                "release-preparation",
			Name:              "Release Preparation",
			Description:       "Prepare changelog, checklist, and release readiness updates.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "release-manager", "docs", "qa", "reviewer"),
			Strategy:          "sequential",
			CreatePullRequest: true,
			RequireApproval:   true,
			TaskTemplate: `Prepare release materials for {{repo}} on {{baseBranch}}.

Release version: {{version}}
Scope since: {{since}}
Required artifacts: {{artifacts}}

Update changelog or release docs according to repository conventions. Include validation, known gaps, and rollback considerations.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
				{Name: "version", Label: "Version", Placeholder: "v1.2.0", Required: true},
				{Name: "since", Label: "Scope since", Placeholder: "v1.1.0 or commit SHA"},
				{Name: "artifacts", Label: "Artifacts", Placeholder: "CHANGELOG.md, upgrade guide, chart values"},
			},
		},
		{
			ID:                "frontend-ui-change",
			Name:              "Frontend UI Change",
			Description:       "Implement a focused UI change with responsive and accessibility checks.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "go-backend", "qa", "reviewer"),
			Strategy:          "sequential",
			CreatePullRequest: true,
			TaskTemplate: `Implement the frontend UI change in {{repo}} on {{baseBranch}}.

Screen or flow: {{screen}}
Change requested: {{change}}
Validation target: {{validation}}

Follow existing frontend conventions, keep text and controls responsive, and include browser or build verification notes.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
				{Name: "screen", Label: "Screen or flow", Placeholder: "Dashboard, New Orchestration"},
				{Name: "change", Label: "Change requested", Placeholder: "Add filter controls", Required: true},
				{Name: "validation", Label: "Validation target", Placeholder: "desktop/mobile screenshots, npm test"},
			},
		},
	}
	return templates
}

func availableAgentNames(registry *agent.Registry, names ...string) []string {
	var available []string
	for _, name := range names {
		if registry == nil || registry.Has(name) {
			available = append(available, name)
		}
	}
	return available
}

func loadRepositoryScenarioTemplates(repoPath string, registry *agent.Registry) ([]scenarioTemplate, error) {
	dir := filepath.Join(repoPath, ".agentos", "scenarios")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []scenarioTemplate{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read .agentos/scenarios: %w", err)
	}

	var templates []scenarioTemplate
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: read: %w", filepath.ToSlash(filepath.Join(".agentos", "scenarios", entry.Name())), err)
		}
		var tmpl scenarioTemplate
		if err := yaml.Unmarshal(raw, &tmpl); err != nil {
			return nil, fmt.Errorf("%s: parse: %w", filepath.ToSlash(filepath.Join(".agentos", "scenarios", entry.Name())), err)
		}
		tmpl.Source = "repository"
		if err := validateScenarioTemplate(&tmpl, registry); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.ToSlash(filepath.Join(".agentos", "scenarios", entry.Name())), err)
		}
		templates = append(templates, tmpl)
	}
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].ID < templates[j].ID
	})
	return templates, nil
}

func validateScenarioTemplate(tmpl *scenarioTemplate, registry *agent.Registry) error {
	tmpl.ID = strings.TrimSpace(tmpl.ID)
	tmpl.Name = strings.TrimSpace(tmpl.Name)
	tmpl.Strategy = strings.TrimSpace(tmpl.Strategy)
	if tmpl.Strategy == "" {
		tmpl.Strategy = "sequential"
	}
	if tmpl.ID == "" {
		return fmt.Errorf("id is required")
	}
	if !customAgentNamePattern.MatchString(tmpl.ID) {
		return fmt.Errorf("id must match %s", customAgentNamePattern.String())
	}
	if tmpl.Name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(tmpl.TaskTemplate) == "" {
		return fmt.Errorf("taskTemplate is required")
	}
	if tmpl.Strategy != "sequential" && tmpl.Strategy != "parallel" {
		return fmt.Errorf("strategy must be sequential or parallel")
	}
	if len(tmpl.Agents) == 0 {
		return fmt.Errorf("agents is required")
	}
	for _, name := range tmpl.Agents {
		if registry != nil && !registry.Has(name) {
			return fmt.Errorf("unknown agent %q", name)
		}
	}
	seenVars := map[string]bool{}
	for i := range tmpl.Variables {
		name := strings.TrimSpace(tmpl.Variables[i].Name)
		tmpl.Variables[i].Name = name
		if name == "" {
			return fmt.Errorf("variables[%d].name is required", i)
		}
		if !scenarioVariableNamePattern.MatchString(name) {
			return fmt.Errorf("variable %q must match %s", name, scenarioVariableNamePattern.String())
		}
		if seenVars[name] {
			return fmt.Errorf("duplicate variable %q", name)
		}
		seenVars[name] = true
	}
	return nil
}

func resolveScenarioTemplateSelection(selection *scenarioTemplateSelection, repoPath string, registry *agent.Registry) *scenarioTemplateSelection {
	if selection == nil || strings.TrimSpace(selection.ID) == "" {
		return nil
	}
	id := strings.TrimSpace(selection.ID)
	builtIns := builtInScenarioTemplates(registry)
	for i := range builtIns {
		tmpl := builtIns[i]
		if tmpl.ID == id {
			return &scenarioTemplateSelection{ID: tmpl.ID, Name: tmpl.Name, Source: tmpl.Source}
		}
	}
	if repoPath != "" {
		templates, err := loadRepositoryScenarioTemplates(repoPath, registry)
		if err == nil {
			for i := range templates {
				tmpl := templates[i]
				if tmpl.ID == id {
					return &scenarioTemplateSelection{ID: tmpl.ID, Name: tmpl.Name, Source: tmpl.Source}
				}
			}
		}
	}
	return &scenarioTemplateSelection{ID: id, Name: strings.TrimSpace(selection.Name), Source: strings.TrimSpace(selection.Source)}
}

func loadRepositoryAgentDefinitions(repoPath string, registry *agent.Registry) ([]agent.Definition, error) {
	dir := filepath.Join(repoPath, ".agentos", "agents")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []agent.Definition{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read .agentos/agents: %w", err)
	}

	var defs []agent.Definition
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		def, err := agent.LoadDefinition(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.ToSlash(filepath.Join(".agentos", "agents", entry.Name())), err)
		}
		defs = append(defs, *def)
	}

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Metadata.Name < defs[j].Metadata.Name
	})
	return validateCustomAgentDefinitions(defs, registry)
}

func validateCustomAgentDefinitions(defs []agent.Definition, registry *agent.Registry) ([]agent.Definition, error) {
	seen := make(map[string]bool, len(defs))
	validated := make([]agent.Definition, 0, len(defs))
	for i := range defs {
		def := defs[i]
		if err := def.Validate(); err != nil {
			return nil, err
		}
		name := strings.TrimSpace(def.Metadata.Name)
		def.Metadata.Name = name
		if !customAgentNamePattern.MatchString(name) {
			return nil, fmt.Errorf("%s: metadata.name must match %s", name, customAgentNamePattern.String())
		}
		if registry != nil && registry.Has(name) {
			return nil, fmt.Errorf("%s: custom agent cannot override a built-in agent", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("%s: duplicate custom agent name", name)
		}
		seen[name] = true
		if err := validateCustomAgentTools(&def); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if err := validateCustomAgentCommands(&def); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		validated = append(validated, def)
	}
	return validated, nil
}

func validateCustomAgentTools(def *agent.Definition) error {
	allowedTools := map[string]bool{
		"read_file":  true,
		"write_file": true,
		"search":     true,
		"shell":      true,
		"git":        true,
		"test":       true,
	}
	if len(def.Spec.Tools.Allow) == 0 {
		return fmt.Errorf("spec.tools.allow is required for repository-defined agents")
	}
	hasShell := false
	for _, tool := range def.Spec.Tools.Allow {
		if !allowedTools[tool] {
			return fmt.Errorf("unsupported tool %q", tool)
		}
		if tool == "shell" {
			hasShell = true
		}
	}
	if hasShell && len(def.Spec.Safety.DenyCommands) == 0 {
		return fmt.Errorf("spec.safety.denyCommands is required when shell is allowed")
	}
	return nil
}

func validateCustomAgentCommands(def *agent.Definition) error {
	commands := []string{def.Spec.Commands.Test, def.Spec.Commands.Lint, def.Spec.Commands.Build}
	blocked := []string{"rm -rf", "sudo", "curl ", "wget ", "ssh ", "scp ", "docker run --privileged"}
	for _, command := range commands {
		normalized := strings.ToLower(strings.TrimSpace(command))
		if normalized == "" {
			continue
		}
		for _, pattern := range blocked {
			if strings.Contains(normalized, pattern) {
				return fmt.Errorf("unsafe command %q contains blocked pattern %q", command, pattern)
			}
		}
	}
	return nil
}

func selectedCustomAgentDefinitions(agentNames []string, customByName map[string]agent.Definition) []agent.Definition {
	var selected []agent.Definition
	for _, name := range agentNames {
		if def, ok := customByName[name]; ok {
			selected = append(selected, def)
		}
	}
	return selected
}

func orchestrationAgentProfiles(record *orchestrationRecord, agents map[string]runtime.Agent) map[string]profile.Profile {
	profiles := make(map[string]profile.Profile, len(record.CustomAgents))
	for i := range record.CustomAgents {
		def := &record.CustomAgents[i]
		prof := factory.ProfileFromDefinition(def)
		profiles[def.Metadata.Name] = *prof
	}
	return profiles
}

func orchestrationAgentMetadata(record *orchestrationRecord, agents map[string]runtime.Agent, registry *agent.Registry) []orchestrator.AgentMetadata {
	infoByName := make(map[string]agent.Info)
	if registry != nil {
		infos := registry.List()
		for i := range infos {
			info := infos[i]
			infoByName[info.Name] = info
		}
	}
	customByName := make(map[string]agent.Definition, len(record.CustomAgents))
	for i := range record.CustomAgents {
		def := record.CustomAgents[i]
		customByName[def.Metadata.Name] = def
	}

	var metadata []orchestrator.AgentMetadata
	for _, name := range record.Agents {
		if def, ok := customByName[name]; ok {
			metadata = append(metadata, customAgentMetadata(&def))
			continue
		}
		if info, ok := infoByName[name]; ok {
			metadata = append(metadata, orchestrator.AgentMetadata{
				Name:                 info.Name,
				Description:          info.Description,
				Domains:              info.Domains,
				TriggerKeywords:      info.TriggerKeywords,
				TriggerFiles:         info.TriggerFiles,
				RecommendedAfter:     info.RecommendedAfter,
				ArchitectureGuidance: info.ArchitectureGuidance,
				OutputExpectations:   info.OutputExpectations,
			})
			continue
		}
		if agt, ok := agents[name]; ok {
			metadata = append(metadata, orchestrator.AgentMetadata{Name: name, Description: agt.Name()})
		}
	}
	return metadata
}

func customAgentMetadata(def *agent.Definition) orchestrator.AgentMetadata {
	role := strings.TrimSpace(def.Metadata.Labels["role"])
	if role == "" {
		role = "repository-defined custom agent"
	}
	var triggers []string
	if def.Metadata.Labels["role"] != "" {
		triggers = append(triggers, def.Metadata.Labels["role"])
	}
	return orchestrator.AgentMetadata{
		Name:                 def.Metadata.Name,
		Description:          role,
		Domains:              []string{"repository-custom"},
		TriggerKeywords:      triggers,
		ArchitectureGuidance: def.Spec.Guidance.Architecture,
		OutputExpectations:   def.Spec.Guidance.OutputExpectations,
	}
}

func recommendRepoSignals(repo, baseBranch string) []string {
	repo = strings.TrimSpace(repo)
	var repoPath string
	if repo == "" || repo == "." {
		wd, err := os.Getwd()
		if err != nil {
			return nil
		}
		repoPath = wd
	} else {
		resolved, err := resolveOrchestrateRepo(repo, defaultBaseBranch(baseBranch))
		if err != nil {
			return nil
		}
		repoPath = resolved
	}
	checks := map[string]string{
		"package.json":        "frontend",
		"vite.config.ts":      "frontend",
		"vite.config.js":      "frontend",
		"next.config.js":      "frontend",
		"next.config.mjs":     "frontend",
		"nuxt.config.ts":      "frontend",
		"svelte.config.js":    "frontend",
		"tailwind.config.js":  "frontend",
		"index.html":          "frontend",
		"go.mod":              "backend",
		"Dockerfile":          "ops",
		"docker-compose.yaml": "ops",
		"charts":              "ops",
		".github/workflows":   "ci",
		"README.md":           "docs",
		"SECURITY.md":         "security",
		"go.sum":              "dependency",
		"package-lock.json":   "dependency",
		"pnpm-lock.yaml":      "dependency",
		"yarn.lock":           "dependency",
	}
	seen := map[string]bool{}
	for path, signal := range checks {
		if _, err := os.Stat(filepath.Join(repoPath, path)); err == nil {
			seen[signal] = true
		}
	}
	signals := make([]string, 0, len(seen))
	for signal := range seen {
		signals = append(signals, signal)
	}
	sort.Strings(signals)
	return signals
}

func recommendOrchestration(task string, repoSignals []string, registry *agent.Registry) orchestrationRecommendation {
	text := strings.ToLower(task + " " + strings.Join(repoSignals, " "))
	preset, confidence, rationale := classifyOrchestrationTask(text)
	rec := orchestrationRecommendation{
		Preset:            preset,
		Confidence:        confidence,
		Rationale:         rationale,
		Agents:            recommendAgentsForPreset(preset, registry),
		Strategy:          recommendStrategyForPreset(preset),
		CreatePullRequest: recommendCreatePullRequest(preset),
		RequireApproval:   recommendApprovalForPreset(preset),
	}
	if len(rec.Agents) == 0 && registry != nil {
		infos := registry.List()
		for i := range infos {
			rec.Agents = append(rec.Agents, infos[i].Name)
			break
		}
	}
	return rec
}

func classifyOrchestrationTask(text string) (preset string, confidence float64, rationale string) {
	rules := []struct {
		preset     string
		confidence float64
		rationale  string
		keywords   []string
	}{
		{"security", 0.88, "Security-related terms were detected.", []string{"security", "vulnerability", "cve", "secret", "xss", "csrf", "sql injection", "permission", "authz"}},
		{"release", 0.87, "Release preparation terms were detected.", []string{"release", "changelog", "version bump", "release tag", "release notes", "rollback"}},
		{"ci-fix", 0.86, "CI or workflow failure terms were detected.", []string{"github actions", "continuous integration", "workflow", "check failed", "failing test", "lint", "build failure"}},
		{"qa", 0.85, "QA or verification terms were detected.", []string{"qa", "quality assurance", "smoke test", "scenario test", "regression test", "manual verification"}},
		{"ops", 0.84, "Docker, Helm, Kubernetes, or deployment terms were detected.", []string{"docker", "helm", "kubernetes", "k8s", "deployment", "ingress", "container", "cluster", "ops"}},
		{"frontend", 0.82, "Frontend UI terms or frontend repository files were detected.", []string{"frontend", "react", "tailwind", "css", "responsive", "browser", "vite"}},
		{"docs", 0.80, "Documentation terms were detected.", []string{"docs", "documentation", "readme", "guide", "manual", "changelog"}},
		{"dependency", 0.78, "Dependency update terms or lockfiles were detected.", []string{"dependency", "dependencies", "upgrade", "bump", "go.sum", "package-lock", "pnpm-lock", "yarn.lock"}},
		{"reporting", 0.76, "Investigation or report-only terms were detected.", []string{"investigate", "analysis", "report", "summarize", "research", "audit"}},
		{"backend", 0.74, "Backend service terms or Go repository files were detected.", []string{"backend", "api", "server", "handler", "endpoint", "database", "go.mod"}},
		{"bugfix", 0.72, "Bug or regression terms were detected.", []string{"bug", "fix", "regression", "error", "panic", "crash", "broken"}},
	}
	for _, rule := range rules {
		for _, keyword := range rule.keywords {
			if strings.Contains(text, keyword) {
				return rule.preset, rule.confidence, rule.rationale
			}
		}
	}
	return "general", 0.55, "No strong task-specific signal was detected; using the general implementation preset."
}

func recommendAgentsForPreset(preset string, registry *agent.Registry) []string {
	candidates := map[string][]string{
		"security":   {"security", "reviewer"},
		"ci-fix":     {"ci-fixer", "reviewer"},
		"ops":        {"devops", "docker", "helm", "kubernetes", "release-manager", "security", "qa", "reviewer"},
		"frontend":   {"frontend", "frontend-app", "qa", "reviewer"},
		"docs":       {"docs", "reviewer"},
		"dependency": {"dependency-updater", "ci-fixer", "reviewer"},
		"reporting":  {"docs", "reviewer"},
		"release":    {"release-manager", "docs", "qa", "reviewer"},
		"qa":         {"qa", "reviewer"},
		"backend":    {"go-backend", "reviewer"},
		"bugfix":     {"go-backend", "reviewer"},
		"general":    {"go-backend", "reviewer"},
	}
	names := candidates[preset]
	if len(names) == 0 {
		names = candidates["general"]
	}
	agents := make([]string, 0, len(names))
	for _, name := range names {
		if registry == nil || registry.Has(name) {
			agents = append(agents, name)
		}
	}
	return agents
}

func recommendStrategyForPreset(preset string) string {
	switch preset {
	case "reporting", "docs":
		return "sequential"
	default:
		return "sequential"
	}
}

func recommendCreatePullRequest(preset string) bool {
	switch preset {
	case "reporting":
		return false
	default:
		return true
	}
}

func recommendApprovalForPreset(preset string) bool {
	switch preset {
	case "security", "ops", "dependency", "release":
		return true
	default:
		return false
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
