// Copyright 2026 ARUN Authors
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

	"github.com/hakobune8/arun/internal/agent"
	"github.com/hakobune8/arun/internal/apphome"
	"github.com/hakobune8/arun/internal/embedding"
	"github.com/hakobune8/arun/internal/factory"
	arungh "github.com/hakobune8/arun/internal/github"
	"github.com/hakobune8/arun/internal/guideline"
	"github.com/hakobune8/arun/internal/llm"
	"github.com/hakobune8/arun/internal/memory"
	"github.com/hakobune8/arun/internal/orchestrator"
	"github.com/hakobune8/arun/internal/profile"
	"github.com/hakobune8/arun/internal/runtime"
	"github.com/hakobune8/arun/internal/safety"
	"github.com/hakobune8/arun/internal/sandbox"
	"github.com/hakobune8/arun/internal/search"
	"github.com/hakobune8/arun/internal/task"
	"github.com/hakobune8/arun/internal/vector"
	"gopkg.in/yaml.v3"
)

//go:embed static
var staticFS embed.FS

var runIDPattern = regexp.MustCompile(`^run-[0-9a-f]{16}$`)
var githubRepoPathPattern = regexp.MustCompile(`^[A-Za-z0-9-]+/[A-Za-z0-9._-]+$`)
var gitRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)
var gitObjectPattern = regexp.MustCompile(`^[0-9a-f]{40,64}$`)
var customAgentNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,62}$`)
var scenarioVariableNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,62}$`)

// Server serves the ARUN web UI and API endpoints.
type Server struct {
	port            int
	server          *http.Server
	search          *search.Service
	agentReg        *agent.Registry
	llmClient       llm.LLMClient
	sandbox         sandbox.Sandbox
	runtimeCfg      *runtime.Config
	auth            authConfig
	llmSettings     llmSettings
	activeMu        sync.Mutex
	activeRuns      map[string]context.CancelFunc
	canceledRun     map[string]bool
	schedulerMu     sync.Mutex
	schedulerCancel context.CancelFunc
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
	mux.HandleFunc("/api/notifications", s.handleNotifications)
	mux.HandleFunc("/api/notifications/", s.handleNotificationDetail)
	mux.HandleFunc("/api/storage", s.handleStorage)
	mux.HandleFunc("/api/storage/", s.handleStorageDetail)
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
	mux.HandleFunc("/api/schedules", s.handleSchedules)
	mux.HandleFunc("/api/schedules/", s.handleScheduleDetail)

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
	slog.Info("ARUN Web UI starting", "port", s.port)
	s.startScheduler()
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.stopScheduler()
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
	agents = localizeAgentInfos(agents, r.URL.Query().Get("uiLanguage"))
	_ = json.NewEncoder(w).Encode(agents) //nolint:errcheck // best-effort
}

