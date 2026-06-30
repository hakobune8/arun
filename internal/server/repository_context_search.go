package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/guideline"
	"github.com/kazyamaz200/agentos/internal/memory"
)

type repositoryContextSearchQuery struct {
	Repo   string
	Branch string
	Query  string
	Source string
	Limit  int
}

type repositoryContextSearchResult struct {
	Source    string                 `json:"source"`
	ID        string                 `json:"id"`
	Title     string                 `json:"title"`
	Content   string                 `json:"content"`
	Repo      string                 `json:"repo"`
	Branch    string                 `json:"branch"`
	RunID     string                 `json:"runId,omitempty"`
	URL       string                 `json:"url,omitempty"`
	Score     float64                `json:"score"`
	CreatedAt *time.Time             `json:"createdAt,omitempty"`
	UpdatedAt *time.Time             `json:"updatedAt,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

func repositoryContextSearch(ctx context.Context, q repositoryContextSearchQuery) ([]repositoryContextSearchResult, error) {
	q.Repo = memory.NormalizeRepository(q.Repo)
	q.Branch = memory.NormalizeBranch(q.Branch)
	q.Query = strings.TrimSpace(q.Query)
	if q.Repo == "" {
		return nil, fmt.Errorf("repo is required")
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	var results []repositoryContextSearchResult
	if contextSourceAllowed(q.Source, "memory") {
		mem, err := repositoryContextMemory(ctx, q)
		if err != nil {
			return nil, err
		}
		results = append(results, mem...)
	}
	if contextSourceAllowed(q.Source, "guideline") {
		gls, err := repositoryContextGuidelines(ctx, q)
		if err != nil {
			return nil, err
		}
		results = append(results, gls...)
	}
	if contextSourceAllowed(q.Source, "run") || contextSourceAllowed(q.Source, "artifact") || contextSourceAllowed(q.Source, "github") {
		runs, err := repositoryContextRuns(q)
		if err != nil {
			return nil, err
		}
		results = append(results, runs...)
	}
	if contextSourceAllowed(q.Source, "code") {
		code, err := repositoryContextCode(q)
		if err == nil {
			results = append(results, code...)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return contextResultTime(&results[i]).After(contextResultTime(&results[j]))
	})
	if len(results) > q.Limit {
		results = results[:q.Limit]
	}
	return results, nil
}

func repositoryContextMemory(ctx context.Context, q repositoryContextSearchQuery) ([]repositoryContextSearchResult, error) {
	store, err := repositoryMemoryStore()
	if err != nil {
		return nil, err
	}
	entries, err := store.List(ctx, &memory.RepositoryQuery{Repo: q.Repo, Branch: q.Branch, Query: q.Query, Limit: q.Limit})
	if err != nil {
		return nil, err
	}
	results := make([]repositoryContextSearchResult, 0, len(entries))
	for i := range entries {
		entry := entries[i]
		score := contextScore(q.Query, entry.Content, entry.Type, entry.Source)
		if q.Query != "" && score == 0 {
			continue
		}
		results = append(results, repositoryContextSearchResult{
			Source:    "memory",
			ID:        entry.ID,
			Title:     entry.Type,
			Content:   entry.Content,
			Repo:      entry.Repo,
			Branch:    entry.Branch,
			RunID:     entry.RunID,
			Score:     score,
			CreatedAt: &entry.CreatedAt,
			UpdatedAt: &entry.UpdatedAt,
			Metadata: map[string]interface{}{
				"type":   entry.Type,
				"status": entry.Status,
				"pinned": entry.Pinned,
				"source": entry.Source,
			},
		})
	}
	return results, nil
}

func repositoryContextGuidelines(ctx context.Context, q repositoryContextSearchQuery) ([]repositoryContextSearchResult, error) {
	store, err := repositoryGuidelineStore()
	if err != nil {
		return nil, err
	}
	entries, err := store.List(ctx, &guideline.RepositoryGuidelineQuery{Repo: q.Repo, Branch: q.Branch, Query: q.Query, Status: guideline.RepositoryGuidelineActive, Limit: q.Limit})
	if err != nil {
		return nil, err
	}
	results := make([]repositoryContextSearchResult, 0, len(entries))
	for i := range entries {
		entry := entries[i]
		score := contextScore(q.Query, entry.Title, entry.Content, entry.Type, strings.Join(entry.Tags, " "))
		if entry.Required {
			score += 2
		}
		results = append(results, repositoryContextSearchResult{
			Source:    "guideline",
			ID:        entry.ID,
			Title:     entry.Title,
			Content:   entry.Content,
			Repo:      entry.Repo,
			Branch:    entry.Branch,
			Score:     score,
			CreatedAt: &entry.CreatedAt,
			UpdatedAt: &entry.UpdatedAt,
			Metadata: map[string]interface{}{
				"type":     entry.Type,
				"required": entry.Required,
				"source":   entry.Source,
				"path":     entry.Path,
				"tags":     entry.Tags,
			},
		})
	}
	return results, nil
}

func repositoryContextRuns(q repositoryContextSearchQuery) ([]repositoryContextSearchResult, error) {
	records, err := listOrchestrationRecords()
	if err != nil {
		return nil, err
	}
	var results []repositoryContextSearchResult
	for _, record := range records {
		if record == nil || memory.NormalizeRepository(record.Repo) != q.Repo || memory.NormalizeBranch(record.BaseBranch) != q.Branch {
			continue
		}
		updated := record.UpdatedAt
		if contextSourceAllowed(q.Source, "run") {
			content := strings.TrimSpace(record.Task + "\n" + record.Summary + "\n" + record.Error)
			if score := contextScore(q.Query, record.ID, record.Status, content); q.Query == "" || score > 0 {
				results = append(results, repositoryContextSearchResult{
					Source:    "run",
					ID:        record.ID,
					Title:     record.Status,
					Content:   content,
					Repo:      record.Repo,
					Branch:    record.BaseBranch,
					RunID:     record.ID,
					Score:     score,
					CreatedAt: &record.CreatedAt,
					UpdatedAt: &updated,
					Metadata: map[string]interface{}{
						"agents":   record.Agents,
						"strategy": record.Strategy,
						"llm":      record.LLMPreset,
					},
				})
			}
		}
		if contextSourceAllowed(q.Source, "artifact") {
			results = append(results, repositoryContextArtifactResults(q, record, updated)...)
		}
		if contextSourceAllowed(q.Source, "github") && record.GitHub != nil {
			results = append(results, repositoryContextGitHubResults(q, record, updated)...)
		}
	}
	return results, nil
}

func repositoryContextArtifactResults(q repositoryContextSearchQuery, record *orchestrationRecord, updated time.Time) []repositoryContextSearchResult {
	var results []repositoryContextSearchResult
	if record.Plan != nil {
		for i := range record.Plan.Subtasks {
			st := record.Plan.Subtasks[i]
			if score := contextScore(q.Query, st.ID, st.AgentName, st.Description); q.Query == "" || score > 0 {
				results = append(results, repositoryContextSearchResult{
					Source:    "artifact",
					ID:        record.ID + ":" + st.ID,
					Title:     "Subtask " + st.ID,
					Content:   st.Description,
					Repo:      record.Repo,
					Branch:    record.BaseBranch,
					RunID:     record.ID,
					Score:     score,
					CreatedAt: &record.CreatedAt,
					UpdatedAt: &updated,
					Metadata:  map[string]interface{}{"agent": st.AgentName},
				})
			}
		}
	}
	for i := range record.Results {
		result := record.Results[i]
		content := strings.TrimSpace(result.Output + "\n" + result.Error + "\n" + result.Diff)
		if score := contextScore(q.Query, result.SubtaskID, content); content != "" && (q.Query == "" || score > 0) {
			results = append(results, repositoryContextSearchResult{
				Source:    "artifact",
				ID:        record.ID + ":" + result.SubtaskID + ":result",
				Title:     "Result " + result.SubtaskID,
				Content:   content,
				Repo:      record.Repo,
				Branch:    record.BaseBranch,
				RunID:     record.ID,
				Score:     score,
				CreatedAt: &record.CreatedAt,
				UpdatedAt: &updated,
				Metadata:  map[string]interface{}{"success": result.Success},
			})
		}
	}
	return results
}

func repositoryContextGitHubResults(q repositoryContextSearchQuery, record *orchestrationRecord, updated time.Time) []repositoryContextSearchResult {
	var results []repositoryContextSearchResult
	items := []struct {
		id      string
		title   string
		content string
		url     string
	}{
		{"issue", record.GitHub.IssueTitle, record.GitHub.IssueURL, record.GitHub.IssueURL},
		{"pr", record.GitHub.PRTitle, record.GitHub.PullRequestURL, record.GitHub.PullRequestURL},
		{"source", record.GitHub.SourceIssueTitle, record.GitHub.SourceIssueURL, record.GitHub.SourceIssueURL},
	}
	for _, item := range items {
		if strings.TrimSpace(item.title+item.content+item.url) == "" {
			continue
		}
		if score := contextScore(q.Query, item.title, item.content, item.url); q.Query == "" || score > 0 {
			results = append(results, repositoryContextSearchResult{
				Source:    "github",
				ID:        record.ID + ":" + item.id,
				Title:     item.title,
				Content:   item.content,
				Repo:      record.Repo,
				Branch:    record.BaseBranch,
				RunID:     record.ID,
				URL:       item.url,
				Score:     score,
				CreatedAt: &record.CreatedAt,
				UpdatedAt: &updated,
			})
		}
	}
	return results
}

func repositoryContextCode(q repositoryContextSearchQuery) ([]repositoryContextSearchResult, error) {
	repoPath, err := resolveOrchestrateRepo(q.Repo, q.Branch)
	if err != nil {
		return nil, err
	}
	var results []repositoryContextSearchResult
	err = filepath.WalkDir(repoPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil || len(results) >= q.Limit {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			switch name {
			case ".git", "node_modules", "vendor", "dist", "build", ".next":
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			info, statErr := entry.Info()
			if statErr != nil || info.Size() > 128*1024 {
				return nil
			}
		}
		rel, relErr := filepath.Rel(repoPath, path)
		if relErr != nil {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil || strings.Contains(string(data[:min(len(data), 512)]), "\x00") {
			return nil
		}
		content := string(data)
		score := contextScore(q.Query, rel, content)
		if q.Query != "" && score == 0 {
			return nil
		}
		results = append(results, repositoryContextSearchResult{
			Source:  "code",
			ID:      filepath.ToSlash(rel),
			Title:   filepath.ToSlash(rel),
			Content: shortContext(content, 1200),
			Repo:    q.Repo,
			Branch:  q.Branch,
			Score:   score,
			Metadata: map[string]interface{}{
				"path": filepath.ToSlash(rel),
			},
		})
		return nil
	})
	return results, err
}

func contextSourceAllowed(selected, source string) bool {
	selected = strings.TrimSpace(selected)
	return selected == "" || selected == "all" || selected == source || selected == source+"s"
}

func contextScore(query string, values ...string) float64 {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 1
	}
	text := strings.ToLower(strings.Join(values, " "))
	score := 0.0
	if strings.Contains(text, query) {
		score += 5
	}
	for _, token := range strings.Fields(query) {
		token = strings.Trim(token, ".,:;()[]{}")
		if len(token) >= 3 && strings.Contains(text, token) {
			score += 1
		}
	}
	return score
}

func contextResultTime(result *repositoryContextSearchResult) time.Time {
	if result == nil {
		return time.Time{}
	}
	if result.UpdatedAt != nil {
		return *result.UpdatedAt
	}
	if result.CreatedAt != nil {
		return *result.CreatedAt
	}
	return time.Time{}
}

func shortContext(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
