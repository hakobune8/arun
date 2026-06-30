package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// RepositoryMemoryApproved is available for future planning.
	RepositoryMemoryApproved = "approved"
	// RepositoryMemoryPending waits for user approval.
	RepositoryMemoryPending = "pending"
	// RepositoryMemoryArchived is hidden from planning but retained for audit.
	RepositoryMemoryArchived = "archived"
)

// RepositoryEntry is durable memory scoped to one repository and branch.
type RepositoryEntry struct {
	ID         string     `json:"id"`
	Repo       string     `json:"repo"`
	Branch     string     `json:"branch"`
	Type       string     `json:"type"`
	Content    string     `json:"content"`
	Source     string     `json:"source,omitempty"`
	RunID      string     `json:"runId,omitempty"`
	SubtaskID  string     `json:"subtaskId,omitempty"`
	Status     string     `json:"status"`
	Pinned     bool       `json:"pinned,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
	ApprovedAt *time.Time `json:"approvedAt,omitempty"`
	ApprovedBy string     `json:"approvedBy,omitempty"`
}

// RepositoryQuery filters repository memory.
type RepositoryQuery struct {
	Repo   string
	Branch string
	Query  string
	Type   string
	Status string
	Limit  int
}

// RepositoryStore persists repository-scoped memory in one JSON file.
type RepositoryStore struct {
	mu      sync.RWMutex
	path    string
	entries []RepositoryEntry
}

// NewRepositoryStore loads a repository memory store from path.
func NewRepositoryStore(path string) (*RepositoryStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create repository memory dir: %w", err)
	}
	s := &RepositoryStore{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read repository memory: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.entries); err != nil {
		return nil, fmt.Errorf("parse repository memory: %w", err)
	}
	return s, nil
}

// Save inserts or replaces an entry.
func (s *RepositoryStore) Save(_ context.Context, entry *RepositoryEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is required")
	}
	now := time.Now().UTC()
	entry.Repo = NormalizeRepository(entry.Repo)
	entry.Branch = NormalizeBranch(entry.Branch)
	entry.Type = normalizeMemoryType(entry.Type)
	entry.Status = normalizeMemoryStatus(entry.Status)
	entry.Content = strings.TrimSpace(entry.Content)
	if entry.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if entry.Content == "" {
		return fmt.Errorf("content is required")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("repo-mem-%d-%d", now.UnixNano(), len(s.entries)+1)
	}
	for i := range s.entries {
		if s.entries[i].ID == entry.ID {
			s.entries[i] = *entry
			return s.flush()
		}
	}
	s.entries = append(s.entries, *entry)
	return s.flush()
}

// List returns entries matching query, newest first with pinned entries first.
func (s *RepositoryStore) List(_ context.Context, query *RepositoryQuery) ([]RepositoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if query == nil {
		query = &RepositoryQuery{}
	}
	repo := NormalizeRepository(query.Repo)
	branch := NormalizeBranch(query.Branch)
	q := strings.ToLower(strings.TrimSpace(query.Query))
	status := strings.TrimSpace(query.Status)
	memType := strings.TrimSpace(query.Type)

	var results []RepositoryEntry
	for i := range s.entries {
		entry := &s.entries[i]
		if repo != "" && NormalizeRepository(entry.Repo) != repo {
			continue
		}
		if branch != "" && NormalizeBranch(entry.Branch) != branch {
			continue
		}
		if status != "" && entry.Status != status {
			continue
		}
		if memType != "" && entry.Type != memType {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(entry.Content+" "+entry.Type+" "+entry.Source), q) {
			continue
		}
		results = append(results, *entry)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Pinned != results[j].Pinned {
			return results[i].Pinned
		}
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

// Get returns one entry by ID.
func (s *RepositoryStore) Get(_ context.Context, id string) (*RepositoryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.entries {
		if s.entries[i].ID == id {
			entry := s.entries[i]
			return &entry, nil
		}
	}
	return nil, fmt.Errorf("repository memory not found: %s", id)
}

// Approve marks a pending entry as approved.
func (s *RepositoryStore) Approve(ctx context.Context, id, actor string) (*RepositoryEntry, error) {
	entry, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	entry.Status = RepositoryMemoryApproved
	entry.ApprovedAt = &now
	entry.ApprovedBy = strings.TrimSpace(actor)
	if err := s.Save(ctx, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// Delete removes an entry by ID.
func (s *RepositoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.entries {
		if s.entries[i].ID == id {
			s.entries = append(s.entries[:i], s.entries[i+1:]...)
			return s.flush()
		}
	}
	return fmt.Errorf("repository memory not found: %s", id)
}

func (s *RepositoryStore) flush() error {
	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal repository memory: %w", err)
	}
	return os.WriteFile(s.path, data, 0o600)
}

// NormalizeRepository normalizes repository identity for memory scoping.
func NormalizeRepository(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "."
	}
	repo = strings.TrimSuffix(repo, ".git")
	repo = strings.TrimPrefix(repo, "https://github.com/")
	return repo
}

// NormalizeBranch returns a stable branch key.
func NormalizeBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "main"
	}
	return branch
}

func normalizeMemoryType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "note"
	}
	return value
}

func normalizeMemoryStatus(value string) string {
	switch strings.TrimSpace(value) {
	case RepositoryMemoryApproved, RepositoryMemoryPending, RepositoryMemoryArchived:
		return value
	default:
		return RepositoryMemoryApproved
	}
}
