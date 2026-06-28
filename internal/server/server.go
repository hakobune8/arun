package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/kazyamaz200/agentos/internal/embedding"
	"github.com/kazyamaz200/agentos/internal/search"
	"github.com/kazyamaz200/agentos/internal/vector"
)

//go:embed static
var staticFS embed.FS

type Server struct {
	port    int
	server  *http.Server
	search  *search.Service
}

func NewServer(port int) *Server {
	vs := newVectorStore()
	emb := embedding.NewLiteLLMEmbedder()
	svc := search.NewService(vs, emb)

	mux := http.NewServeMux()
	s := &Server{
		port:   port,
		search: svc,
	}

	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunDetail)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/health", s.handleHealth)

	staticSub, err := fs.Sub(staticFS, "static")
	if err == nil {
		mux.Handle("/", http.FileServer(http.FS(staticSub)))
	}

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s
}

func (s *Server) Start() error {
	fmt.Printf("AgentOS Web UI starting on http://localhost:%d\n", s.port)
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	runsDir, _ := os.UserHomeDir()
	runsDir = filepath.Join(runsDir, ".agentos", "runs")

	entries, err := os.ReadDir(runsDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

func (s *Server) handleRunDetail(w http.ResponseWriter, r *http.Request) {
	id := filepath.Base(r.URL.Path)
	runsDir, _ := os.UserHomeDir()
	runDir := filepath.Join(runsDir, ".agentos", "runs", id)

	artifacts := []string{
		"task.yaml", "profile.yaml", "plan.json",
		"summary.md", "pr_body.md", "diff.patch",
		"test.log", "lint.log",
	}

	result := map[string]interface{}{
		"id":         id,
		"artifacts":  map[string]string{},
	}

	for _, name := range artifacts {
		path := filepath.Join(runDir, name)
		if data, err := os.ReadFile(path); err == nil {
			result["artifacts"].(map[string]string)[name] = string(data)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query param q required", http.StatusBadRequest)
		return
	}

	results, err := s.search.Search(r.Context(), query, search.TypeAll, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func newVectorStore() vector.VectorStore {
	qdrantURL := os.Getenv("QDRANT_URL")
	if qdrantURL != "" {
		return vector.NewQdrantClient()
	}
	home, _ := os.UserHomeDir()
	return vector.NewLocalStore(home + "/.agentos/vectors")
}
