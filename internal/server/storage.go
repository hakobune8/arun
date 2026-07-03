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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hakobune8/arun/internal/apphome"
	"github.com/hakobune8/arun/internal/guideline"
	"github.com/hakobune8/arun/internal/memory"
	"github.com/hakobune8/arun/internal/safety"
)

const (
	defaultOrchestrationRetention = 30 * 24 * time.Hour
	defaultRunArtifactRetention   = 14 * 24 * time.Hour
	defaultWorkspaceRetention     = 7 * 24 * time.Hour
	defaultContextRetention       = 180 * 24 * time.Hour
	defaultKeepLastOrchestrations = 100
)

type storagePolicy struct {
	Repo                     string `json:"repo,omitempty"`
	BaseBranch               string `json:"baseBranch,omitempty"`
	OrchestrationRetention   string `json:"orchestrationRetention"`
	RunArtifactRetention     string `json:"runArtifactRetention"`
	WorkspaceRetention       string `json:"workspaceRetention"`
	MemoryRetention          string `json:"memoryRetention"`
	GuidelineRetention       string `json:"guidelineRetention"`
	KeepLastOrchestrations   int    `json:"keepLastOrchestrations"`
	ArchiveBeforeDelete      bool   `json:"archiveBeforeDelete"`
	AllowLinkedGitHubCleanup bool   `json:"allowLinkedGitHubCleanup"`
}

type storageUsage struct {
	HomeBytes           int64 `json:"homeBytes"`
	OrchestrationBytes  int64 `json:"orchestrationBytes"`
	RunArtifactBytes    int64 `json:"runArtifactBytes"`
	WorkspaceBytes      int64 `json:"workspaceBytes"`
	ArchiveBytes        int64 `json:"archiveBytes"`
	AuditBytes          int64 `json:"auditBytes"`
	NotificationBytes   int64 `json:"notificationBytes"`
	MemoryBytes         int64 `json:"memoryBytes"`
	GuidelineBytes      int64 `json:"guidelineBytes"`
	OrchestrationCount  int   `json:"orchestrationCount"`
	RunArtifactCount    int   `json:"runArtifactCount"`
	WorkspaceCount      int   `json:"workspaceCount"`
	MemoryCount         int   `json:"memoryCount"`
	GuidelineCount      int   `json:"guidelineCount"`
	ArchivedMemoryCount int   `json:"archivedMemoryCount"`
	ArchivedGuideCount  int   `json:"archivedGuidelineCount"`
}

type storageResponse struct {
	Policy storagePolicy `json:"policy"`
	Usage  storageUsage  `json:"usage"`
}

type storageCleanupRequest struct {
	DryRun bool           `json:"dryRun"`
	Policy *storagePolicy `json:"policy,omitempty"`
}

type storageCleanupResult struct {
	DryRun  bool                  `json:"dryRun"`
	Policy  storagePolicy         `json:"policy"`
	Items   []storageCleanupItem  `json:"items"`
	Summary storageCleanupSummary `json:"summary"`
}

type storageCleanupSummary struct {
	Selected int   `json:"selected"`
	Archived int   `json:"archived"`
	Deleted  int   `json:"deleted"`
	Skipped  int   `json:"skipped"`
	Bytes    int64 `json:"bytes"`
}

type storageCleanupItem struct {
	Type    string `json:"type"`
	ID      string `json:"id,omitempty"`
	Path    string `json:"path,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Action  string `json:"action"`
	Reason  string `json:"reason,omitempty"`
	Bytes   int64  `json:"bytes,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
}

func defaultStoragePolicy() storagePolicy {
	return storagePolicy{
		OrchestrationRetention: defaultOrchestrationRetention.String(),
		RunArtifactRetention:   defaultRunArtifactRetention.String(),
		WorkspaceRetention:     defaultWorkspaceRetention.String(),
		MemoryRetention:        defaultContextRetention.String(),
		GuidelineRetention:     defaultContextRetention.String(),
		KeepLastOrchestrations: defaultKeepLastOrchestrations,
		ArchiveBeforeDelete:    true,
	}
}

