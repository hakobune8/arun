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
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/orchestrator"
)

const (
	defaultMaxRunDuration       = 30 * time.Minute
	defaultMaxSubtasks          = 12
	defaultMaxConcurrentRepoRun = 1
)

type governanceLimits struct {
	MaxDuration          string `json:"maxDuration,omitempty"`
	MaxSubtasks          int    `json:"maxSubtasks,omitempty"`
	MaxRetries           int    `json:"maxRetries,omitempty"`
	MaxLLMTokens         int    `json:"maxLlmTokens,omitempty"`
	MaxGitHubRequests    int    `json:"maxGitHubRequests,omitempty"`
	MaxConcurrentRepoRun int    `json:"maxConcurrentRepoRuns,omitempty"`
	MaxConcurrentOrgRun  int    `json:"maxConcurrentOrgRuns,omitempty"`
}

type governanceUsage struct {
	StartedAt            time.Time `json:"startedAt,omitempty"`
	FinishedAt           time.Time `json:"finishedAt,omitempty"`
	Duration             string    `json:"duration,omitempty"`
	SubtasksPlanned      int       `json:"subtasksPlanned,omitempty"`
	SubtasksCompleted    int       `json:"subtasksCompleted,omitempty"`
	FailedSubtasks       int       `json:"failedSubtasks,omitempty"`
	LLMTokensBudget      int       `json:"llmTokensBudget,omitempty"`
	LLMTokensUsed        int       `json:"llmTokensUsed,omitempty"`
	GitHubRequestsBudget int       `json:"gitHubRequestsBudget,omitempty"`
	GitHubRequestsUsed   int       `json:"gitHubRequestsUsed,omitempty"`
	BudgetStatus         string    `json:"budgetStatus,omitempty"`
	LimitExceeded        string    `json:"limitExceeded,omitempty"`
}

func normalizeGovernanceLimits(limits governanceLimits) (governanceLimits, error) {
	limits.MaxDuration = strings.TrimSpace(limits.MaxDuration)
	if limits.MaxDuration == "" {
		limits.MaxDuration = defaultMaxRunDuration.String()
	}
	if _, err := parseGovernanceDuration(limits.MaxDuration); err != nil {
		return governanceLimits{}, fmt.Errorf("limits.maxDuration: %w", err)
	}
	if limits.MaxSubtasks == 0 {
		limits.MaxSubtasks = defaultMaxSubtasks
	}
	if limits.MaxConcurrentRepoRun == 0 {
		limits.MaxConcurrentRepoRun = defaultMaxConcurrentRepoRun
	}
	if limits.MaxSubtasks < 0 || limits.MaxRetries < 0 || limits.MaxLLMTokens < 0 || limits.MaxGitHubRequests < 0 || limits.MaxConcurrentRepoRun < 0 || limits.MaxConcurrentOrgRun < 0 {
		return governanceLimits{}, fmt.Errorf("limits values must be non-negative")
	}
	return limits, nil
}

func parseGovernanceDuration(raw string) (time.Duration, error) {
	duration, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("must be a positive duration")
	}
	return duration, nil
}

func (limits governanceLimits) maxDuration() time.Duration {
	duration, err := parseGovernanceDuration(limits.MaxDuration)
	if err != nil {
		return defaultMaxRunDuration
	}
	return duration
}

func (s *Server) enforceGovernanceBeforeStart(req *orchestrateRequest, limits governanceLimits) error {
	if limits.MaxConcurrentRepoRun > 0 {
		active := s.activeOrchestrationsForRepo(req.Repo)
		if active >= limits.MaxConcurrentRepoRun {
			return fmt.Errorf("repo concurrency limit exceeded: %d active run(s), limit %d", active, limits.MaxConcurrentRepoRun)
		}
	}
	if limits.MaxConcurrentOrgRun > 0 {
		org := organizationFromRepo(req.Repo)
		if org != "" {
			active := s.activeOrchestrationsForOrg(org)
			if active >= limits.MaxConcurrentOrgRun {
				return fmt.Errorf("org concurrency limit exceeded: %d active run(s), limit %d", active, limits.MaxConcurrentOrgRun)
			}
		}
	}
	return nil
}

