package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/apphome"
	"github.com/kazyamaz200/agentos/internal/memory"
	"github.com/kazyamaz200/agentos/internal/orchestrator"
)

type repositoryMemoryRequest struct {
	Repo       string `json:"repo"`
	BaseBranch string `json:"baseBranch"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	Status     string `json:"status,omitempty"`
	Pinned     bool   `json:"pinned,omitempty"`
}

func repositoryMemoryPath() string {
	return filepath.Join(apphome.Dir(), "repository-memory.json")
}

func repositoryMemoryStore() (*memory.RepositoryStore, error) {
	return memory.NewRepositoryStore(repositoryMemoryPath())
}

func (s *Server) handleRepositoryMemory(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listRepositoryMemory(w, r, user)
	case http.MethodPost:
		s.createRepositoryMemory(w, r, user)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listRepositoryMemory(w http.ResponseWriter, r *http.Request, user *authUser) {
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	branch := defaultBaseBranch(r.URL.Query().Get("baseBranch"))
	if !s.requireAutomationPermission(w, r, user, "repository-memory.read", "repository", repo, "") {
		return
	}
	store, err := repositoryMemoryStore()
	if err != nil {
		http.Error(w, "open repository memory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	entries, err := store.List(r.Context(), &memory.RepositoryQuery{
		Repo:   repo,
		Branch: branch,
		Query:  r.URL.Query().Get("q"),
		Type:   r.URL.Query().Get("type"),
		Status: r.URL.Query().Get("status"),
		Limit:  parsePositiveInt(r.URL.Query().Get("limit"), 100),
	})
	if err != nil {
		http.Error(w, "list repository memory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(entries) //nolint:errcheck // best-effort response
}

func (s *Server) createRepositoryMemory(w http.ResponseWriter, r *http.Request, user *authUser) {
	var req repositoryMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "repository-memory.create", "repository", req.Repo, "") {
		return
	}
	store, err := repositoryMemoryStore()
	if err != nil {
		http.Error(w, "open repository memory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	entry := &memory.RepositoryEntry{
		Repo:       req.Repo,
		Branch:     req.BaseBranch,
		Type:       req.Type,
		Content:    req.Content,
		Status:     memory.RepositoryMemoryApproved,
		Pinned:     req.Pinned,
		Source:     "manual",
		ApprovedAt: &now,
		ApprovedBy: actorLogin(user),
	}
	if req.Status != "" {
		entry.Status = req.Status
	}
	if err := store.Save(r.Context(), entry); err != nil {
		http.Error(w, "save repository memory: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(entry) //nolint:errcheck // best-effort response
}

func (s *Server) handleRepositoryMemoryItem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	id, action := repositoryMemoryItemPath(r.URL.Path)
	if id == "" {
		http.Error(w, "repository memory id is required", http.StatusBadRequest)
		return
	}
	store, err := repositoryMemoryStore()
	if err != nil {
		http.Error(w, "open repository memory: "+err.Error(), http.StatusInternalServerError)
		return
	}
	entry, err := store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "repository memory not found", http.StatusNotFound)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "repository-memory.manage", "repository", entry.Repo, "") {
		return
	}

	switch {
	case action == "approve" && r.Method == http.MethodPost:
		approved, err := store.Approve(r.Context(), id, actorLogin(user))
		if err != nil {
			http.Error(w, "approve repository memory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(approved) //nolint:errcheck // best-effort response
	case action == "" && r.Method == http.MethodPut:
		s.updateRepositoryMemory(w, r, store, entry)
	case action == "" && r.Method == http.MethodDelete:
		if err := store.Delete(r.Context(), id); err != nil {
			http.Error(w, "delete repository memory: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) updateRepositoryMemory(w http.ResponseWriter, r *http.Request, store *memory.RepositoryStore, entry *memory.RepositoryEntry) {
	var req repositoryMemoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Content) != "" {
		entry.Content = req.Content
	}
	if strings.TrimSpace(req.Type) != "" {
		entry.Type = req.Type
	}
	if strings.TrimSpace(req.Status) != "" {
		entry.Status = req.Status
	}
	entry.Pinned = req.Pinned
	if err := store.Save(r.Context(), entry); err != nil {
		http.Error(w, "save repository memory: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(entry) //nolint:errcheck // best-effort response
}

func repositoryMemoryItemPath(path string) (id, action string) {
	rest := strings.TrimPrefix(path, "/api/repository-memory/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) > 0 {
		id = parts[0]
	}
	if len(parts) > 1 {
		action = parts[1]
	}
	return id, action
}

func parsePositiveInt(value string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func repositoryMemoryForPlanning(ctx context.Context, record *orchestrationRecord) []memory.RepositoryEntry {
	store, err := repositoryMemoryStore()
	if err != nil {
		slogWarnRepositoryMemory(record, "open repository memory", err)
		return nil
	}
	entries, err := store.List(ctx, &memory.RepositoryQuery{
		Repo:   record.Repo,
		Branch: record.BaseBranch,
		Query:  record.Task,
		Status: memory.RepositoryMemoryApproved,
		Limit:  8,
	})
	if err != nil {
		slogWarnRepositoryMemory(record, "list repository memory", err)
		return nil
	}
	return entries
}

func taskWithRepositoryMemory(task string, entries []memory.RepositoryEntry) string {
	if len(entries) == 0 {
		return task
	}
	var b strings.Builder
	b.WriteString(task)
	b.WriteString("\n\nRepository memory to apply:\n")
	for i := range entries {
		entry := &entries[i]
		pinned := ""
		if entry.Pinned {
			pinned = " pinned"
		}
		b.WriteString(fmt.Sprintf("- [%s%s] %s\n", entry.Type, pinned, entry.Content))
	}
	return b.String()
}

func proposeRepositoryMemory(ctx context.Context, record *orchestrationRecord, results []orchestrator.SubtaskResult) []memory.RepositoryEntry {
	proposals := repositoryMemoryProposals(record, results)
	if len(proposals) == 0 {
		return nil
	}
	store, err := repositoryMemoryStore()
	if err != nil {
		slogWarnRepositoryMemory(record, "open repository memory", err)
		return nil
	}
	saved := make([]memory.RepositoryEntry, 0, len(proposals))
	for i := range proposals {
		if err := store.Save(ctx, &proposals[i]); err != nil {
			slogWarnRepositoryMemory(record, "save repository memory proposal", err)
			continue
		}
		saved = append(saved, proposals[i])
	}
	return saved
}

func repositoryMemoryProposals(record *orchestrationRecord, results []orchestrator.SubtaskResult) []memory.RepositoryEntry {
	if record == nil || record.Repo == "" {
		return nil
	}
	var proposals []memory.RepositoryEntry
	now := time.Now().UTC()
	base := memory.RepositoryEntry{
		Repo:      record.Repo,
		Branch:    record.BaseBranch,
		Source:    "orchestration",
		RunID:     record.ID,
		Status:    memory.RepositoryMemoryPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if commands := validationCommands(record.Plan, results); len(commands) > 0 {
		entry := base
		entry.Type = "validation"
		entry.Content = "Validation commands observed for this repository: " + strings.Join(commands, "; ")
		proposals = append(proposals, entry)
	}
	if len(record.Agents) > 0 {
		entry := base
		entry.Type = "workflow"
		entry.Content = fmt.Sprintf("Task %q was handled with agents: %s.", truncateForMemory(record.Task, 160), strings.Join(record.Agents, ", "))
		proposals = append(proposals, entry)
	}
	if pitfalls := failureSummaries(results); len(pitfalls) > 0 {
		entry := base
		entry.Type = "pitfall"
		entry.Content = "Previous orchestration failures to consider: " + strings.Join(pitfalls, "; ")
		proposals = append(proposals, entry)
	}
	return proposals
}

func validationCommands(plan *orchestrator.TaskPlan, results []orchestrator.SubtaskResult) []string {
	seen := map[string]bool{}
	var commands []string
	if plan != nil {
		for _, subtask := range plan.Subtasks {
			if subtask.QualityGate == nil {
				continue
			}
			for _, command := range subtask.QualityGate.ValidationCommands {
				addUniqueMemoryValue(&commands, seen, command)
			}
		}
	}
	for _, result := range results {
		if result.QualityGate == nil {
			continue
		}
		for _, check := range result.QualityGate.Checks {
			if check.Type == "command" {
				addUniqueMemoryValue(&commands, seen, check.Target)
			}
		}
	}
	sort.Strings(commands)
	return commands
}

func failureSummaries(results []orchestrator.SubtaskResult) []string {
	seen := map[string]bool{}
	var failures []string
	for _, result := range results {
		if result.Success || strings.TrimSpace(result.Error) == "" {
			continue
		}
		addUniqueMemoryValue(&failures, seen, result.SubtaskID+": "+truncateForMemory(result.Error, 180))
	}
	return failures
}

func addUniqueMemoryValue(values *[]string, seen map[string]bool, value string) {
	value = strings.TrimSpace(value)
	if value == "" || seen[value] {
		return
	}
	seen[value] = true
	*values = append(*values, value)
}

func truncateForMemory(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func slogWarnRepositoryMemory(record *orchestrationRecord, message string, err error) {
	id := ""
	if record != nil {
		id = record.ID
	}
	slog.Warn(message, "id", id, "error", err)
}
