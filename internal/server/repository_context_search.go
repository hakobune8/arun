package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	arungh "github.com/hakobune8/arun/internal/github"
	"github.com/hakobune8/arun/internal/guideline"
	"github.com/hakobune8/arun/internal/memory"
	"github.com/hakobune8/arun/internal/safety"
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
	if contextSourceExplicit(q.Source, "github") {
		ghResults, err := repositoryContextLiveGitHub(q)
		if err != nil {
			results = append(results, repositoryContextSourceError(q, "github", err))
		} else {
			results = append(results, ghResults...)
		}
	}
	if contextSourceExplicit(q.Source, "kubernetes") {
		k8sResult := repositoryContextKubernetesLogs(ctx, q)
		results = append(results, k8sResult)
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

func repositoryContextLiveGitHub(q repositoryContextSearchQuery) ([]repositoryContextSearchResult, error) {
	parts := splitRepo(q.Repo)
	if len(parts) != 2 {
		return nil, fmt.Errorf("repo must be owner/name")
	}
	client := arungh.NewClient(parts[0], parts[1])
	redactor := safety.NewRedactor()
	var results []repositoryContextSearchResult

	issues, err := client.ListIssues("all")
	if err != nil {
		results = append(results, repositoryContextSourceError(q, "github", err))
	} else {
		for i := range issues {
			issue := issues[i]
			content := redactor.RedactString(strings.TrimSpace(issue.Body))
			title := fmt.Sprintf("#%d %s", issue.Number, issue.Title)
			if score := contextScore(q.Query, title, content, issue.State); q.Query == "" || score > 0 {
				created := issue.CreatedAt
				results = append(results, repositoryContextSearchResult{
					Source:    "github",
					ID:        fmt.Sprintf("issue:%d", issue.Number),
					Title:     title,
					Content:   shortContext(content, 2000),
					Repo:      q.Repo,
					Branch:    q.Branch,
					URL:       issue.HTMLURL,
					Score:     score,
					CreatedAt: &created,
					UpdatedAt: &created,
					Metadata: map[string]interface{}{
						"type":   "issue",
						"state":  issue.State,
						"number": issue.Number,
						"labels": issue.Labels,
					},
				})
			}
		}
	}

	prs, err := client.ListPRs("all")
	if err != nil {
		results = append(results, repositoryContextSourceError(q, "github", err))
	} else {
		for i := range prs {
			pr := prs[i]
			title := fmt.Sprintf("#%d %s", pr.Number, pr.Title)
			content := redactor.RedactString(strings.TrimSpace(pr.Body + "\n" + pr.Head + " -> " + pr.Base))
			if score := contextScore(q.Query, title, content, pr.State); q.Query == "" || score > 0 {
				results = append(results, repositoryContextSearchResult{
					Source:  "github",
					ID:      fmt.Sprintf("pull:%d", pr.Number),
					Title:   title,
					Content: shortContext(content, 2000),
					Repo:    q.Repo,
					Branch:  q.Branch,
					URL:     pr.HTMLURL,
					Score:   score,
					Metadata: map[string]interface{}{
						"type":   "pull_request",
						"state":  pr.State,
						"number": pr.Number,
						"head":   pr.Head,
						"base":   pr.Base,
					},
				})
			}
		}
	}

	checks, err := client.GetCheckRuns(q.Branch)
	if err != nil {
		results = append(results, repositoryContextSourceError(q, "github", err))
	} else {
		for i := range checks {
			check := checks[i]
			content := redactor.RedactString(strings.TrimSpace(check.Output.Title + "\n" + check.Output.Summary + "\n" + check.Output.Text))
			title := strings.TrimSpace(check.Name + " " + check.Conclusion)
			if score := contextScore(q.Query, title, content, check.Status, check.Conclusion); q.Query == "" || score > 0 {
				completed := check.CompletedAt
				results = append(results, repositoryContextSearchResult{
					Source:    "github",
					ID:        fmt.Sprintf("check:%d", check.ID),
					Title:     strings.TrimSpace(check.Name),
					Content:   shortContext(content, 2000),
					Repo:      q.Repo,
					Branch:    q.Branch,
					URL:       check.HTMLURL,
					Score:     score,
					UpdatedAt: &completed,
					Metadata: map[string]interface{}{
						"type":       "check_run",
						"status":     check.Status,
						"conclusion": check.Conclusion,
					},
				})
			}
		}
	}

	runs, err := client.ListWorkflowRuns(q.Branch)
	if err != nil {
		results = append(results, repositoryContextSourceError(q, "github", err))
	} else {
		results = append(results, repositoryContextWorkflowRunResults(q, client, redactor, runs)...)
	}

	return results, nil
}

func repositoryContextWorkflowRunResults(q repositoryContextSearchQuery, client *arungh.Client, redactor *safety.Redactor, runs []arungh.WorkflowRun) []repositoryContextSearchResult {
	var results []repositoryContextSearchResult
	for i := range runs {
		run := runs[i]
		title := strings.TrimSpace(run.DisplayTitle)
		if title == "" {
			title = run.Name
		}
		contentParts := []string{run.Status, run.Conclusion, run.HeadBranch, run.HeadSHA}
		logs, err := client.GetWorkflowRunLogs(run.ID)
		if err == nil {
			contentParts = append(contentParts, redactor.RedactString(logs))
		}
		content := strings.TrimSpace(strings.Join(contentParts, "\n"))
		if score := contextScore(q.Query, title, content); q.Query == "" || score > 0 {
			created := run.CreatedAt
			updated := run.UpdatedAt
			results = append(results, repositoryContextSearchResult{
				Source:    "github",
				ID:        fmt.Sprintf("workflow-run:%d", run.ID),
				Title:     title,
				Content:   shortContext(content, 3000),
				Repo:      q.Repo,
				Branch:    q.Branch,
				URL:       run.HTMLURL,
				Score:     score,
				CreatedAt: &created,
				UpdatedAt: &updated,
				Metadata: map[string]interface{}{
					"type":       "workflow_run",
					"status":     run.Status,
					"conclusion": run.Conclusion,
					"headSha":    run.HeadSHA,
					"logStatus":  workflowLogStatus(err),
				},
			})
		}
	}
	return results
}

func repositoryContextKubernetesLogs(ctx context.Context, q repositoryContextSearchQuery) repositoryContextSearchResult {
	now := time.Now().UTC()
	namespace := strings.TrimSpace(os.Getenv("ARUN_KUBERNETES_NAMESPACE"))
	selector := strings.TrimSpace(os.Getenv("ARUN_KUBERNETES_SELECTOR"))
	if namespace == "" || selector == "" {
		return repositoryContextSourceError(q, "kubernetes", fmt.Errorf("ARUN_KUBERNETES_NAMESPACE and ARUN_KUBERNETES_SELECTOR are required"))
	}

	kubectl := strings.TrimSpace(os.Getenv("ARUN_KUBECTL"))
	if kubectl == "" {
		kubectl = "kubectl"
	}
	args := []string{}
	if kubeconfig := strings.TrimSpace(os.Getenv("ARUN_KUBECONFIG")); kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	if contextName := strings.TrimSpace(os.Getenv("ARUN_KUBERNETES_CONTEXT")); contextName != "" {
		args = append(args, "--context", contextName)
	}
	args = append(args, "-n", namespace, "logs", "-l", selector, "--tail=200")
	if container := strings.TrimSpace(os.Getenv("ARUN_KUBERNETES_CONTAINER")); container != "" {
		args = append(args, "-c", container)
	}

	out, err := exec.CommandContext(ctx, kubectl, args...).CombinedOutput()
	if err != nil {
		return repositoryContextSourceError(q, "kubernetes", fmt.Errorf("kubectl logs failed: %s", strings.TrimSpace(string(out))))
	}
	content := safety.NewRedactor().RedactString(string(out))
	score := contextScore(q.Query, namespace, selector, content)
	return repositoryContextSearchResult{
		Source:    "kubernetes",
		ID:        "kubernetes:logs:" + namespace + ":" + selector,
		Title:     "Kubernetes logs",
		Content:   shortContext(content, 4000),
		Repo:      q.Repo,
		Branch:    q.Branch,
		Score:     score,
		CreatedAt: &now,
		UpdatedAt: &now,
		Metadata: map[string]interface{}{
			"type":      "pod_logs",
			"namespace": namespace,
			"selector":  selector,
		},
	}
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

func contextSourceExplicit(selected, source string) bool {
	selected = strings.TrimSpace(selected)
	return selected == source || selected == source+"s"
}

func repositoryContextSourceError(q repositoryContextSearchQuery, source string, err error) repositoryContextSearchResult {
	now := time.Now().UTC()
	return repositoryContextSearchResult{
		Source:    source,
		ID:        source + ":error",
		Title:     source + " source unavailable",
		Content:   safety.NewRedactor().RedactString(err.Error()),
		Repo:      q.Repo,
		Branch:    q.Branch,
		Score:     0.1,
		CreatedAt: &now,
		UpdatedAt: &now,
		Metadata: map[string]interface{}{
			"type":  "source_error",
			"error": true,
		},
	}
}

func workflowLogStatus(err error) string {
	if err == nil {
		return "included"
	}
	return "unavailable"
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