func localizeAgentInfos(agents []agent.Info, language string) []agent.Info {
	if strings.TrimSpace(language) != "ja" {
		return agents
	}
	descriptions := map[string]string{
		"analyst":            "Analyst agent — logs、runs、artifacts、GitHub context、repository evidence を調査し、findings と next actions を整理します",
		"ci-fixer":           "CI fix agent — CI failure を分析し、GitHub Actions と validation の修正を行います",
		"dependency-updater": "Dependency updater agent — Go modules、lockfiles、GitHub Actions versions を互換性確認付きで更新します",
		"devops":             "DevOps agent — Docker、Helm、Kubernetes、deployment debugging、release hardening を横断調整します",
		"docker":             "Docker ops agent — Dockerfile、image build、.dockerignore、runtime config、container safety defaults を保守します",
		"docs":               "Documentation agent — 既存 style に合わせて実用的な repository documentation を作成・更新します",
		"frontend":           "Frontend agent — UI 実装、layout、responsive、accessibility、frontend validation を担当します",
		"go-backend":         "Go backend agent — 既存構成を保ちながら Go の設計、実装、テスト、lint を担当します",
		"helm":               "Helm ops agent — charts、templates、values、schema、chart linting、release-safe packaging を保守します",
		"kubernetes":         "Kubernetes ops agent — manifests、deployments、services、ingress、probes、resources、rollout checks を保守します",
		"qa":                 "QA agent — scenario tests、smoke checks、regression coverage、manual verification notes を追加します",
		"release-manager":    "Release manager agent — changelog、release notes、checklist、readiness validation を準備します",
		"reporter":           "Reporter agent — findings を Markdown reports、stakeholder summaries、GitHub-ready updates に整理します",
		"reviewer":           "Review agent — correctness、tests、security、maintainability、release readiness の観点で diff をレビューします",
		"security":           "Security agent — dependencies、auth/session、secrets、security-sensitive diffs をレビューします",
	}
	localized := make([]agent.Info, len(agents))
	copy(localized, agents)
	for i := range localized {
		if description, ok := descriptions[localized[i].Name]; ok {
			localized[i].Description = description
		}
	}
	return localized
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
		repoPath, err := resolveOrchestrateRepoWithToken(req.Repo, req.BaseBranch, userGitHubToken(user))
		if err != nil {
			slog.Warn("skip repository scenario templates", "repo", req.Repo, "error", err)
		} else {
			repoTemplates, err := loadRepositoryScenarioTemplates(repoPath, s.agentReg)
			if err != nil {
				slog.Warn("skip invalid repository scenario templates", "repo", req.Repo, "error", err)
			} else {
				templates = append(templates, repoTemplates...)
			}
		}
	}
	localizeScenarioTemplates(templates, req.UILanguage)
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
		Branch:      "arun/" + id,
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
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query param q required", http.StatusBadRequest)
		return
	}
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if repo != "" {
		branch := defaultBaseBranch(r.URL.Query().Get("baseBranch"))
		if !s.requireAutomationPermission(w, r, user, "search.repository", "repository", repo, "") {
			return
		}
		results, err := repositoryContextSearch(r.Context(), repositoryContextSearchQuery{
			Repo:   repo,
			Branch: branch,
			Query:  query,
			Source: r.URL.Query().Get("source"),
			Limit:  parsePositiveInt(r.URL.Query().Get("limit"), 50),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(results) //nolint:errcheck // best-effort
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
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	path := r.URL.Path[len("/api/github/"):]
	if path == "repositories" {
		client := arungh.NewClient("", "")
		if user != nil && user.AccessToken != "" {
			repos, err := client.WithToken(user.AccessToken).ListUserRepositories()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			privateCount := 0
			for _, repo := range repos {
				if repo.Private {
					privateCount++
				}
			}
			slog.Info("GitHub OAuth repositories listed", "login", user.Login, "count", len(repos), "private", privateCount)
			if repos == nil {
				repos = []arungh.RepositorySummary{}
			}
			_ = json.NewEncoder(w).Encode(repos) //nolint:errcheck // best-effort response
			return
		}
		repos, err := client.ListRepositories()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if repos == nil {
			repos = []arungh.RepositorySummary{}
		}
		_ = json.NewEncoder(w).Encode(repos) //nolint:errcheck // best-effort response
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
	client := arungh.NewClient(parts[0], parts[1])

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
			issues = []arungh.Issue{}
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
			prs = []arungh.PullRequest{}
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
			runs = []arungh.CheckRun{}
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
	Limits         governanceLimits           `json:"limits,omitempty"`
}

type orchestrationStartOptions struct {
	Source      *orchestrationSourceIssue
	ScheduleID  string
	Actor       string
	GitHubToken string
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
	UILanguage string `json:"uiLanguage"`
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
	OutputLanguage    string                     `json:"outputLanguage,omitempty" yaml:"outputLanguage,omitempty"`
	Agents            []string                   `json:"agents" yaml:"agents"`
	Strategy          string                     `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	CreateIssue       bool                       `json:"createIssue,omitempty" yaml:"createIssue,omitempty"`
	CreatePullRequest bool                       `json:"createPullRequest,omitempty" yaml:"createPullRequest,omitempty"`
	RequireApproval   bool                       `json:"requireApproval,omitempty" yaml:"requireApproval,omitempty"`
	Limits            governanceLimits           `json:"limits,omitempty" yaml:"limits,omitempty"`
	TaskTemplate      string                     `json:"taskTemplate" yaml:"taskTemplate"`
	Variables         []scenarioTemplateVariable `json:"variables,omitempty" yaml:"variables,omitempty"`
}

type orchestrationStagePreset struct {
	Stage    string `json:"stage"`
	Agent    string `json:"agent,omitempty"`
	PresetID string `json:"presetId"`
	Fallback bool   `json:"fallback,omitempty"`
	Reason   string `json:"reason,omitempty"`
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
	ScheduleID               string                                 `json:"scheduleId,omitempty"`
	Strategy                 string                                 `json:"strategy"`
	LLMPreset                string                                 `json:"llmPreset"`
	StagePresets             []orchestrationStagePreset             `json:"stagePresets,omitempty"`
	OutputLanguage           string                                 `json:"outputLanguage,omitempty"`
	Limits                   governanceLimits                       `json:"limits,omitempty"`
	Usage                    governanceUsage                        `json:"usage,omitempty"`
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
	GitHubToken              string                                 `json:"-"`
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
	if req != nil {
		req.Repo = strings.TrimSpace(req.Repo)
	}
	if !s.requireAutomationPermission(w, r, user, "orchestrate.create", "orchestration", reqRepo(req), "") {
		return
	}
	record, err := s.createOrchestration(req, orchestrationStartOptions{Source: source, Actor: actorLogin(user), GitHubToken: userGitHubToken(user)})
	if err != nil {
		http.Error(w, err.Error(), orchestrationStartStatus(err))
		return
	}
	_ = json.NewEncoder(w).Encode(record) //nolint:errcheck // best-effort response
}

func (s *Server) createOrchestration(req *orchestrateRequest, opts orchestrationStartOptions) (*orchestrationRecord, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	req.Repo = strings.TrimSpace(req.Repo)
	req.BaseBranch = defaultBaseBranch(req.BaseBranch)
	req.Strategy = strings.TrimSpace(req.Strategy)
	if req.Strategy == "" {
		req.Strategy = "sequential"
	}
	if req.Strategy != "sequential" && req.Strategy != "parallel" {
		return nil, fmt.Errorf("strategy must be sequential or parallel")
	}

	if len(req.Agents) == 0 || req.Task == "" {
		return nil, fmt.Errorf("agents and task are required")
	}
	limits, err := normalizeGovernanceLimits(req.Limits)
	if err != nil {
		return nil, err
	}
	if err := s.enforceGovernanceBeforeStart(req, limits); err != nil {
		return nil, err
	}

	llmClient, presetID, err := s.llmClientForPreset(req.LLMPreset)
	if err != nil {
		return nil, err
	}

	repoPath, err := resolveOrchestrateRepoWithToken(req.Repo, req.BaseBranch, opts.GitHubToken)
	if err != nil {
		return nil, err
	}
	artifactConfig := loadArtifactConfig(repoPath)
	applyArtifactConfig(req, artifactConfig)
	scenarioSelection := resolveScenarioTemplateSelection(req.Scenario, repoPath, s.agentReg)
	stageRouting := s.resolveOrchestrationStagePresets(req.Agents, presetID)
	planningClient := llmClient
	if stageRouting.enabled {
		planningClient = stageRouting.clientForStage("planning", llmClient)
	}

	agents := make(map[string]runtime.Agent)
	customAgents, err := validateCustomAgentDefinitions(req.CustomAgents, s.agentReg)
	if err != nil {
		return nil, fmt.Errorf("custom agents: %w", err)
	}
	customByName := make(map[string]agent.Definition, len(customAgents))
	for i := range customAgents {
		def := customAgents[i]
		customByName[def.Metadata.Name] = def
	}
	for _, name := range req.Agents {
		agentClient := llmClient
		if stageRouting.enabled {
			agentClient = stageRouting.clientForAgent(name, llmClient)
		}
		if def, ok := customByName[name]; ok {
			agents[name] = agent.NewBaseAgent(def.Metadata.Name, agentClient)
		} else {
			a, err := s.agentReg.Create(name, agentClient)
			if err != nil {
				return nil, fmt.Errorf("lookup agent %s: %w", name, err)
			}
			agents[name] = a
		}
	}

	id := generateID()
	githubState, err := prepareOrchestrationGitHub(id, req)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	githubState = attachSourceIssue(githubState, opts.Source)

	now := time.Now().UTC()
	actor := strings.TrimSpace(opts.Actor)
	if actor == "" {
		actor = "system"
	}
	record := &orchestrationRecord{
		ID:             id,
		Actor:          actor,
		Repo:           req.Repo,
		RepoPath:       repoPath,
		BaseBranch:     req.BaseBranch,
		Task:           req.Task,
		Agents:         req.Agents,
		CustomAgents:   selectedCustomAgentDefinitions(req.Agents, customByName),
		Scenario:       scenarioSelection,
		ScheduleID:     opts.ScheduleID,
		Strategy:       req.Strategy,
		LLMPreset:      presetID,
		StagePresets:   stageRouting.records,
		OutputLanguage: normalizeOutputLanguage(req.OutputLanguage),
		Limits:         limits,
		Status:         "planning",
		GitHub:         githubState,
		GitHubToken:    strings.TrimSpace(opts.GitHubToken),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	appendOrchestrationEvent(record, "created", "", "Orchestration created")
	for _, entry := range stageRouting.records {
		label := entry.Stage
		if entry.Agent != "" {
			label = fmt.Sprintf("%s/%s", entry.Agent, entry.Stage)
		}
		if entry.Fallback {
			appendOrchestrationEvent(record, "preset.fallback", "", fmt.Sprintf("%s uses %s: %s", label, entry.PresetID, entry.Reason))
		} else {
			appendOrchestrationEvent(record, "preset.selected", "", fmt.Sprintf("%s uses %s", label, entry.PresetID))
		}
	}
	initializeGovernanceUsage(record)
	if record.Repo == "" {
		record.Repo = "."
	}
	if err := saveOrchestrationRecord(record); err != nil {
		return nil, fmt.Errorf("save orchestration: %w", err)
	}

	go s.runOrchestration(record, agents, planningClient)

	return record, nil
}

func reqRepo(req *orchestrateRequest) string {
	if req == nil {
		return ""
	}
	return strings.TrimSpace(req.Repo)
}

type orchestrationStagePresetRouting struct {
	enabled bool
	records []orchestrationStagePreset
	clients map[string]llm.LLMClient
}

func (s *Server) resolveOrchestrationStagePresets(agentNames []string, fallbackPreset string) orchestrationStagePresetRouting {
	routing := orchestrationStagePresetRouting{
		enabled: true,
		clients: map[string]llm.LLMClient{},
	}
	specs := []orchestrationStagePreset{{Stage: "planning", PresetID: "planning"}}
	seenAgents := map[string]bool{}
	for _, agentName := range agentNames {
		agentName = strings.TrimSpace(agentName)
		if agentName == "" || seenAgents[agentName] {
			continue
		}
		seenAgents[agentName] = true
		specs = append(specs, orchestrationStagePreset{
			Stage:    orchestrationPresetStageForAgent(agentName),
			Agent:    agentName,
			PresetID: orchestrationPresetForAgent(agentName),
		})
	}
	for _, spec := range specs {
		presetID := spec.PresetID
		client, resolved, err := s.llmClientForPreset(presetID)
		entry := orchestrationStagePreset{
			Stage:    spec.Stage,
			Agent:    spec.Agent,
			PresetID: resolved,
		}
		if err != nil {
			client, resolved, err = s.llmClientForPreset(fallbackPreset)
			entry.PresetID = resolved
			entry.Fallback = true
			entry.Reason = fmt.Sprintf("preset %q is not configured", presetID)
		}
		if err != nil {
			entry.PresetID = fallbackPreset
			entry.Fallback = true
			entry.Reason = err.Error()
		} else {
			routing.clients[spec.Stage] = client
			if spec.Agent != "" {
				routing.clients["agent:"+spec.Agent] = client
			}
		}
		routing.records = append(routing.records, entry)
	}
	return routing
}

func (r orchestrationStagePresetRouting) clientForStage(stage string, fallback llm.LLMClient) llm.LLMClient {
	if r.clients != nil {
		if client := r.clients[stage]; client != nil {
			return client
		}
	}
	return fallback
}

func (r orchestrationStagePresetRouting) clientForAgent(agentName string, fallback llm.LLMClient) llm.LLMClient {
	if r.clients != nil {
		if client := r.clients["agent:"+agentName]; client != nil {
			return client
		}
	}
	return fallback
}

func orchestrationPresetForAgent(agentName string) string {
	switch agentName {
	case "analyst":
		return "planning"
	case "reviewer":
		return "review"
	case "qa":
		return "smoke"
	case "reporter":
		return "reporting"
	default:
		return "coding"
	}
}

func orchestrationPresetStageForAgent(agentName string) string {
	switch orchestrationPresetForAgent(agentName) {
	case "planning":
		return "planning"
	case "review":
		return "review"
	case "smoke":
		return "smoke"
	case "reporting":
		return "reporting"
	default:
		return "coding"
	}
}

func orchestrationStartStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if strings.HasPrefix(err.Error(), "save orchestration:") {
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

func (s *Server) runOrchestration(record *orchestrationRecord, agents map[string]runtime.Agent, llmClient llm.LLMClient) {
	baseCtx, cancelBase := context.WithCancel(context.Background())
	defer cancelBase()
	runCtx := baseCtx
	cancelBudget := func() {}
	if timeout := record.Limits.maxDuration(); timeout > 0 {
		runCtx, cancelBudget = context.WithTimeout(baseCtx, timeout)
	}
	defer cancelBudget()
	s.registerActiveOrchestration(record.ID, cancelBase)
	defer s.unregisterActiveOrchestration(record.ID)

	cfg := &runtime.Config{Verbose: false}
	orch := orchestrator.NewOrchestrator(llmClient, sandbox.NewLocalSandbox(record.RepoPath), agents, cfg)
	orch.SetAgentMetadata(orchestrationAgentMetadata(record, agents, s.agentReg), orchestrationAgentProfiles(record, agents))
	orch.SetBaseBranch(record.BaseBranch)
	orch.SetRunID(record.ID)
	orch.SetMaxRetries(record.Limits.MaxRetries)

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

	var plan *orchestrator.TaskPlan
	if usesImplementationHeavyScrumPlan(record) {
		plan = implementationHeavyScrumPlan(record)
		appendOrchestrationEvent(record, "planning.template", "", "Using implementation-heavy scrum sprint-stage workflow")
	} else {
		planCtx, cancelPlan := context.WithTimeout(runCtx, orchestratePlanTimeout())
		defer cancelPlan()
		planningTask := taskWithRepositoryMemory(record.Task, record.MemoryUsed)
		planningTask = taskWithRepositoryGuidelines(planningTask, repositoryGuidelines)
		var err error
		plan, err = orch.Plan(planCtx, planningTask)
		if err != nil {
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				record.Status = "failed"
				record.Error = fmt.Sprintf("governance limit exceeded: maxDuration %s", record.Limits.MaxDuration)
				markGovernanceLimitExceeded(record, record.Error)
				appendOrchestrationEvent(record, "budget.exceeded", "", record.Error)
			} else if errors.Is(planCtx.Err(), context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
				s.stopCanceledOrchestration(record, "Orchestration canceled during planning")
				return
			} else {
				record.Status = "failed"
				record.Error = "plan: " + err.Error()
				appendOrchestrationEvent(record, "planning.failed", "", record.Error)
			}
			record.UpdatedAt = time.Now().UTC()
			updateGovernanceUsage(record)
			if saveErr := saveOrchestrationRecord(record); saveErr != nil {
				slog.Warn("save orchestration failed", "id", record.ID, "error", saveErr)
			}
			s.auditOrchestrationOutcome(record, auditOutcomeFailure, record.Error)
			s.postSourceIssueFinalComment(record)
			s.notifyScheduledRun(record)
			return
		}
	}
	if err := enforceGovernancePlan(record, plan); err != nil {
		record.Plan = plan
		record.Subtasks = makeSubtaskStates(plan)
		record.Status = "failed"
		record.Error = "governance limit exceeded: " + err.Error()
		markGovernanceLimitExceeded(record, record.Error)
		appendOrchestrationEvent(record, "budget.exceeded", "", record.Error)
		record.UpdatedAt = time.Now().UTC()
		updateGovernanceUsage(record)
		if saveErr := saveOrchestrationRecord(record); saveErr != nil {
			slog.Warn("save orchestration failed", "id", record.ID, "error", saveErr)
		}
		s.auditOrchestrationOutcome(record, auditOutcomeFailure, record.Error)
		s.postSourceIssueFinalComment(record)
		s.notifyScheduledRun(record)
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
	updateGovernanceUsage(record)
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
		if err := commitScrumSprintCheckpoint(record, &event); err != nil {
			appendOrchestrationEvent(record, "sprint.commit_failed", event.Subtask.ID, err.Error())
			slog.Warn("commit scrum sprint checkpoint failed", "id", record.ID, "subtask", event.Subtask.ID, "error", err)
		}
		record.UpdatedAt = time.Now().UTC()
		updateGovernanceUsage(record)
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
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			record.Error = fmt.Sprintf("governance limit exceeded: maxDuration %s", record.Limits.MaxDuration)
			markGovernanceLimitExceeded(record, record.Error)
			appendOrchestrationEvent(record, "budget.exceeded", "", record.Error)
		} else {
			updateGovernanceUsage(record)
		}
		if saveErr := saveOrchestrationRecord(record); saveErr != nil {
			slog.Warn("save orchestration failed", "id", record.ID, "error", saveErr)
		}
		s.auditOrchestrationOutcome(record, auditOutcomeFailure, record.Error)
		s.postSourceIssueFinalComment(record)
		s.notifyScheduledRun(record)
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
	updateGovernanceUsage(record)
	if err := saveOrchestrationRecord(record); err != nil {
		slog.Warn("save orchestration failed", "id", record.ID, "error", err)
	}
	s.auditOrchestrationOutcome(record, auditOutcomeSuccess, "")
	s.createPullRequestForOrchestration(record)
	s.postSourceIssueFinalComment(record)
	s.closeSourceIssueIfPolicyAllows(record)
	s.notifyScheduledRun(record)
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
		s.notifyScheduledRun(latest)
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
	s.notifyScheduledRun(record)
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

func usesImplementationHeavyScrumPlan(record *orchestrationRecord) bool {
	return record != nil && record.Scenario != nil && record.Scenario.ID == "implementation-heavy-scrum"
}

func implementationHeavyScrumPlan(record *orchestrationRecord) *orchestrator.TaskPlan {
	task := ""
	if record != nil {
		task = strings.TrimSpace(record.Task)
	}
	if task == "" {
		task = "Build the requested product increment."
	}
	const artifactHygiene = "Artifact hygiene: do not copy the full parent task, prompt text, run workspace contents, compiled binaries, or ARUN-generated archive artifacts into repository documentation or product files. Summarize only the relevant product requirements and keep generated reports concise."
	step := func(id, agentName, description string, deps ...string) orchestrator.Subtask {
		return orchestrator.Subtask{
			ID:          id,
			AgentName:   agentName,
			Description: description + " " + artifactHygiene + "\n\nParent task:\n" + task,
			Deps:        deps,
		}
	}
	return &orchestrator.TaskPlan{
		Description: "Implementation-heavy scrum workflow with three PDCA-style sprint checkpoints. Each sprint runs planning, implementation, QA, adjustment planning, remediation, reporting, and one checkpoint commit in one pull request.",
		Subtasks: []orchestrator.Subtask{
			step("sprint-1-plan", "analyst", "Sprint 1 product planning and design: inspect the repository state and translate the user's request into one concise product concept before implementation. Create or update a single source-of-truth product brief and do not introduce competing concepts, alternate product names, or contradictory mechanics in later docs. Identify the target user, core loop or primary workflow, differentiating mechanic or value proposition, non-goals, and sprint-level acceptance criteria. If the request contains qualitative words such as novel, playful, simple, production-ready, or Japanese equivalents, turn each into observable behavior or review criteria. For games or UX-heavy work, define the unique mechanic, interaction model, visual direction, and how QA can verify that the result is more than a generic scaffold. Define validation commands, the files expected to change, and a simple repository layout that separates backend, frontend, deployment, and docs. For new repositories, prefer cmd/<app> or internal/ for Go code and web/ or static/ for browser assets instead of placing every artifact at the repository root. Do not implement code in this planning stage; leave concrete file changes to the following backend and frontend stages."),
			step("sprint-1-backend", "go-backend", "Sprint 1 coding: create or improve a minimal Go net/http server with a /healthz health endpoint, one product endpoint or static handler, focused tests, clear errors, and simple configuration. Preserve the Sprint 1 product concept and acceptance criteria rather than building a generic scaffold. For an empty repository, initialize the Go module and make `/` serve the primary frontend or product response that matches the selected concept; avoid a root handler that returns unrelated placeholder text when a browser UI exists.", "sprint-1-plan"),
			step("sprint-1-frontend", "frontend", "Sprint 1 coding: create or connect a minimal user-facing frontend/static experience that can be served by the project. Put browser assets under a dedicated web/, static/, or assets/ directory unless existing conventions require otherwise. Make the primary path inspectable in a browser, avoid placeholder-only UI, preserve backend work, and implement the differentiating mechanic, interaction model, or user-facing value defined in Sprint 1 planning. Do not create an unconnected alternate frontend tree; app title, visible labels, README, and implementation behavior must use the same product concept.", "sprint-1-backend"),
			step("sprint-1-qa", "qa", "Sprint 1 QA: run available tests or smoke checks, verify the acceptance criteria, primary user path, differentiating mechanic or value proposition, and repository layout clarity, record gaps in repository artifacts, and explicitly call out backend/frontend follow-up work for this sprint adjustment pass. Compare the product brief against README, app title, visible UI labels, Go server root behavior, frontend code, and docs; treat concept drift, unserved alternate UI files, generic scaffolds, or missing differentiating mechanics as release-blocking gaps.", "sprint-1-frontend"),
			step("sprint-1-adjust-plan", "analyst", "Sprint 1 adjustment planning: read Sprint 1 QA evidence and turn failures or missing baseline behavior into concise backend/frontend remediation notes. Do not implement code in this planning stage.", "sprint-1-qa"),
			step("sprint-1-backend-fix", "go-backend", "Sprint 1 remediation: address QA findings that require backend, API, test, or integration changes. Keep the repository runnable from a fresh checkout.", "sprint-1-adjust-plan"),
			step("sprint-1-frontend-fix", "frontend", "Sprint 1 remediation: address QA findings that require frontend or static asset changes, preserving the backend fixes from this sprint.", "sprint-1-backend-fix"),
			step("sprint-1-report", "release-manager", "Sprint 1 reporting: summarize delivered baseline, QA evidence, remediation performed, and remaining backlog before the Sprint 1 checkpoint commit. Keep the report focused on product outcome and validation evidence; do not paste the full original task prompt.", "sprint-1-frontend-fix"),

			step("sprint-2-plan", "analyst", "Sprint 2 planning: read Sprint 1 report and repository state, then produce concise product-design refinement notes, deployment packaging notes, and unresolved product gaps. Confirm whether the implemented behavior still matches the user's original intent, the single product brief, and the differentiating requirement. Do not rename the product concept or introduce a second mechanic unless the first was explicitly rejected as a QA blocker. Do not implement code in this planning stage.", "sprint-1-report"),
			step("sprint-2-backend", "go-backend", "Sprint 2 coding: harden app startup, configuration, tests, and user-facing behavior before packaging. Address remaining product or design gaps from Sprint 1 if they block the user's requested experience, and avoid broad rewrites that reduce reviewability. Keep `/`, health endpoints, tests, Docker, and Helm aligned with the same primary product path.", "sprint-2-plan"),
			step("sprint-2-docker", "docker", "Sprint 2 coding: add or improve a Dockerfile and container-focused run instructions for the application produced so far. Keep layers deterministic, avoid secret leakage, and make the image runnable with documented ports and health checks.", "sprint-2-backend"),
			step("sprint-2-helm", "helm", "Sprint 2 coding: add or improve a Helm chart suitable for deploying this application into the same Kubernetes environment as ARUN. Include values for image, service, probes, resources, and labels. Ingress is not required.", "sprint-2-docker"),
			step("sprint-2-kubernetes", "kubernetes", "Sprint 2 coding: add Kubernetes Deployment and Service manifests or chart templates for the application. Include selectors, probes where practical, resource defaults, and operational notes. Avoid ingress unless explicitly requested.", "sprint-2-helm"),
			step("sprint-2-qa", "qa", "Sprint 2 QA: validate Docker, Helm, Kubernetes, and app-level smoke paths where tooling is available. Record exact commands, observed results, release blockers, and packaging or deployment gaps for this sprint adjustment pass.", "sprint-2-kubernetes"),
			step("sprint-2-adjust-plan", "analyst", "Sprint 2 adjustment planning: read Sprint 2 QA evidence and turn deployment/package failures into concise remediation notes before the Sprint 2 checkpoint. Do not implement code in this planning stage.", "sprint-2-qa"),
			step("sprint-2-infra-fix", "kubernetes", "Sprint 2 remediation: fix Kubernetes, Helm, or deployment manifest issues found by QA, coordinating with existing Docker and app artifacts.", "sprint-2-adjust-plan"),
			step("sprint-2-report", "release-manager", "Sprint 2 reporting: summarize deployment artifacts, QA evidence, remediation performed, and remaining CI/release backlog before the Sprint 2 checkpoint commit. Keep the report concise and avoid repeating long command blocks already covered in focused docs.", "sprint-2-infra-fix"),

			step("sprint-3-plan", "analyst", "Sprint 3 planning: read Sprint 2 report and repository state, then produce concise product-readiness, CI, documentation, final QA, review, and release-readiness notes. Re-check the user request against the actual product experience and identify any remaining design gap before final polish. Do not implement code in this planning stage.", "sprint-2-report"),
			step("sprint-3-devops", "devops", "Sprint 3 coding: add or improve GitHub Actions CI so future pull requests can continuously run the available backend/frontend/container checks. Keep workflows minimal, reproducible, and aligned with local validation commands.", "sprint-3-plan"),
			step("sprint-3-docs", "docs", "Sprint 3 documentation: update README or docs with a product-centered overview, primary user walkthrough, acceptance criteria status, local run, test, Docker, Helm/Kubernetes deploy, rollback or operations notes, and reviewer guidance. Explain what was built and how it behaves before listing commands. Keep README concise, move detailed procedures into focused docs, remove duplicated or stale instructions, remove copied parent-task prompt text, and remove links to non-existent sprint reports or operations docs. Documentation must describe the implemented product, not an aspirational or alternate concept.", "sprint-3-devops"),
			step("sprint-3-qa", "qa", "Sprint 3 QA: run final smoke and validation checks across app, CI, docs, Docker, Helm, and Kubernetes artifacts. Verify reviewer-facing setup from a fresh checkout perspective, check that frontend/backend/deployment/docs layout is understandable, and record any release-blocking gaps for this sprint adjustment pass. Include a product coherence check: the selected product brief, README, app title, served UI, source files, acceptance criteria status, and validation evidence must agree on the same concept and differentiating mechanic.", "sprint-3-docs"),
			step("sprint-3-adjust-plan", "analyst", "Sprint 3 adjustment planning: read final QA evidence and convert release-blocking issues into concise remediation notes before review. Do not implement code in this planning stage.", "sprint-3-qa"),
			step("sprint-3-backend-fix", "go-backend", "Sprint 3 remediation: fix any final app, test, startup, or integration issues discovered by QA before review.", "sprint-3-adjust-plan"),
			step("sprint-3-review", "reviewer", "Sprint 3 review: inspect the final diff for correctness, maintainability, missing tests, user-facing completeness, repository structure, documentation duplication, accidental binary/workspace artifacts, product concept drift, unserved alternate UI files, broken documentation links, operational safety, and deployment risks. Leave actionable notes in repository artifacts where appropriate.", "sprint-3-backend-fix"),
			step("sprint-3-report", "release-manager", "Sprint 3 reporting: produce a concise final report covering what was built, acceptance criteria status, the three sprint checkpoints, validation results, GitHub artifacts, residual risks, and remaining backlog. Keep PR-ready output short enough for GitHub and link to detailed docs instead of embedding full sprint logs.", "sprint-3-review"),
		},
	}
}

func commitScrumSprintCheckpoint(record *orchestrationRecord, event *orchestrator.SubtaskEvent) error {
	if !usesImplementationHeavyScrumPlan(record) || event == nil || event.Type != orchestrator.SubtaskCompleted {
		return nil
	}
	if event.Result == nil || !event.Result.Success {
		return nil
	}
	sprint, ok := scrumSprintCheckpoint(event.Subtask.ID)
	if !ok {
		return nil
	}
	if strings.TrimSpace(record.RepoPath) == "" {
		return fmt.Errorf("missing repository workspace")
	}
	hygiene, err := scrubRepositoryArtifacts(record.RepoPath)
	if err != nil {
		return fmt.Errorf("repository hygiene: %w", err)
	}
	if len(hygiene.Removed) > 0 || len(hygiene.Updated) > 0 {
		appendOrchestrationEvent(record, "repository.hygiene", event.Subtask.ID, repositoryHygieneMessage(hygiene))
	}
	if err := gitAddAll(record.RepoPath, record.GitHubToken); err != nil {
		return err
	}
	if err := gitConfig(record.RepoPath, record.GitHubToken, "user.email", "arun@example.invalid"); err != nil {
		return err
	}
	if err := gitConfig(record.RepoPath, record.GitHubToken, "user.name", "ARUN"); err != nil {
		return err
	}
	message := fmt.Sprintf("ARUN %s sprint %d checkpoint", record.ID, sprint)
	if gitTreeClean(record.RepoPath, record.GitHubToken) {
		if err := gitCommitAllowEmpty(record.RepoPath, record.GitHubToken, message); err != nil {
			return err
		}
	} else if err := gitCommit(record.RepoPath, record.GitHubToken, message); err != nil {
		return err
	}
	appendOrchestrationEvent(record, "sprint.commit", event.Subtask.ID, fmt.Sprintf("Committed Sprint %d checkpoint", sprint))
	return nil
}

func scrumSprintCheckpoint(subtaskID string) (int, bool) {
	switch subtaskID {
	case "sprint-1-report":
		return 1, true
	case "sprint-2-report":
		return 2, true
	case "sprint-3-report":
		return 3, true
	default:
		return 0, false
	}
}

func orchestrateSubtaskTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("ARUN_ORCHESTRATE_SUBTASK_TIMEOUT"))
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
	raw := strings.TrimSpace(os.Getenv("ARUN_ORCHESTRATE_PLAN_TIMEOUT"))
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
	return resolveOrchestrateRepoWithToken(repo, baseBranch, "")
}

func resolveOrchestrateRepoWithToken(repo, baseBranch, token string) (string, error) {
	if repo == "" {
		repo = "."
	}
	if err := validateGitRef(defaultBaseBranch(baseBranch)); err != nil {
		return "", err
	}

	if cloneURL, ok := normalizeRemoteRepo(repo); ok {
		return cloneRemoteRepoWithToken(cloneURL, defaultBaseBranch(baseBranch), token)
	}
	if repo != "." {
		return "", fmt.Errorf("repo must be a GitHub HTTPS URL, owner/repo, or current directory")
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

func cloneRemoteRepoWithToken(cloneURL, baseBranch, token string) (string, error) {
	root := filepath.Join(apphome.Dir(), "workspaces", "orchestrate")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("create workspace root: %w", err)
	}

	dest := filepath.Join(root, fmt.Sprintf("%s-%s-%s", time.Now().UTC().Format("20060102T150405"), generateID(), safeRepoSlug(cloneURL)))
	args := gitCloneArgs(cloneURL, baseBranch, dest)
	out, err := runGitCloneWithToken(args, token)
	if err != nil && shouldRetryCloneWithoutBranch(string(out)) {
		_ = os.RemoveAll(dest)
		args = gitCloneArgs(cloneURL, "", dest)
		out, err = runGitCloneWithToken(args, token)
	}
	if err != nil {
		return "", fmt.Errorf("clone repo: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return dest, nil
}

func runGitCloneWithToken(args []string, token string) ([]byte, error) {
	// Inputs are constrained to HTTPS github.com owner/repo URLs and validated refs before args are built.
	//
	// codeql[go/command-injection]
	cmd := exec.Command("git", args...)
	cmd.Env = gitCloneEnvWithToken(args, token)
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
	return gitCloneEnvWithToken(args, "")
}

func gitCloneEnvWithToken(args []string, token string) []string {
	env := os.Environ()
	token = strings.TrimSpace(token)
	if token == "" {
		envToken, err := arungh.TokenFromEnv(context.Background())
		if err == nil {
			token = envToken
		}
	}
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
		branch = "arun/" + id
	}
	if err := validateGitRef(branch); err != nil {
		return nil, err
	}

	prBase := defaultBaseBranch(req.GitHub.PRBase)
	if err := validateGitRef(prBase); err != nil {
		return nil, err
	}

	issueTitle := normalizeGitHubArtifactTitle(req.GitHub.IssueTitle, req.Task, "ARUN orchestration "+id)
	prTitle := normalizeGitHubArtifactTitle(req.GitHub.PRTitle, req.Task, "ARUN orchestration "+id)
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

func normalizeGitHubArtifactTitle(title, taskText, fallback string) string {
	for _, candidate := range []string{title, firstNonEmptyLine(taskText), fallback} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		return truncateGitHubTitle(candidate)
	}
	return "ARUN orchestration"
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateGitHubTitle(title string) string {
	const maxGitHubTitleRunes = 256
	runes := []rune(strings.TrimSpace(title))
	if len(runes) <= maxGitHubTitleRunes {
		return string(runes)
	}
	const suffix = "..."
	limit := maxGitHubTitleRunes - len([]rune(suffix))
	if limit < 1 {
		return string(runes[:maxGitHubTitleRunes])
	}
	return string(runes[:limit]) + suffix
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
			BranchName:        fmt.Sprintf("arun/issue-%d", importReq.IssueNumber),
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
		case "arun:create-pr":
			value := true
			opts.CreatePullRequest = &value
		case "arun:report-only":
			value := false
			opts.CreatePullRequest = &value
		case "arun:parallel":
			opts.Strategy = "parallel"
		case "arun:sequential":
			opts.Strategy = "sequential"
		case "arun:close-never":
			opts.ClosePolicy = "never"
		case "arun:close-on-quality-gate-pass":
			opts.ClosePolicy = "on_quality_gate_pass"
		case "arun:close-on-pr-merge":
			opts.ClosePolicy = "on_pr_merge"
		case "arun:approval-required":
			value := true
			opts.RequireApproval = &value
		}
	}
	if command, ok := parseARUNRunCommand(text); ok {
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

func parseARUNRunCommand(text string) (issueTriggerOptions, bool) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "/arun run") {
			continue
		}
		var opts issueTriggerOptions
		for _, field := range strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, "/arun run"))) {
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

func userGitHubToken(user *authUser) string {
	if user == nil {
		return ""
	}
	return strings.TrimSpace(user.AccessToken)
}

func githubClientForRecord(record *orchestrationRecord, owner, name string) *arungh.Client {
	client := arungh.NewClient(owner, name)
	if record != nil && strings.TrimSpace(record.GitHubToken) != "" {
		client.WithToken(record.GitHubToken)
	}
	return client
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
	client := githubClientForRecord(record, owner, name)
	issue, err := client.CreateIssue(arungh.CreateIssueRequest{
		Title: record.GitHub.IssueTitle,
		Body:  orchestrationIssueBody(record),
		Labels: []string{
			"arun",
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
	if err := publishOrchestrationBranch(record); err != nil {
		record.GitHub.Error = "create pull request: " + err.Error()
		record.UpdatedAt = time.Now().UTC()
		_ = saveOrchestrationRecord(record)
		s.auditGitHubArtifact(record, "github.pull_request.create", auditOutcomeFailure, record.GitHub.Error)
		return
	}
	client := githubClientForRecord(record, owner, name)
	pr, err := client.CreatePR(arungh.CreatePRRequest{
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

func publishOrchestrationBranch(record *orchestrationRecord) error {
	if record == nil || record.GitHub == nil {
		return fmt.Errorf("missing GitHub state")
	}
	if strings.TrimSpace(record.RepoPath) == "" {
		return fmt.Errorf("missing repository workspace")
	}
	if err := validateGitRef(record.GitHub.BranchName); err != nil {
		return err
	}
	if err := validateGitRef(record.GitHub.PRBase); err != nil {
		return err
	}
	if _, err := scrubRepositoryArtifacts(record.RepoPath); err != nil {
		return fmt.Errorf("repository hygiene: %w", err)
	}
	if err := gitAddAll(record.RepoPath, record.GitHubToken); err != nil {
		return err
	}
	message := fmt.Sprintf("ARUN orchestration %s", record.ID)
	if !gitTreeClean(record.RepoPath, record.GitHubToken) {
		if err := gitConfig(record.RepoPath, record.GitHubToken, "user.email", "arun@example.invalid"); err != nil {
			return err
		}
		if err := gitConfig(record.RepoPath, record.GitHubToken, "user.name", "ARUN"); err != nil {
			return err
		}
		if err := gitCommit(record.RepoPath, record.GitHubToken, message); err != nil {
			return err
		}
	}
	if err := ensurePullRequestBaseBranch(record.RepoPath, record.GitHubToken, record.GitHub.PRBase, message); err != nil {
		return err
	}
	if err := gitPushHead(record.RepoPath, record.GitHubToken, record.GitHub.BranchName); err != nil {
		return err
	}
	return nil
}

func repositoryHygieneMessage(result repositoryHygieneResult) string {
	var parts []string
	if len(result.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("removed %d compiled artifact(s)", len(result.Removed)))
	}
	if len(result.Updated) > 0 {
		parts = append(parts, fmt.Sprintf("cleaned %d prompt-contaminated Markdown file(s)", len(result.Updated)))
	}
	if len(parts) == 0 {
		return "Repository hygiene check passed"
	}
	return "Repository hygiene: " + strings.Join(parts, ", ")
}

func ensurePullRequestBaseBranch(dir, token, baseBranch, message string) error {
	baseBranch = strings.TrimSpace(baseBranch)
	if baseBranch == "" {
		return nil
	}
	exists, err := gitRemoteBranchExists(dir, token, baseBranch)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	baseCommit, err := gitCreateEmptyCommit(dir, token, "Initialize "+baseBranch+" for ARUN orchestration PRs")
	if err != nil {
		return err
	}
	if err := gitPushCommitToBranch(dir, token, baseCommit, baseBranch); err != nil {
		return err
	}
	headCommit, err := gitReparentHeadToCommit(dir, token, baseCommit, message)
	if err != nil {
		return err
	}
	return gitResetHard(dir, token, headCommit)
}

func gitTreeClean(dir, token string) bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	cmd.Dir = dir
	cmd.Env = gitCloneEnvWithToken([]string{"https://github.com/"}, token)
	return cmd.Run() == nil
}

func gitRemoteBranchExists(dir, token, branch string) (bool, error) {
	ref := "refs/heads/" + branch
	// codeql[go/command-injection]
	cmd := exec.Command("git", "ls-remote", "--heads", "origin")
	cmd.Dir = dir
	cmd.Env = gitCloneEnvWithToken([]string{"https://github.com/"}, token)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("%s: %w: %s", strings.Join(cmd.Args, " "), err, strings.TrimSpace(string(out)))
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref {
			return true, nil
		}
	}
	return false, nil
}

func gitAddAll(dir, token string) error {
	// codeql[go/command-injection]
	cmd := exec.Command("git", "add", ".")
	return runPreparedGitCommand(cmd, dir, token)
}

func gitConfig(dir, token, key, value string) error {
	// codeql[go/command-injection]
	cmd := exec.Command("git", "config", key, value)
	return runPreparedGitCommand(cmd, dir, token)
}

func gitCommit(dir, token, message string) error {
	// codeql[go/command-injection]
	cmd := exec.Command("git", "commit", "-m", message)
	return runPreparedGitCommand(cmd, dir, token)
}

func gitCommitAllowEmpty(dir, token, message string) error {
	// codeql[go/command-injection]
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", message)
	return runPreparedGitCommand(cmd, dir, token)
}

func gitCreateEmptyCommit(dir, token, message string) (string, error) {
	// codeql[go/command-injection]
	treeCmd := exec.Command("git", "mktree")
	treeCmd.Stdin = strings.NewReader("")
	tree, err := runPreparedGitCommandOutput(treeCmd, dir, token)
	if err != nil {
		return "", err
	}
	tree = strings.TrimSpace(tree)
	if !gitObjectPattern.MatchString(tree) {
		return "", fmt.Errorf("invalid git tree object: %s", tree)
	}
	// codeql[go/command-injection]
	// #nosec G204 -- tree is validated as a git object id from git mktree.
	commitCmd := exec.Command("git", "commit-tree", strings.TrimSpace(tree), "-m", message)
	return runPreparedGitCommandOutput(commitCmd, dir, token)
}

func gitReparentHeadToCommit(dir, token, parentCommit, message string) (string, error) {
	// codeql[go/command-injection]
	treeCmd := exec.Command("git", "rev-parse", "HEAD^{tree}")
	tree, err := runPreparedGitCommandOutput(treeCmd, dir, token)
	if err != nil {
		return "", err
	}
	tree = strings.TrimSpace(tree)
	parentCommit = strings.TrimSpace(parentCommit)
	if !gitObjectPattern.MatchString(tree) {
		return "", fmt.Errorf("invalid git tree object: %s", tree)
	}
	if !gitObjectPattern.MatchString(parentCommit) {
		return "", fmt.Errorf("invalid git parent object: %s", parentCommit)
	}
	// codeql[go/command-injection]
	// #nosec G204 -- tree and parent commit are validated git object ids.
	commitCmd := exec.Command("git", "commit-tree", tree, "-p", parentCommit, "-m", message)
	return runPreparedGitCommandOutput(commitCmd, dir, token)
}

func gitResetHard(dir, token, commit string) error {
	commit = strings.TrimSpace(commit)
	if !gitObjectPattern.MatchString(commit) {
		return fmt.Errorf("invalid git commit object: %s", commit)
	}
	// codeql[go/command-injection]
	// #nosec G204 -- commit is validated as a git object id.
	cmd := exec.Command("git", "reset", "--hard", commit)
	return runPreparedGitCommand(cmd, dir, token)
}

func gitPushHead(dir, token, branch string) error {
	refspec := "HEAD:refs/heads/" + branch
	// codeql[go/command-injection]
	cmd := exec.Command("git", "push", "--set-upstream", "origin", refspec, "--force-with-lease")
	return runPreparedGitCommand(cmd, dir, token)
}

func gitPushCommitToBranch(dir, token, commit, branch string) error {
	refspec := strings.TrimSpace(commit) + ":refs/heads/" + branch
	// codeql[go/command-injection]
	cmd := exec.Command("git", "push", "origin", refspec)
	return runPreparedGitCommand(cmd, dir, token)
}

func runPreparedGitCommand(cmd *exec.Cmd, dir, token string) error {
	_, err := runPreparedGitCommandCombinedOutput(cmd, dir, token)
	return err
}

func runPreparedGitCommandOutput(cmd *exec.Cmd, dir, token string) (string, error) {
	out, err := runPreparedGitCommandCombinedOutput(cmd, dir, token)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runPreparedGitCommandCombinedOutput(cmd *exec.Cmd, dir, token string) ([]byte, error) {
	cmd.Dir = dir
	cmd.Env = gitCloneEnvWithToken([]string{"https://github.com/"}, token)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", strings.Join(cmd.Args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
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

func (s *Server) createSourceIssueComment(record *orchestrationRecord, body string) (*arungh.IssueComment, error) {
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
	client := githubClientForRecord(record, owner, name)
	return client.CreateIssueComment(record.GitHub.SourceIssueNumber, arungh.CreateIssueCommentRequest{Body: body})
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
	client := githubClientForRecord(record, owner, name)
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
	return strings.TrimSpace(fmt.Sprintf(`ARUN orchestration started.

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
	fmt.Fprintf(&b, "ARUN orchestration finished.\n\n")
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
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("ARUN_PUBLIC_URL")), "/")
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

const githubPullRequestBodyMaxBytes = 60000

func loadArtifactConfig(repoPath string) artifactConfig {
	var cfg artifactConfig
	if repoPath == "" {
		return cfg
	}
	raw, err := os.ReadFile(filepath.Join(repoPath, ".arun", "config.yaml"))
	if err != nil {
		return cfg
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		slog.Warn("parse .arun/config.yaml failed", "repoPath", repoPath, "error", err)
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
		{
			ID:                "three-sprint-agile-scrum",
			Name:              "Three-Sprint Agile Scrum",
			Description:       "Run a guided three-sprint scrum workflow with planning, implementation, review, smoke, and reporting stages.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "analyst", "release-manager", "reviewer", "qa", "reporter"),
			Strategy:          "sequential",
			CreatePullRequest: false,
			Limits: governanceLimits{
				MaxDuration:          "45m",
				MaxSubtasks:          18,
				MaxConcurrentRepoRun: 1,
			},
			TaskTemplate: `Run a three-sprint agile scrum simulation for {{repo}} on {{baseBranch}}.

Operating mode: report-first. Do not make destructive changes. Prefer Markdown reports, sprint plans, review notes, and smoke-test notes over code changes unless the repository state explicitly requires a small safe change.

Sprint 1:
- Plan a narrow product increment from repository context.
- Identify acceptance criteria, likely files, risks, and validation.
- Implement only if the work is low risk and clearly scoped.
- Review the result and run a lightweight smoke check.

Sprint 2:
- Continue from Sprint 1 findings.
- Refine scope based on review and smoke-test evidence.
- Implement only safe incremental changes.
- Review and smoke test again.

Sprint 3:
- Stabilize the outcome.
- Summarize what was built, what was not built, validation results, risks, and recommended backlog.
- Produce a final stakeholder report in the selected output language or the repository's usual language.

Expected output:
- Sprint 1, Sprint 2, and Sprint 3 sections.
- Planning, coding, review, smoke, and reporting notes for each sprint.
- Clear list of repository changes, if any.
- Final recommendation for the next human review step.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
			},
		},
		{
			ID:                "implementation-heavy-scrum",
			Name:              "Implementation-Heavy Scrum",
			Description:       "Run a build-oriented scrum workflow for new or sandbox repositories with app, CI/CD, Kubernetes, review, smoke, and reporting artifacts.",
			Source:            "built-in",
			Agents:            availableAgentNames(registry, "analyst", "go-backend", "frontend", "docs", "qa", "reviewer", "release-manager", "docker", "helm", "kubernetes", "devops"),
			Strategy:          "sequential",
			CreateIssue:       true,
			CreatePullRequest: true,
			Limits: governanceLimits{
				MaxDuration:          "180m",
				MaxSubtasks:          30,
				MaxConcurrentRepoRun: 1,
			},
			TaskTemplate: `Run an implementation-heavy agile scrum workflow for {{repo}} on {{baseBranch}}.

Operating mode: build-first for a new or sandbox repository. Create concrete, reviewable repository artifacts where safe. Prefer a small vertical slice over a broad unfinished scaffold. Do not rewrite unrelated existing work. If the repository is completely empty or has no commits yet, create an initial minimal product scaffold before extending it.

Target baseline for a new repository:
- A minimal Go HTTP server with a health endpoint, clear startup/configuration behavior, focused tests, and a small product API or static asset handler.
- A minimal Web UI or static frontend slice when it fits the repository goal. It must expose a reviewable primary user path rather than placeholder-only screens.
- Dockerfile and local validation commands.
- Helm chart and Kubernetes manifests suitable for deploying into the same Kubernetes environment as ARUN. Include Service, Deployment, selectors, labels, probes where practical, and resource defaults. Ingress is not required.
- GitHub Actions CI that runs tests, lint or smoke checks, and build validation so future PRs can improve the product continuously.
- README documentation with setup, local run, product walkthrough, Kubernetes deploy notes, validation commands, known limitations, and operational follow-up backlog. Emphasize what the product does, the primary user experience, acceptance criteria, and review points before listing commands. Keep README concise and move deep operational details into focused docs.

Quality bar:
- Start with product planning, not scaffolding. Translate the user's requested value into a concrete concept, user, core loop or workflow, differentiating behavior, non-goals, and acceptance criteria before coding.
- Keep one product concept as the source of truth. Do not create multiple competing product briefs, alternate product names, or contradictory differentiating mechanics across README, docs, UI labels, and code.
- When the request includes qualitative intent such as novelty, fun, polish, simplicity, production readiness, or a language-specific equivalent, define observable review criteria for that intent and verify them in QA.
- For games or UX-heavy apps, implement at least one concrete mechanic, interaction, or content choice that makes the result specific to the request. A generic shell with renamed labels is not enough.
- Define sprint-level acceptance criteria before implementing and verify them in QA.
- Prefer cohesive, reviewer-friendly changes over broad generated scaffolding.
- Keep generated code simple, idiomatic, and runnable from a fresh checkout.
- Keep frontend, backend, deployment, and documentation concerns separated in the repository layout unless the existing project convention clearly differs. Avoid mixing browser assets, Go server code, Helm charts, and narrative docs in one flat directory when a clearer structure is possible.
- Serve the primary user-facing experience from the main application path. Avoid creating unconnected alternate frontend trees or a Go root handler that returns placeholder text while the real UI lives elsewhere.
- Avoid duplicated documentation. Use README as the short entry point and link to focused docs for testing, deployment, operations, and sprint reports.
- Make outcome documentation product-centered: explain the behavior delivered, the user journey, important implementation decisions, validation evidence, and remaining product gaps. Avoid filling docs with generic process narration, repeated command lists, aspirational alternate concepts, or links to files that do not exist.
- Do not copy the full parent task, prompt text, ARUN run workspace, generated archive, or compiled binaries into the target repository. Summarize requirements and commit only source, configuration, tests, docs, and intentional assets.
- Record exact validation commands and outcomes, including skipped checks and why they were skipped.
- Treat broken tests, missing startup instructions, non-rendering UI, invalid Helm/Kubernetes output, concept drift between planning/docs/code, unserved alternate UI files, broken docs links, and unclear next steps as release-blocking gaps.

Sprint 1:
- Inspect repository state and choose the smallest coherent product increment.
- Produce a concise product/design brief before implementation. Include the intended user, core loop or workflow, differentiating behavior, acceptance criteria, non-goals, and how QA will judge the requested value. This brief is the single source of truth for later implementation and documentation.
- For an empty repository, start with a minimal Go server plus a lightweight frontend or static response that can be opened, reviewed, validated without external services, and served by the main app route.
- Decide the primary implementation path from repository evidence and define the acceptance criteria for the first user-visible slice. Use backend, frontend, documentation, or a combination only when it fits the repository.
- Implement a minimal working slice with setup or usage documentation and a repository layout that keeps backend/server code, frontend/static assets, charts/manifests, and docs easy to distinguish.
- Review and smoke test the slice, including whether visible product title, README, source behavior, and acceptance criteria all match the same concept.

Sprint 2:
- Extend the slice with one meaningful capability, test, CI check, containerization, or Kubernetes integration.
- Keep changes cohesive and easy to review.
- Update documentation and run validation.
- Review risks, gaps, and follow-up work. Preserve the Sprint 1 product concept unless QA explicitly found it invalid.

Sprint 3:
- Stabilize the result, improve developer ergonomics, add or refine Helm/Kubernetes deploy artifacts, and remove obvious rough edges.
- Ensure README or docs explain how to run and verify the work without repeating the same long instructions in multiple files.
- Verify the final result from a fresh-checkout reviewer perspective, including product coherence across README, docs, served UI, source files, tests, and deployment artifacts.
- Produce final review, smoke-test notes, and a stakeholder report in the selected output language or the repository's usual language.

Expected output:
- Concrete file changes or a clear explanation of why no safe implementation was possible.
- Sprint 1, Sprint 2, and Sprint 3 sections.
- Product/design brief and acceptance criteria status, including how qualitative requirements were made observable.
- Implementation, documentation, review, smoke, CI/CD, Kubernetes, and release-readiness notes.
- Commands run and validation results.
- Acceptance criteria status, residual risks, and known limitations.
- Repository layout summary, including where backend, frontend, deployment, and docs live.
- Product coherence status: confirm the single concept, app title, primary served path, differentiating mechanic, and docs all match.
- Final backlog for the next human-led sprint.`,
			Variables: []scenarioTemplateVariable{
				{Name: "repo", Label: "Repository", Placeholder: "owner/repo", Required: true},
				{Name: "baseBranch", Label: "Base branch", Default: "main", Required: true},
			},
		},
	}
	return templates
}

type localizedScenarioTemplate struct {
	Name        string
	Description string
	Task        string
}

func localizeScenarioTemplates(templates []scenarioTemplate, language string) {
	if strings.TrimSpace(language) != "ja" {
		return
	}
	names := map[string]localizedScenarioTemplate{
		"go-http-service-bootstrap": {
			Name:        "Go HTTP サービス作成",
			Description: "既存構成を保ちながら Go HTTP サービスを作成または拡張します。",
			Task: `{{repo}} の {{baseBranch}} 上で Go HTTP サービスを作成または拡張してください。

対象 package または module: {{packageName}}
Endpoint または handler: {{endpoints}}

既存の repository layout と conventions を維持してください。テストを追加または更新し、サービスの実行方法を文書化し、検証結果を要約してください。`,
		},
		"bug-fix-with-tests": {
			Name:        "テスト付きバグ修正",
			Description: "不具合を修正し、回帰テストを追加してレビューします。",
			Task: `{{repo}} の {{baseBranch}} 上で bug を修正してください。

Bug または issue: {{targetIssue}}
期待される挙動: {{expectedBehavior}}
関連 file または component: {{scope}}

焦点を絞った regression test を追加し、変更を最小限に保ち、検証結果を含めてください。`,
		},
		"documentation-only-update": {
			Name:        "ドキュメント更新のみ",
			Description: "コード変更を最小限に抑え、README または docs を更新します。",
			Task: `{{repo}} の {{baseBranch}} 上で documentation を更新してください。

Documentation target: {{docTarget}}
Audience または use case: {{audience}}
必要な詳細: {{details}}

既存 documentation style に合わせてください。コマンドは copy-paste 可能にし、無関係な code change は避けてください。`,
		},
		"ci-failure-fixer": {
			Name:        "CI 失敗修正",
			Description: "CI 失敗を診断し、ローカル検証付きで修正します。",
			Task: `{{repo}} の {{baseBranch}} 上で CI failure を修正してください。

Workflow または check: {{workflow}}
Failure URL または log excerpt: {{failure}}

既存 workflow の意図を維持し、可能な範囲で CI validation をローカルでも再現し、root cause を要約してください。`,
		},
		"security-remediation": {
			Name:        "セキュリティ修正",
			Description: "セキュリティまたはコードスキャンの指摘を検証付きで修正します。",
			Task: `{{repo}} の {{baseBranch}} 上で security finding を修正してください。

Finding: {{finding}}
Affected area: {{scope}}
必要な制約: {{constraints}}

狭く防御的な修正を優先し、テストまたは manual verification notes を追加し、残存 risk を文書化してください。`,
		},
		"release-preparation": {
			Name:        "リリース準備",
			Description: "CHANGELOG、チェックリスト、リリース準備資料を更新します。",
			Task: `{{repo}} の {{baseBranch}} 上で release material を準備してください。

Release version: {{version}}
Scope since: {{since}}
必要な artifact: {{artifacts}}

Repository conventions に従って changelog または release docs を更新してください。validation、known gaps、rollback considerations を含めてください。`,
		},
		"frontend-ui-change": {
			Name:        "フロントエンド UI 変更",
			Description: "レスポンシブとアクセシビリティを確認しながら UI 変更を実装します。",
			Task: `{{repo}} の {{baseBranch}} 上で frontend UI change を実装してください。

Screen または flow: {{screen}}
依頼された変更: {{change}}
Validation target: {{validation}}

既存 frontend conventions に従い、text と controls が responsive に収まるようにし、browser または build verification notes を含めてください。`,
		},
		"three-sprint-agile-scrum": {
			Name:        "3 スプリント Agile Scrum",
			Description: "計画、実装、レビュー、スモーク、報告を含む 3 スプリントの scrum ワークフローを実行します。",
			Task: `{{repo}} の {{baseBranch}} 上で 3 スプリントの agile scrum simulation を実行してください。

Operating mode: report-first。破壊的な変更は行わないでください。Repository state が小さく安全な変更を明確に必要としていない限り、code change よりも Markdown report、sprint plan、review notes、smoke-test notes を優先してください。

Sprint 1:
- Repository context から狭い product increment を計画してください。
- Acceptance criteria、likely files、risks、validation を特定してください。
- 作業が low risk かつ明確に scoped されている場合のみ実装してください。
- 結果を review し、軽量な smoke check を実行してください。

Sprint 2:
- Sprint 1 findings から継続してください。
- Review と smoke-test evidence に基づいて scope を調整してください。
- 安全な incremental changes のみ実装してください。
- 再度 review と smoke test を行ってください。

Sprint 3:
- Outcome を安定化してください。
- 何を作ったか、何を作らなかったか、validation results、risks、recommended backlog を要約してください。
- 選択された output language または repository の通常言語で final stakeholder report を作成してください。

Expected output:
- Sprint 1、Sprint 2、Sprint 3 sections。
- 各 sprint の planning、coding、review、smoke、reporting notes。
- Repository changes がある場合は明確な list。
- 次の human review step に向けた final recommendation。`,
		},
		"implementation-heavy-scrum": {
			Name:        "実装重視 Scrum",
			Description: "新規または sandbox リポジトリ向けに、アプリ、CI/CD、Kubernetes、レビュー、スモーク、報告成果物を作成します。",
			Task: `{{repo}} の {{baseBranch}} 上で implementation-heavy agile scrum workflow を実行してください。

Operating mode: 新規または sandbox repository 向けの build-first。安全な範囲で、review 可能な具体的 repository artifacts を作成してください。広く未完成な scaffold より、小さな vertical slice を優先してください。無関係な既存 work は書き換えないでください。Repository が完全に空、または commit がまだ無い場合は、拡張前に初期の minimal product scaffold を作成してください。

新規 repository の target baseline:
- Health endpoint、明確な startup/configuration、focused tests、小さな product API または static asset handler を持つ minimal Go HTTP server。
- Repository goal に合う場合の minimal Web UI または static frontend slice。placeholder だけの画面ではなく、review 可能な primary user path を用意してください。
- Dockerfile と local validation commands。
- ARUN と同じ Kubernetes environment に deploy できる Helm chart と Kubernetes manifests。Service、Deployment、selectors、labels、可能な範囲の probes、resource defaults を含めてください。Ingress は不要です。
- Tests、lint または smoke checks、build validation を実行する GitHub Actions CI。
- Setup、local run、product walkthrough、Kubernetes deploy notes、validation commands、known limitations、operational follow-up backlog を含む README documentation。Commands の列挙より先に、何を作ったか、主要な user experience、acceptance criteria、review points を説明してください。README は短い入口に保ち、詳細な運用手順は focused docs に分離してください。

Quality bar:
- Scaffold から始めず、product planning から始めてください。ユーザーが求める価値を、具体的な concept、対象 user、core loop または workflow、差別化される behavior、non-goals、acceptance criteria に分解してから実装してください。
- Product concept は 1 つだけを source of truth にしてください。README、docs、UI labels、code の間で複数の product brief、別名の product name、矛盾した差別化 mechanic を作らないでください。
- 「新規性」「楽しい」「ポップ」「シンプル」「production-ready」のような定性的意図が含まれる場合は、それぞれを review 可能な observable criteria に変換し、QA で確認してください。
- Game または UX-heavy app では、要求に固有の mechanic、interaction、content choice を少なくとも 1 つ実装してください。label を変えただけの generic shell は不十分です。
- 実装前に sprint-level acceptance criteria を定義し、QA で確認してください。
- 広い generated scaffold より、cohesive で reviewer が追いやすい変更を優先してください。
- 生成 code は単純、idiomatic、fresh checkout から runnable にしてください。
- 既存 convention が明確に違わない限り、frontend、backend、deployment、documentation の関心を repository layout 上で分離してください。Browser assets、Go server code、Helm charts、説明 docs を 1 つの flat directory に混在させることは避けてください。
- Main application path から primary user-facing experience を提供してください。実際の UI が別場所にあるのに Go root handler が placeholder text を返す構成や、接続されていない別 frontend tree は避けてください。
- 重複 documentation を避けてください。README は短い入口として使い、testing、deployment、operations、sprint reports は focused docs へ link してください。
- 成果物側の documentation は product-centered にしてください。実装された behavior、user journey、重要な implementation decisions、validation evidence、残っている product gaps を説明し、generic process narration、command list の繰り返し、実装されていない別 concept、存在しない file への link で埋めないでください。
- 親タスク全文、prompt text、ARUN run workspace、generated archive、compiled binary を target repository にコピーしないでください。Requirements は要約し、source、configuration、tests、docs、意図した assets のみ commit してください。
- 実行した validation commands と outcomes を正確に記録し、skip した check は理由を書いてください。
- 壊れた tests、startup 手順不足、render できない UI、無効な Helm/Kubernetes output、planning/docs/code 間の concept drift、接続されていない別 UI、壊れた docs link、不明瞭な next steps は release-blocking gaps として扱ってください。

Sprint 1:
- Repository state を調査し、最小で一貫した product increment を選んでください。
- 実装前に concise な product/design brief を作成してください。対象 user、core loop または workflow、差別化 behavior、acceptance criteria、non-goals、要求された価値を QA がどう判定するかを含めてください。この brief を後続 implementation/documentation の唯一の source of truth にしてください。
- 空 repository の場合は、外部 service なしで開いて review/validation でき、main app route から提供される minimal Go server と lightweight frontend または static response から始めてください。
- Repository evidence から primary implementation path を決め、最初の user-visible slice の acceptance criteria を定義してください。Backend、frontend、documentation、またはその組み合わせは repository に合う場合のみ使ってください。
- Setup または usage documentation を含む minimal working slice を実装し、backend/server code、frontend/static assets、charts/manifests、docs が区別しやすい repository layout にしてください。
- Slice を review し、smoke test してください。visible product title、README、source behavior、acceptance criteria が同じ concept を指しているかも確認してください。

Sprint 2:
- Slice に意味のある capability、test、CI check、containerization、または Kubernetes integration を 1 つ追加してください。
- 変更を cohesive で review しやすく保ってください。
- Documentation を更新し、validation を実行してください。
- Risks、gaps、follow-up work を review してください。QA が明示的に無効と判定していない限り Sprint 1 product concept を維持してください。

Sprint 3:
- Result を安定化し、developer ergonomics を改善し、Helm/Kubernetes deploy artifacts を追加または調整し、明らかな rough edges を取り除いてください。
- README または docs で実行方法と検証方法を説明し、同じ長い手順を複数 file に繰り返さないでください。
- Fresh checkout の reviewer perspective で final result を確認してください。README、docs、served UI、source files、tests、deployment artifacts が product として一貫しているかも確認してください。
- 選択された output language または repository の通常言語で final review、smoke-test notes、stakeholder report を作成してください。

Expected output:
- Concrete file changes、または安全な implementation が不可能だった理由の明確な explanation。
- Sprint 1、Sprint 2、Sprint 3 sections。
- Product/design brief と acceptance criteria status。定性的要求をどう observable にしたかを含めてください。
- Implementation、documentation、review、smoke、CI/CD、Kubernetes、release-readiness notes。
- 実行した commands と validation results。
- Acceptance criteria status、residual risks、known limitations。
- Backend、frontend、deployment、docs がどこにあるかを示す repository layout summary。
- Product coherence status: single concept、app title、primary served path、differentiating mechanic、docs が一致していることを確認してください。
- 次の human-led sprint の final backlog。`,
		},
	}
	const japaneseOutputInstruction = "\n\n出力言語: 日本語。Repository conventions が別言語を明確に要求しない限り、生成する reports、issue/PR bodies、user-facing summaries、stakeholder notes は日本語で書いてください。"
	for i := range templates {
		if localized, ok := names[templates[i].ID]; ok && templates[i].Source == "built-in" {
			templates[i].Name = localized.Name
			templates[i].Description = localized.Description
			if localized.Task != "" {
				templates[i].TaskTemplate = localized.Task
			}
			if !strings.Contains(templates[i].TaskTemplate, "出力言語: 日本語。") {
				templates[i].TaskTemplate += japaneseOutputInstruction
			}
		}
		if strings.TrimSpace(templates[i].OutputLanguage) == "" {
			templates[i].OutputLanguage = "ja"
		}
	}
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
	dir := filepath.Join(repoPath, ".arun", "scenarios")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []scenarioTemplate{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read .arun/scenarios: %w", err)
	}

	var templates []scenarioTemplate
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: read: %w", filepath.ToSlash(filepath.Join(".arun", "scenarios", entry.Name())), err)
		}
		var tmpl scenarioTemplate
		if err := yaml.Unmarshal(raw, &tmpl); err != nil {
			return nil, fmt.Errorf("%s: parse: %w", filepath.ToSlash(filepath.Join(".arun", "scenarios", entry.Name())), err)
		}
		tmpl.Source = "repository"
		if err := validateScenarioTemplate(&tmpl, registry); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.ToSlash(filepath.Join(".arun", "scenarios", entry.Name())), err)
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
	dir := filepath.Join(repoPath, ".arun", "agents")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []agent.Definition{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read .arun/agents: %w", err)
	}

	var defs []agent.Definition
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		def, err := agent.LoadDefinition(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.ToSlash(filepath.Join(".arun", "agents", entry.Name())), err)
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
	taskText := strings.ToLower(task)
	preset, confidence, rationale := classifyOrchestrationTask(taskText)
	if preset == "general" {
		text := strings.ToLower(task + " " + strings.Join(repoSignals, " "))
		preset, confidence, rationale = classifyOrchestrationTask(text)
	}
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
	return truncateMarkdownBytes(renderArtifactBody(record, "pull_request"), githubPullRequestBodyMaxBytes, prBodyTruncationNotice(artifactLanguage(record)))
}

func prBodyTruncationNotice(language string) string {
	if language == "ja" {
		return "\n\n---\n\nこの PR 本文は GitHub の本文サイズ上限に収めるため ARUN により短縮されました。詳細は run summary、生成された repository docs、各 sprint checkpoint を確認してください。\n"
	}
	return "\n\n---\n\nThis PR body was shortened by ARUN to stay within GitHub's body size limit. See the run summary, generated repository docs, and sprint checkpoint commits for full details.\n"
}

func truncateMarkdownBytes(body string, maxBytes int, notice string) string {
	if maxBytes <= 0 || len(body) <= maxBytes {
		return body
	}
	notice = strings.TrimSpace(notice)
	if notice != "" {
		notice = "\n\n" + notice + "\n"
	}
	limit := maxBytes - len([]byte(notice))
	if limit < 0 {
		limit = maxBytes
		notice = ""
	}
	var trimmed strings.Builder
	trimmed.Grow(limit)
	used := 0
	for _, r := range body {
		runeLen := len(string(r))
		if used+runeLen > limit {
			break
		}
		trimmed.WriteRune(r)
		used += runeLen
	}
	return strings.TrimSpace(trimmed.String()) + notice
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
			return "ARUN Orchestrate により作成されました。\n\n" +
				"- Run: `{{.RunID}}`\n" +
				"- Repository: `{{.Repository}}`\n" +
				"- Base branch: `{{.BaseBranch}}`\n" +
				"- Target branch: `{{.TargetBranch}}`\n" +
				"- Strategy: `{{.Strategy}}`\n" +
				"- Agents: `{{.Agents}}`\n\n" +
				"## タスク\n\n{{.Task}}\n"
		}
		return "Created by ARUN Orchestrate.\n\n" +
			"- Run: `{{.RunID}}`\n" +
			"- Repository: `{{.Repository}}`\n" +
			"- Base branch: `{{.BaseBranch}}`\n" +
			"- Target branch: `{{.TargetBranch}}`\n" +
			"- Strategy: `{{.Strategy}}`\n" +
			"- Agents: `{{.Agents}}`\n\n" +
			"## Task\n\n{{.Task}}\n"
	}
	if language == "ja" {
		return "ARUN Orchestrate により作成されました。\n\n" +
			"{{if .IssueURL}}Tracking issue: {{.IssueURL}}\n\n{{end}}" +
			"- Run: `{{.RunID}}`\n" +
			"- Base branch: `{{.PRBase}}`\n" +
			"- Agents: `{{.Agents}}`\n\n" +
			"{{if .Summary}}## 概要\n\n{{.Summary}}\n{{end}}"
	}
	return "Created by ARUN Orchestrate.\n\n" +
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