func storagePolicyPath() string {
	return filepath.Join(apphome.Dir(), "storage", "policy.json")
}

func readStoragePolicy() (storagePolicy, error) {
	policy := defaultStoragePolicy()
	data, err := os.ReadFile(storagePolicyPath())
	if err != nil {
		if os.IsNotExist(err) {
			return policy, nil
		}
		return storagePolicy{}, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return policy, nil
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		return storagePolicy{}, err
	}
	return normalizeStoragePolicy(&policy)
}

func saveStoragePolicy(policy *storagePolicy) error {
	normalized, err := normalizeStoragePolicy(policy)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(storagePolicyPath()), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storagePolicyPath(), data, 0o600)
}

func normalizeStoragePolicy(policy *storagePolicy) (storagePolicy, error) {
	normalized := *policy
	defaults := defaultStoragePolicy()
	normalized.Repo = strings.TrimSpace(normalized.Repo)
	normalized.BaseBranch = defaultBaseBranch(normalized.BaseBranch)
	normalized.OrchestrationRetention = storageFirstNonEmpty(strings.TrimSpace(normalized.OrchestrationRetention), defaults.OrchestrationRetention)
	normalized.RunArtifactRetention = storageFirstNonEmpty(strings.TrimSpace(normalized.RunArtifactRetention), defaults.RunArtifactRetention)
	normalized.WorkspaceRetention = storageFirstNonEmpty(strings.TrimSpace(normalized.WorkspaceRetention), defaults.WorkspaceRetention)
	normalized.MemoryRetention = storageFirstNonEmpty(strings.TrimSpace(normalized.MemoryRetention), defaults.MemoryRetention)
	normalized.GuidelineRetention = storageFirstNonEmpty(strings.TrimSpace(normalized.GuidelineRetention), defaults.GuidelineRetention)
	if normalized.KeepLastOrchestrations == 0 {
		normalized.KeepLastOrchestrations = defaults.KeepLastOrchestrations
	}
	if normalized.KeepLastOrchestrations < 0 {
		return storagePolicy{}, fmt.Errorf("keepLastOrchestrations must be non-negative")
	}
	for name, value := range map[string]string{
		"orchestrationRetention": normalized.OrchestrationRetention,
		"runArtifactRetention":   normalized.RunArtifactRetention,
		"workspaceRetention":     normalized.WorkspaceRetention,
		"memoryRetention":        normalized.MemoryRetention,
		"guidelineRetention":     normalized.GuidelineRetention,
	} {
		if _, err := parseRetentionDuration(value); err != nil {
			return storagePolicy{}, fmt.Errorf("%s: %w", name, err)
		}
	}
	return normalized, nil
}

func storageFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func parseRetentionDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if value == "0" || value == "0s" {
		return 0, nil
	}
	if strings.HasSuffix(value, "d") {
		days := strings.TrimSuffix(value, "d")
		parsed, err := time.ParseDuration(days + "h")
		if err != nil {
			return 0, fmt.Errorf("must be a duration such as 30d or 720h")
		}
		return parsed * 24, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration < 0 {
		return 0, fmt.Errorf("must be a non-negative duration")
	}
	return duration, nil
}

