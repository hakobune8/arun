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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/embedding"
	agentosgh "github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/orchestrator"
	"github.com/kazyamaz200/agentos/internal/runtime"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/search"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/kazyamaz200/agentos/internal/vector"
)

//go:embed static
var staticFS embed.FS

// Server serves the AgentOS web UI and API endpoints.
type Server struct {
	port      int
	server    *http.Server
	search    *search.Service
	agentReg  *agent.Registry
	llmClient llm.LLMClient
	sandbox   sandbox.Sandbox
	runtimeCfg *runtime.Config
}

// NewServer creates a new Server listening on the given port.
func NewServer(port int) *Server {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	svc := search.NewService(vs, emb)

	llmCfg := llm.DefaultConfig()
	llmClient := llm.NewLiteLLMClient(llmCfg)

	mux := http.NewServeMux()
	s := &Server{
		port:      port,
		search:    svc,
		agentReg:  agent.DefaultRegistry(),
		llmClient: llmClient,
		sandbox:   sandbox.NewLocalSandbox("."),
		runtimeCfg: &runtime.Config{Verbose: false},
	}

	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunDetail)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/github/", s.handleGitHub)
	mux.HandleFunc("/api/orchestrate", s.handleOrchestrate)

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

// --- Runs ---

type createRunRequest struct {
	Agent       string `json:"agent"`
	Task        string `json:"task"`
	Description string `json:"description"`
	Repo        string `json:"repo"`
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
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

	homeDir, _ := os.UserHomeDir()
	runsDir := filepath.Join(homeDir, ".agentos", "runs")

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

	agt, err := s.agentReg.Create(req.Agent, s.llmClient)
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
		Description: req.Description,
	}

	sb := sandbox.NewLocalSandbox(repo)
	cfg := &runtime.Config{Verbose: false}

	rt := runtime.NewRuntime(s.llmClient, nil, sb, cfg, agt)

	go func() {
		if err := rt.Run(context.Background(), tk); err != nil {
			slog.Warn("async run failed", "id", id, "error", err)
		}
	}()

	_ = json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck // best-effort
		"id":     id,
		"status": "started",
	})
}

// --- Run Detail ---

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	id := filepath.Base(r.URL.Path)

	homeDir, _ := os.UserHomeDir()
	runDir := filepath.Join(homeDir, ".agentos", "runs", id)

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
		_ = json.NewEncoder(w).Encode(issues) //nolint:errcheck // best-effort response

	case "pulls":
		state := r.URL.Query().Get("state")
		prs, err := client.ListPRs(state)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(prs) //nolint:errcheck // best-effort response

	case "checks":
		ref := r.URL.Query().Get("ref")
		if ref == "" {
			ref = "main"
		}
		suites, err := client.GetCheckSuites(ref)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(suites) //nolint:errcheck // best-effort response

	default:
		http.Error(w, "unknown github resource: "+path, http.StatusNotFound)
	}
}

// --- Orchestrate ---

type orchestrateRequest struct {
	Agents   []string `json:"agents"`
	Task     string   `json:"task"`
	Strategy string   `json:"strategy"`
}

func (s *Server) handleOrchestrate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

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

	if len(req.Agents) == 0 || req.Task == "" {
		http.Error(w, "agents and task are required", http.StatusBadRequest)
		return
	}

	agents := make(map[string]runtime.Agent)
	for _, name := range req.Agents {
		a, err := s.agentReg.Create(name, s.llmClient)
		if err != nil {
			http.Error(w, "lookup agent "+name+": "+err.Error(), http.StatusBadRequest)
			return
		}
		agents[name] = a
	}

	cfg := &runtime.Config{Verbose: false}
	orch := orchestrator.NewOrchestrator(s.llmClient, s.sandbox, agents, cfg)

	if req.Strategy == "parallel" {
		orch.SetStrategy(orchestrator.StrategyParallel)
	}

	plan, err := orch.Plan(r.Context(), req.Task)
	if err != nil {
		http.Error(w, "plan: "+err.Error(), http.StatusInternalServerError)
		return
	}

	results, err := orch.Execute(r.Context(), plan)
	if err != nil {
		http.Error(w, "execute: "+err.Error(), http.StatusInternalServerError)
		return
	}

	summary := orch.MergeResults(results)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck // best-effort response
		"plan":    plan,
		"results": results,
		"summary": summary,
	})
}

// --- Store ---

func newVectorStore() vector.VectorStore {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL != "" {
		return vector.NewQdrantClient()
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return vector.NewLocalStore(filepath.Join(home, ".agentos", "vectors"))
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
