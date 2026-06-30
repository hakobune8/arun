package guideline

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	RepositoryGuidelineActive   = "active"
	RepositoryGuidelineArchived = "archived"
)

type RepositoryGuideline struct {
	ID        string    `json:"id" yaml:"id"`
	Repo      string    `json:"repo,omitempty" yaml:"-"`
	Branch    string    `json:"branch,omitempty" yaml:"-"`
	Title     string    `json:"title" yaml:"title"`
	Content   string    `json:"content" yaml:"content"`
	Rule      string    `json:"rule,omitempty" yaml:"rule,omitempty"`
	Type      string    `json:"type,omitempty" yaml:"type,omitempty"`
	Tags      []string  `json:"tags,omitempty" yaml:"tags,omitempty"`
	Required  bool      `json:"required,omitempty" yaml:"required,omitempty"`
	Status    string    `json:"status" yaml:"status,omitempty"`
	Source    string    `json:"source,omitempty" yaml:"-"`
	Path      string    `json:"path,omitempty" yaml:"-"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type RepositoryGuidelineQuery struct {
	Repo   string
	Branch string
	Query  string
	Agent  string
	Type   string
	Status string
	Limit  int
}

type AppliedRepositoryGuideline struct {
	SubtaskID   string `json:"subtaskId,omitempty"`
	AgentName   string `json:"agentName,omitempty"`
	GuidelineID string `json:"guidelineId"`
	Title       string `json:"title"`
	Required    bool   `json:"required,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type RepositoryStore struct {
	mu         sync.RWMutex
	path       string
	guidelines []RepositoryGuideline
}

func NewRepositoryStore(path string) (*RepositoryStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create repository guideline dir: %w", err)
	}
	s := &RepositoryStore{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("read repository guidelines: %w", err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.guidelines); err != nil {
		return nil, fmt.Errorf("parse repository guidelines: %w", err)
	}
	return s, nil
}

func (s *RepositoryStore) Save(_ context.Context, guideline *RepositoryGuideline) error {
	if guideline == nil {
		return fmt.Errorf("guideline is required")
	}
	now := time.Now().UTC()
	guideline.Repo = NormalizeRepository(guideline.Repo)
	guideline.Branch = NormalizeBranch(guideline.Branch)
	guideline.Title = strings.TrimSpace(guideline.Title)
	guideline.Content = strings.TrimSpace(firstNonEmpty(guideline.Content, guideline.Rule))
	guideline.Type = normalizeGuidelineType(guideline.Type)
	guideline.Status = normalizeGuidelineStatus(guideline.Status)
	if guideline.Repo == "" {
		return fmt.Errorf("repo is required")
	}
	if guideline.Title == "" {
		return fmt.Errorf("title is required")
	}
	if guideline.Content == "" {
		return fmt.Errorf("content is required")
	}
	if guideline.CreatedAt.IsZero() {
		guideline.CreatedAt = now
	}
	guideline.UpdatedAt = now

	s.mu.Lock()
	defer s.mu.Unlock()
	if guideline.ID == "" {
		guideline.ID = fmt.Sprintf("repo-gl-%d-%d", now.UnixNano(), len(s.guidelines)+1)
	}
	for i := range s.guidelines {
		if s.guidelines[i].ID == guideline.ID {
			s.guidelines[i] = *guideline
			return s.flush()
		}
	}
	s.guidelines = append(s.guidelines, *guideline)
	return s.flush()
}

func (s *RepositoryStore) List(_ context.Context, query *RepositoryGuidelineQuery) ([]RepositoryGuideline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if query == nil {
		query = &RepositoryGuidelineQuery{}
	}
	repo := NormalizeRepository(query.Repo)
	branch := NormalizeBranch(query.Branch)
	status := strings.TrimSpace(query.Status)
	if status == "" {
		status = RepositoryGuidelineActive
	}
	var scored []scoredGuideline
	for i := range s.guidelines {
		g := &s.guidelines[i]
		if repo != "" && NormalizeRepository(g.Repo) != repo {
			continue
		}
		if branch != "" && NormalizeBranch(g.Branch) != branch {
			continue
		}
		if status != "" && g.Status != status {
			continue
		}
		if query.Type != "" && g.Type != query.Type {
			continue
		}
		score := guidelineScore(g, query.Query, query.Agent)
		if strings.TrimSpace(query.Query) != "" && score == 0 {
			continue
		}
		scored = append(scored, scoredGuideline{guideline: *g, score: score})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if scored[i].guideline.Required != scored[j].guideline.Required {
			return scored[i].guideline.Required
		}
		return scored[i].guideline.UpdatedAt.After(scored[j].guideline.UpdatedAt)
	})
	results := make([]RepositoryGuideline, 0, len(scored))
	for i := range scored {
		results = append(results, scored[i].guideline)
	}
	if query.Limit > 0 && len(results) > query.Limit {
		results = results[:query.Limit]
	}
	return results, nil
}