func (s *Server) handleStorage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.requireAutomationPermission(w, r, user, "storage.read", "storage", "", "") {
			return
		}
		policy, err := readStoragePolicy()
		if err != nil {
			http.Error(w, "read storage policy: "+err.Error(), http.StatusInternalServerError)
			return
		}
		usage, err := collectStorageUsage(context.Background())
		if err != nil {
			http.Error(w, "collect storage usage: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(storageResponse{Policy: policy, Usage: usage}) //nolint:errcheck // best-effort response
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStorageDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/storage/"), "/")
	switch action {
	case "policy":
		s.handleStoragePolicy(w, r, user)
	case "cleanup":
		s.handleStorageCleanup(w, r, user)
	default:
		http.Error(w, "unknown storage action", http.StatusNotFound)
	}
}

func (s *Server) handleStoragePolicy(w http.ResponseWriter, r *http.Request, user *authUser) {
	if !s.requireAutomationPermission(w, r, user, "storage.manage", "storage", "", "") {
		return
	}
	switch r.Method {
	case http.MethodGet:
		policy, err := readStoragePolicy()
		if err != nil {
			http.Error(w, "read storage policy: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(policy) //nolint:errcheck // best-effort response
	case http.MethodPut:
		var policy storagePolicy
		if err := json.NewDecoder(r.Body).Decode(&policy); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveStoragePolicy(&policy); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		normalized, _ := readStoragePolicy()
		_ = appendAuditEvent(&auditEvent{Actor: actorLogin(user), Action: "storage.policy.update", Target: "storage/policy", Outcome: auditOutcomeSuccess}) //nolint:errcheck // best-effort audit
		_ = json.NewEncoder(w).Encode(normalized)                                                                                                           //nolint:errcheck // best-effort response
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleStorageCleanup(w http.ResponseWriter, r *http.Request, user *authUser) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "storage.cleanup", "storage", "", "") {
		return
	}
	var req storageCleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "parse body: "+err.Error(), http.StatusBadRequest)
		return
	}
	result, err := runStorageCleanup(context.Background(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	outcome := auditOutcomeSuccess
	if req.DryRun {
		outcome = auditOutcomeAllowed
	}
	_ = appendAuditEvent(&auditEvent{Actor: actorLogin(user), Action: "storage.cleanup", Target: "storage", Outcome: outcome, Message: fmt.Sprintf("selected=%d archived=%d deleted=%d skipped=%d", result.Summary.Selected, result.Summary.Archived, result.Summary.Deleted, result.Summary.Skipped)}) //nolint:errcheck // best-effort audit
	_ = json.NewEncoder(w).Encode(result)                                                                                                                                                                                                                                                               //nolint:errcheck // best-effort response
}

func (s *Server) runAutomaticStorageCleanup(reason string) {
	policy, err := readStoragePolicy()
	if err != nil {
		slog.Warn("read storage policy failed", "error", err)
		return
	}
	result, err := runStorageCleanup(context.Background(), storageCleanupRequest{DryRun: false, Policy: &policy})
	if err != nil {
		slog.Warn("automatic storage cleanup failed", "reason", reason, "error", err)
		return
	}
	if result.Summary.Selected > 0 {
		_ = appendAuditEvent(&auditEvent{Actor: "system", Action: "storage.cleanup.automatic", Target: "storage", Outcome: auditOutcomeSuccess, Message: fmt.Sprintf("reason=%s selected=%d archived=%d deleted=%d skipped=%d", reason, result.Summary.Selected, result.Summary.Archived, result.Summary.Deleted, result.Summary.Skipped)}) //nolint:errcheck // best-effort audit
	}
}

func runStorageCleanup(ctx context.Context, req storageCleanupRequest) (*storageCleanupResult, error) {
	policy, err := readStoragePolicy()
	if err != nil {
		return nil, err
	}
	if req.Policy != nil {
		policy, err = normalizeStoragePolicy(req.Policy)
		if err != nil {
			return nil, err
		}
	}
	items, err := planStorageCleanup(ctx, &policy)
	if err != nil {
		return nil, err
	}
	result := &storageCleanupResult{DryRun: req.DryRun, Policy: policy, Items: items}
	for i := range result.Items {
		item := &result.Items[i]
		if item.Skipped {
			result.Summary.Skipped++
			continue
		}
		result.Summary.Selected++
		result.Summary.Bytes += item.Bytes
		if req.DryRun {
			continue
		}
		if err := applyStorageCleanupItem(ctx, &policy, item); err != nil {
			item.Skipped = true
			item.Reason = err.Error()
			result.Summary.Skipped++
			result.Summary.Selected--
			continue
		}
		switch item.Action {
		case "archive", "archive_status":
			result.Summary.Archived++
		case "delete":
			result.Summary.Deleted++
		}
	}
	result.Items = redactCleanupItems(result.Items)
	return result, nil
}

func planStorageCleanup(ctx context.Context, policy *storagePolicy) ([]storageCleanupItem, error) {
	now := time.Now().UTC()
	var items []storageCleanupItem
	orchestrationRetention, _ := parseRetentionDuration(policy.OrchestrationRetention)
	if orchestrationRetention > 0 {
		records, err := listOrchestrationRecords()
		if err != nil {
			return nil, err
		}
		sort.Slice(records, func(i, j int) bool {
			return records[i].UpdatedAt.After(records[j].UpdatedAt)
		})
		kept := 0
		for _, record := range records {
			if record == nil || !storagePolicyMatchesRecord(policy, record) {
				continue
			}
			kept++
			if kept <= policy.KeepLastOrchestrations {
				continue
			}
			updated := record.UpdatedAt
			if updated.IsZero() {
				updated = record.CreatedAt
			}
			if updated.IsZero() || now.Sub(updated) < orchestrationRetention {
				continue
			}
			item := storageCleanupItem{
				Type:   "orchestration",
				ID:     record.ID,
				Repo:   record.Repo,
				Branch: record.BaseBranch,
				Path:   filepath.Join(orchestrationsDir(), record.ID+".json"),
				Action: cleanupArchiveAction(policy),
			}
			item.Bytes = pathSize(item.Path)
			if orchestrationInProgress(record.Status) {
				item.Skipped = true
				item.Reason = "orchestration is still active"
			} else if orchestrationHasGitHubLinks(record) && !policy.AllowLinkedGitHubCleanup {
				item.Skipped = true
				item.Reason = "orchestration has GitHub-linked artifacts"
			}
			items = append(items, item)
		}
	}
	items = append(items, planRunArtifactCleanup(policy, now)...)
	items = append(items, planWorkspaceCleanup(policy, now)...)
	memoryItems, err := planMemoryCleanup(ctx, policy, now)
	if err != nil {
		return nil, err
	}
	items = append(items, memoryItems...)
	guidelineItems, err := planGuidelineCleanup(ctx, policy, now)
	if err != nil {
		return nil, err
	}
	items = append(items, guidelineItems...)
	return items, nil
}

func cleanupArchiveAction(policy *storagePolicy) string {
	if policy.ArchiveBeforeDelete {
		return "archive"
	}
	return "delete"
}

func planRunArtifactCleanup(policy *storagePolicy, now time.Time) []storageCleanupItem {
	retention, _ := parseRetentionDuration(policy.RunArtifactRetention)
	if retention <= 0 {
		return nil
	}
	var items []storageCleanupItem
	entries, err := os.ReadDir(apphome.RunsDir())
	if err != nil {
		return nil
	}
	active := activeOrchestrationIDs()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(apphome.RunsDir(), entry.Name())
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime().UTC()) < retention {
			continue
		}
		item := storageCleanupItem{Type: "run_artifacts", ID: entry.Name(), Path: path, Action: cleanupArchiveAction(policy), Bytes: pathSize(path)}
		for id := range active {
			if strings.HasPrefix(entry.Name(), id) {
				item.Skipped = true
				item.Reason = "run artifacts belong to active orchestration"
				break
			}
		}
		items = append(items, item)
	}
	return items
}

func planWorkspaceCleanup(policy *storagePolicy, now time.Time) []storageCleanupItem {
	retention, _ := parseRetentionDuration(policy.WorkspaceRetention)
	if retention <= 0 {
		return nil
	}
	root := filepath.Join(apphome.Dir(), "workspaces", "orchestrate")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	activePaths := activeWorkspacePaths()
	var items []storageCleanupItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime().UTC()) < retention {
			continue
		}
		item := storageCleanupItem{Type: "workspace", ID: entry.Name(), Path: path, Action: "delete", Bytes: pathSize(path)}
		if activePaths[path] {
			item.Skipped = true
			item.Reason = "workspace belongs to active orchestration"
		}
		items = append(items, item)
	}
	return items
}