func enforceGovernancePlan(record *orchestrationRecord, plan *orchestrator.TaskPlan) error {
	if record == nil || plan == nil || record.Limits.MaxSubtasks <= 0 {
		return nil
	}
	if len(plan.Subtasks) > record.Limits.MaxSubtasks {
		return fmt.Errorf("subtask limit exceeded: planned %d, limit %d", len(plan.Subtasks), record.Limits.MaxSubtasks)
	}
	return nil
}

func initializeGovernanceUsage(record *orchestrationRecord) {
	if record == nil {
		return
	}
	record.Usage.StartedAt = record.CreatedAt
	record.Usage.BudgetStatus = "within_limits"
	record.Usage.LLMTokensBudget = record.Limits.MaxLLMTokens
	record.Usage.GitHubRequestsBudget = record.Limits.MaxGitHubRequests
}

func updateGovernanceUsage(record *orchestrationRecord) {
	if record == nil {
		return
	}
	if record.Plan != nil {
		record.Usage.SubtasksPlanned = len(record.Plan.Subtasks)
	}
	completed := 0
	failed := 0
	for _, result := range record.Results {
		if result.SubtaskID == "" {
			continue
		}
		completed++
		if !result.Success {
			failed++
		}
	}
	record.Usage.SubtasksCompleted = completed
	record.Usage.FailedSubtasks = failed
	if !record.CreatedAt.IsZero() {
		end := record.UpdatedAt
		if end.IsZero() {
			end = time.Now().UTC()
		}
		record.Usage.Duration = end.Sub(record.CreatedAt).Round(time.Second).String()
	}
	if isTerminalOrchestrationStatus(record.Status) {
		record.Usage.FinishedAt = record.UpdatedAt
	}
	if record.Usage.BudgetStatus == "" {
		record.Usage.BudgetStatus = "within_limits"
	}
}

func markGovernanceLimitExceeded(record *orchestrationRecord, message string) {
	if record == nil {
		return
	}
	record.Usage.BudgetStatus = "exceeded"
	record.Usage.LimitExceeded = message
	updateGovernanceUsage(record)
}

func (s *Server) activeOrchestrationsForRepo(repo string) int {
	repo = normalizeGovernanceRepo(repo)
	count := 0
	seen := map[string]bool{}
	for _, record := range activeOrchestrationRecords() {
		if normalizeGovernanceRepo(record.Repo) == repo {
			count++
			seen[record.ID] = true
		}
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for id := range s.activeRuns {
		if seen[id] {
			continue
		}
		record, err := readOrchestrationRecord(id)
		if err == nil && normalizeGovernanceRepo(record.Repo) == repo {
			count++
		}
	}
	return count
}

func (s *Server) activeOrchestrationsForOrg(org string) int {
	org = strings.ToLower(strings.TrimSpace(org))
	if org == "" {
		return 0
	}
	count := 0
	seen := map[string]bool{}
	for _, record := range activeOrchestrationRecords() {
		if strings.EqualFold(organizationFromRepo(record.Repo), org) {
			count++
			seen[record.ID] = true
		}
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	for id := range s.activeRuns {
		if seen[id] {
			continue
		}
		record, err := readOrchestrationRecord(id)
		if err == nil && strings.EqualFold(organizationFromRepo(record.Repo), org) {
			count++
		}
	}
	return count
}

func activeOrchestrationRecords() []*orchestrationRecord {
	records, err := listOrchestrationRecords()
	if err != nil {
		return nil
	}
	active := make([]*orchestrationRecord, 0, len(records))
	for _, record := range records {
		if record != nil && orchestrationInProgress(record.Status) {
			active = append(active, record)
		}
	}
	return active
}

func normalizeGovernanceRepo(repo string) string {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "."
	}
	if parsed, err := url.Parse(repo); err == nil && parsed.Host == "github.com" {
		path := strings.Trim(parsed.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		if path != "" {
			return strings.ToLower(path)
		}
	}
	return strings.ToLower(strings.TrimSuffix(repo, ".git"))
}

func organizationFromRepo(repo string) string {
	repo = normalizeGovernanceRepo(repo)
	if repo == "." || !strings.Contains(repo, "/") {
		return ""
	}
	org, _, _ := strings.Cut(repo, "/")
	return org
}

func isTerminalOrchestrationStatus(status string) bool {
	switch status {
	case "completed", "failed", "canceled", "pending_approval", "approval_rejected":
		return true
	default:
		return false
	}
}
