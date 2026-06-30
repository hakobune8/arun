package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/kazyamaz200/agentos/internal/apphome"
	"github.com/kazyamaz200/agentos/internal/guideline"
	"github.com/kazyamaz200/agentos/internal/orchestrator"
)

type repositoryGuidelineRequest struct {
	Repo       string   `json:"repo"`
	BaseBranch string   `json:"baseBranch"`
	Title      string   `json:"title"`
	Content    string   `json:"content"`
	Rule       string   `json:"rule,omitempty"`
	Type       string   `json:"type,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Required   bool     `json:"required,omitempty"`
	Status     string   `json:"status,omitempty"`
}

func repositoryGuidelinesPath() string {
	return filepath.Join(apphome.Dir(), "repository-guidelines.json")
}

func repositoryGuidelineStore() (*guideline.RepositoryStore, error) {
	return guideline.NewRepositoryStore(repositoryGuidelinesPath())
}

func (s *Server) handleRepositoryGuidelines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listRepositoryGuidelines(w, r, user)
	case http.MethodPost:
		s.createRepositoryGuideline(w, r, user)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listRepositoryGuidelines(w http.ResponseWriter, r *http.Request, user *authUser) {
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	branch := defaultBaseBranch(r.URL.Query().Get("baseBranch"))
	if !s.requireAutomationPermission(w, r, user, "repository-guidelines.read", "repository", repo, "") {
		return
	}
	store, err := repositoryGuidelineStore()
	if err != nil {
		http.Error(w, "open repository guidelines: "+err.Error(), http.StatusInternalServerError)
		return
	}
	guidelines, err := store.List(r.Context(), &guideline.RepositoryGuidelineQuery{
		Repo:   repo,
		Branch: branch,
		Query:  r.URL.Query().Get("q"),
		Agent:  r.URL.Query().Get("agent"),
		Type:   r.URL.Query().Get("type"),
		Status: r.URL.Query().Get("status"),
		Limit:  parsePositiveInt(r.URL.Query().Get("limit"), 100),
	})
	if err != nil {
		http.Error(w, "list repository guidelines: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(guidelines) //nolint:errcheck // best-effort response
}

func (s *Server) createRepositoryGuideline(w http.ResponseWriter, r *http.Request, user *authUser) {
	var req repositoryGuidelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "repository-guidelines.create", "repository", req.Repo, "") {
		return
	}
	store, err := repositoryGuidelineStore()
	if err != nil {
		http.Error(w, "open repository guidelines: "+err.Error(), http.StatusInternalServerError)
		return
	}
	entry := &guideline.RepositoryGuideline{
		Repo:     req.Repo,
		Branch:   req.BaseBranch,
		Title:    req.Title,
		Content:  req.Content,
		Rule:     req.Rule,
		Type:     req.Type,
		Tags:     req.Tags,
		Required: req.Required,
		Status:   req.Status,
		Source:   "manual",
	}
	if err := store.Save(r.Context(), entry); err != nil {
		http.Error(w, "save repository guideline: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(entry) //nolint:errcheck // best-effort response
}

func (s *Server) handleRepositoryGuidelineItem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	id := repositoryGuidelineItemPath(r.URL.Path)
	if id == "" {
		http.Error(w, "repository guideline id is required", http.StatusBadRequest)
		return
	}
	store, err := repositoryGuidelineStore()
	if err != nil {
		http.Error(w, "open repository guidelines: "+err.Error(), http.StatusInternalServerError)
		return
	}
	entry, err := store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, "repository guideline not found", http.StatusNotFound)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "repository-guidelines.manage", "repository", entry.Repo, "") {
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.updateRepositoryGuideline(w, r, store, entry)
	case http.MethodDelete:
		archived, err := store.Archive(r.Context(), id)
		if err != nil {
			http.Error(w, "archive repository guideline: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(archived) //nolint:errcheck // best-effort response
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) updateRepositoryGuideline(w http.ResponseWriter, r *http.Request, store *guideline.RepositoryStore, entry *guideline.RepositoryGuideline) {
	var req repositoryGuidelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Title) != "" {
		entry.Title = req.Title
	}
	if strings.TrimSpace(req.Content) != "" || strings.TrimSpace(req.Rule) != "" {
		entry.Content = req.Content
		entry.Rule = req.Rule
	}
	if strings.TrimSpace(req.Type) != "" {
		entry.Type = req.Type
	}
	if strings.TrimSpace(req.Status) != "" {
		entry.Status = req.Status
	}
	if req.Tags != nil {
		entry.Tags = req.Tags
	}
	entry.Required = req.Required
	if err := store.Save(r.Context(), entry); err != nil {
		http.Error(w, "save repository guideline: "+err.Error(), http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(entry) //nolint:errcheck // best-effort response
}

func repositoryGuidelineItemPath(path string) string {
	rest := strings.TrimPrefix(path, "/api/repository-guidelines/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

func loadRepositoryGuidelinesForRecord(ctx context.Context, record *orchestrationRecord) []guideline.RepositoryGuideline {
	if record == nil || strings.TrimSpace(record.RepoPath) == "" {
		return nil
	}
	store, err := repositoryGuidelineStore()
	if err != nil {
		slogWarnRepositoryGuidelines(record, "open repository guidelines", err)
		return nil
	}
	loaded, err := guideline.LoadRepositoryDirectory(ctx, store, record.RepoPath, record.Repo, record.BaseBranch)
	if err != nil {
		slogWarnRepositoryGuidelines(record, "load repository guideline directory", err)
		return nil
	}
	return loaded
}

func repositoryGuidelinesForPlanning(ctx context.Context, record *orchestrationRecord, agentName string) []guideline.RepositoryGuideline {
	if record == nil {
		return nil
	}
	store, err := repositoryGuidelineStore()
	if err != nil {
		slogWarnRepositoryGuidelines(record, "open repository guidelines", err)
		return nil
	}
	entries, err := store.List(ctx, &guideline.RepositoryGuidelineQuery{
		Repo:   record.Repo,
		Branch: record.BaseBranch,
		Query:  record.Task,
		Agent:  agentName,
		Status: guideline.RepositoryGuidelineActive,
		Limit:  12,
	})
	if err != nil {
		slogWarnRepositoryGuidelines(record, "list repository guidelines", err)
		return nil
	}
	return entries
}

func taskWithRepositoryGuidelines(task string, entries []guideline.RepositoryGuideline) string {
	if len(entries) == 0 {
		return task
	}
	var b strings.Builder
	b.WriteString(task)
	b.WriteString("\n\nRepository guidelines to apply:\n")
	for i := range entries {
		entry := &entries[i]
		required := "advisory"
		if entry.Required {
			required = "required"
		}
		b.WriteString(fmt.Sprintf("- [%s/%s] %s: %s\n", required, entry.Type, entry.Title, entry.Content))
	}
	return b.String()
}

func applyRepositoryGuidelinesToPlan(plan *orchestrator.TaskPlan, entries []guideline.RepositoryGuideline) []guideline.AppliedRepositoryGuideline {
	if plan == nil || len(entries) == 0 || len(plan.Subtasks) == 0 {
		return nil
	}
	applied := make([]guideline.AppliedRepositoryGuideline, 0, len(plan.Subtasks)*len(entries))
	for i := range plan.Subtasks {
		subtask := &plan.Subtasks[i]
		var b strings.Builder
		b.WriteString(strings.TrimSpace(subtask.Description))
		b.WriteString("\n\nRepository guidelines to follow:\n")
		for j := range entries {
			entry := &entries[j]
			required := "advisory"
			if entry.Required {
				required = "required"
			}
			b.WriteString(fmt.Sprintf("- [%s/%s] %s: %s\n", required, entry.Type, entry.Title, entry.Content))
			applied = append(applied, guideline.AppliedRepositoryGuideline{
				SubtaskID:   subtask.ID,
				AgentName:   subtask.AgentName,
				GuidelineID: entry.ID,
				Title:       entry.Title,
				Required:    entry.Required,
				Reason:      "attached to subtask context",
			})
		}
		subtask.Description = strings.TrimSpace(b.String())
	}
	return applied
}

func missedRequiredGuidelines(entries []guideline.RepositoryGuideline, applied []guideline.AppliedRepositoryGuideline) []guideline.RepositoryGuideline {
	if len(entries) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for i := range applied {
		seen[applied[i].GuidelineID] = true
	}
	var missed []guideline.RepositoryGuideline
	for i := range entries {
		entry := entries[i]
		if entry.Required && !seen[entry.ID] {
			missed = append(missed, entry)
		}
	}
	return missed
}

func slogWarnRepositoryGuidelines(record *orchestrationRecord, message string, err error) {
	id := ""
	if record != nil {
		id = record.ID
	}
	slog.Warn(message, "id", id, "error", err)
}