func (s *RepositoryStore) Get(_ context.Context, id string) (*RepositoryGuideline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i := range s.guidelines {
		if s.guidelines[i].ID == id {
			g := s.guidelines[i]
			return &g, nil
		}
	}
	return nil, fmt.Errorf("repository guideline not found: %s", id)
}

func (s *RepositoryStore) Archive(ctx context.Context, id string) (*RepositoryGuideline, error) {
	g, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	g.Status = RepositoryGuidelineArchived
	if err := s.Save(ctx, g); err != nil {
		return nil, err
	}
	return g, nil
}

func (s *RepositoryStore) flush() error {
	data, err := json.MarshalIndent(s.guidelines, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal repository guidelines: %w", err)
	}
	return os.WriteFile(s.path, data, 0o600)
}

func LoadRepositoryDirectory(ctx context.Context, store *RepositoryStore, repoPath, repo, branch string) ([]RepositoryGuideline, error) {
	if store == nil {
		return nil, fmt.Errorf("store is required")
	}
	dir := filepath.Join(repoPath, ".agentos", "guidelines")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read repository guidelines: %w", err)
	}
	var loaded []RepositoryGuideline
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		var guidelines []RepositoryGuideline
		switch ext {
		case ".yaml", ".yml":
			guidelines, err = readYAMLGuidelines(path)
		case ".md":
			guidelines, err = readMarkdownGuideline(path)
		default:
			continue
		}
		if err != nil {
			return nil, err
		}
		for i := range guidelines {
			guidelines[i].Repo = repo
			guidelines[i].Branch = branch
			guidelines[i].Source = "repository"
			guidelines[i].Path = filepath.ToSlash(filepath.Join(".agentos", "guidelines", entry.Name()))
			if strings.TrimSpace(guidelines[i].ID) == "" {
				guidelines[i].ID = repositoryGuidelineID(guidelines[i].Repo, guidelines[i].Branch, guidelines[i].Path, guidelines[i].Title)
			}
			if err := store.Save(ctx, &guidelines[i]); err != nil {
				return nil, err
			}
			loaded = append(loaded, guidelines[i])
		}
	}
	return loaded, nil
}

func readYAMLGuidelines(path string) ([]RepositoryGuideline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	var wrapper struct {
		Guidelines []RepositoryGuideline `yaml:"guidelines"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err == nil && len(wrapper.Guidelines) > 0 {
		return wrapper.Guidelines, nil
	}
	var list []RepositoryGuideline
	if err := yaml.Unmarshal(data, &list); err == nil && len(list) > 0 {
		return list, nil
	}
	var single RepositoryGuideline
	if err := yaml.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	return []RepositoryGuideline{single}, nil
}

func readMarkdownGuideline(path string) ([]RepositoryGuideline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	content := strings.TrimSpace(string(data))
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			title = strings.TrimSpace(strings.TrimLeft(line, "#"))
			break
		}
	}
	return []RepositoryGuideline{{Title: title, Content: content, Type: "general"}}, nil
}

type scoredGuideline struct {
	guideline RepositoryGuideline
	score     int
}

func guidelineScore(g *RepositoryGuideline, query, agent string) int {
	text := strings.ToLower(g.Title + " " + g.Content + " " + g.Type + " " + strings.Join(g.Tags, " "))
	score := 0
	if g.Required {
		score += 4
	}
	for _, token := range strings.Fields(strings.ToLower(query + " " + agent)) {
		token = strings.Trim(token, ".,:;()[]{}")
		if len(token) >= 3 && strings.Contains(text, token) {
			score += 2
		}
	}
	if agent != "" {
		for _, tag := range g.Tags {
			if strings.EqualFold(tag, agent) || strings.Contains(strings.ToLower(agent), strings.ToLower(tag)) {
				score += 3
			}
		}
	}
	return score
}

func NormalizeRepository(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "."
	}
	repo = strings.TrimSuffix(repo, ".git")
	repo = strings.TrimPrefix(repo, "https://github.com/")
	return repo
}

func NormalizeBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return "main"
	}
	return branch
}

func normalizeGuidelineType(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "general"
	}
	return value
}

func normalizeGuidelineStatus(value string) string {
	switch strings.TrimSpace(value) {
	case RepositoryGuidelineActive, RepositoryGuidelineArchived:
		return value
	default:
		return RepositoryGuidelineActive
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func repositoryGuidelineID(repo, branch, path, title string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.Join([]string{
		NormalizeRepository(repo),
		NormalizeBranch(branch),
		strings.TrimSpace(path),
		strings.TrimSpace(title),
	}, "\x00")))
	return fmt.Sprintf("repo-gl-%x", h.Sum64())
}