func planMemoryCleanup(ctx context.Context, policy *storagePolicy, now time.Time) ([]storageCleanupItem, error) {
	retention, _ := parseRetentionDuration(policy.MemoryRetention)
	if retention <= 0 {
		return nil, nil
	}
	store, err := repositoryMemoryStore()
	if err != nil {
		return nil, err
	}
	entries, err := store.List(ctx, &memory.RepositoryQuery{Repo: policy.Repo, Branch: policy.BaseBranch})
	if err != nil {
		return nil, err
	}
	var items []storageCleanupItem
	for i := range entries {
		entry := &entries[i]
		if entry.Status == memory.RepositoryMemoryArchived || entry.Pinned || now.Sub(entry.UpdatedAt) < retention {
			continue
		}
		items = append(items, storageCleanupItem{
			Type:   "memory",
			ID:     entry.ID,
			Repo:   entry.Repo,
			Branch: entry.Branch,
			Action: "archive_status",
			Reason: "older than memory retention",
		})
	}
	return items, nil
}

func planGuidelineCleanup(ctx context.Context, policy *storagePolicy, now time.Time) ([]storageCleanupItem, error) {
	retention, _ := parseRetentionDuration(policy.GuidelineRetention)
	if retention <= 0 {
		return nil, nil
	}
	store, err := repositoryGuidelineStore()
	if err != nil {
		return nil, err
	}
	entries, err := store.List(ctx, &guideline.RepositoryGuidelineQuery{Repo: policy.Repo, Branch: policy.BaseBranch, Status: guideline.RepositoryGuidelineActive})
	if err != nil {
		return nil, err
	}
	var items []storageCleanupItem
	for i := range entries {
		entry := &entries[i]
		if entry.Required || now.Sub(entry.UpdatedAt) < retention {
			continue
		}
		items = append(items, storageCleanupItem{
			Type:   "guideline",
			ID:     entry.ID,
			Repo:   entry.Repo,
			Branch: entry.Branch,
			Action: "archive_status",
			Reason: "older than guideline retention",
		})
	}
	return items, nil
}

func applyStorageCleanupItem(ctx context.Context, policy *storagePolicy, item *storageCleanupItem) error {
	switch item.Type {
	case "orchestration":
		return archiveOrDeletePath(item.Path, archivePath("orchestrates", filepath.Base(item.Path)), policy.ArchiveBeforeDelete)
	case "run_artifacts":
		return archiveOrDeletePath(item.Path, archivePath("runs", filepath.Base(item.Path)), policy.ArchiveBeforeDelete)
	case "workspace":
		return os.RemoveAll(item.Path)
	case "memory":
		store, err := repositoryMemoryStore()
		if err != nil {
			return err
		}
		entry, err := store.Get(ctx, item.ID)
		if err != nil {
			return err
		}
		entry.Status = memory.RepositoryMemoryArchived
		entry.Pinned = false
		return store.Save(ctx, entry)
	case "guideline":
		store, err := repositoryGuidelineStore()
		if err != nil {
			return err
		}
		_, err = store.Archive(ctx, item.ID)
		return err
	default:
		return fmt.Errorf("unsupported cleanup item type %q", item.Type)
	}
}

func archiveOrDeletePath(src, dst string, archive bool) error {
	if !archive {
		return os.RemoveAll(src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyPath(src, dst); err != nil {
		return err
	}
	return os.RemoveAll(src)
}

func archivePath(kind, name string) string {
	return filepath.Join(apphome.Dir(), "archive", kind, time.Now().UTC().Format("20060102T150405")+"-"+name)
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst)
	}
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func collectStorageUsage(ctx context.Context) (storageUsage, error) {
	usage := storageUsage{
		HomeBytes:          pathSize(apphome.Dir()),
		OrchestrationBytes: pathSize(orchestrationsDir()),
		RunArtifactBytes:   pathSize(apphome.RunsDir()),
		WorkspaceBytes:     pathSize(filepath.Join(apphome.Dir(), "workspaces", "orchestrate")),
		ArchiveBytes:       pathSize(filepath.Join(apphome.Dir(), "archive")),
		AuditBytes:         pathSize(filepath.Join(apphome.Dir(), "audit")),
		NotificationBytes:  pathSize(filepath.Join(apphome.Dir(), "notifications")),
		MemoryBytes:        pathSize(repositoryMemoryPath()),
		GuidelineBytes:     pathSize(repositoryGuidelinesPath()),
	}
	if records, err := listOrchestrationRecords(); err == nil {
		usage.OrchestrationCount = len(records)
	}
	usage.RunArtifactCount = dirEntryCount(apphome.RunsDir())
	usage.WorkspaceCount = dirEntryCount(filepath.Join(apphome.Dir(), "workspaces", "orchestrate"))
	if store, err := repositoryMemoryStore(); err == nil {
		if entries, err := store.List(ctx, &memory.RepositoryQuery{}); err == nil {
			usage.MemoryCount = len(entries)
			for i := range entries {
				entry := &entries[i]
				if entry.Status == memory.RepositoryMemoryArchived {
					usage.ArchivedMemoryCount++
				}
			}
		}
	}
	if store, err := repositoryGuidelineStore(); err == nil {
		if entries, err := store.List(ctx, &guideline.RepositoryGuidelineQuery{Status: guideline.RepositoryGuidelineActive}); err == nil {
			usage.GuidelineCount = len(entries)
		}
		if entries, err := store.List(ctx, &guideline.RepositoryGuidelineQuery{Status: guideline.RepositoryGuidelineArchived}); err == nil {
			usage.GuidelineCount += len(entries)
			usage.ArchivedGuideCount = len(entries)
		}
	}
	return usage, nil
}

func pathSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

func dirEntryCount(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count++
		}
	}
	return count
}

func storagePolicyMatchesRecord(policy *storagePolicy, record *orchestrationRecord) bool {
	if policy.Repo != "" && memory.NormalizeRepository(record.Repo) != memory.NormalizeRepository(policy.Repo) {
		return false
	}
	if policy.BaseBranch != "" && defaultBaseBranch(record.BaseBranch) != defaultBaseBranch(policy.BaseBranch) {
		return false
	}
	return true
}

func orchestrationHasGitHubLinks(record *orchestrationRecord) bool {
	if record == nil || record.GitHub == nil {
		return false
	}
	return record.GitHub.IssueURL != "" || record.GitHub.PullRequestURL != "" || record.GitHub.SourceIssueURL != ""
}

func activeOrchestrationIDs() map[string]bool {
	ids := map[string]bool{}
	records, err := listOrchestrationRecords()
	if err != nil {
		return ids
	}
	for _, record := range records {
		if record != nil && orchestrationInProgress(record.Status) {
			ids[record.ID] = true
		}
	}
	return ids
}

func activeWorkspacePaths() map[string]bool {
	paths := map[string]bool{}
	records, err := listOrchestrationRecords()
	if err != nil {
		return paths
	}
	for _, record := range records {
		if record != nil && orchestrationInProgress(record.Status) && record.RepoPath != "" {
			paths[record.RepoPath] = true
		}
	}
	return paths
}

func redactCleanupItems(items []storageCleanupItem) []storageCleanupItem {
	redactor := safety.NewRedactor()
	out := make([]storageCleanupItem, len(items))
	for i := range items {
		out[i] = items[i]
		out[i].Path = redactor.RedactString(out[i].Path)
		out[i].Reason = redactor.RedactString(out[i].Reason)
	}
	return out
}
