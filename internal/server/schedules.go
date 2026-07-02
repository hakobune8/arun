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
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/agent"
	"github.com/kazyamaz200/agentos/internal/apphome"
	"github.com/kazyamaz200/agentos/internal/safety"
)

var scheduleIDPattern = regexp.MustCompile(`^schedule-[0-9a-f]{16}$`)

const (
	scheduleStatusActive = "active"
	scheduleStatusPaused = "paused"

	schedulePolicyForbid = "forbid"
	schedulePolicyAllow  = "allow"
)

type scheduleDefinition struct {
	ID                string                     `json:"id"`
	Actor             string                     `json:"actor,omitempty"`
	TemplateID        string                     `json:"templateId,omitempty"`
	Name              string                     `json:"name"`
	Status            string                     `json:"status"`
	Repo              string                     `json:"repo"`
	BaseBranch        string                     `json:"baseBranch"`
	Task              string                     `json:"task"`
	Agents            []string                   `json:"agents"`
	CustomAgents      []agent.Definition         `json:"customAgents,omitempty"`
	Scenario          *scenarioTemplateSelection `json:"scenarioTemplate,omitempty"`
	Strategy          string                     `json:"strategy"`
	LLMPreset         string                     `json:"llmPreset,omitempty"`
	OutputLanguage    string                     `json:"outputLanguage,omitempty"`
	GitHub            *orchestrateGitHubRequest  `json:"github,omitempty"`
	Limits            governanceLimits           `json:"limits,omitempty"`
	Schedule          scheduleSpec               `json:"schedule"`
	ConcurrencyPolicy string                     `json:"concurrencyPolicy"`
	RetryPolicy       scheduleRetryPolicy        `json:"retryPolicy,omitempty"`
	Notification      scheduleNotification       `json:"notification,omitempty"`
	NextRunAt         time.Time                  `json:"nextRunAt,omitempty"`
	LastRunAt         time.Time                  `json:"lastRunAt,omitempty"`
	LastRunID         string                     `json:"lastRunId,omitempty"`
	LastRunStatus     string                     `json:"lastRunStatus,omitempty"`
	Executions        []scheduleExecution        `json:"executions,omitempty"`
	CreatedAt         time.Time                  `json:"createdAt"`
	UpdatedAt         time.Time                  `json:"updatedAt"`
}

type scheduleSpec struct {
	Type     string `json:"type"`
	Cron     string `json:"cron,omitempty"`
	Interval string `json:"interval,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

type scheduleRetryPolicy struct {
	MaxRetries    int    `json:"maxRetries,omitempty"`
	RetryInterval string `json:"retryInterval,omitempty"`
}

type scheduleNotification struct {
	Enabled      bool     `json:"enabled,omitempty"`
	Triggers     []string `json:"triggers,omitempty"`
	Destinations []string `json:"destinations,omitempty"`
	WebhookURL   string   `json:"webhookUrl,omitempty"`
}

type scheduleExecution struct {
	ID        string    `json:"id"`
	RunID     string    `json:"runId,omitempty"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	Message   string    `json:"message,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

type scheduledWorkflowTemplate struct {
	ID                  string                   `json:"id"`
	Name                string                   `json:"name"`
	Description         string                   `json:"description"`
	Category            string                   `json:"category"`
	Task                string                   `json:"task"`
	Agents              []string                 `json:"agents"`
	Strategy            string                   `json:"strategy"`
	Schedule            scheduleSpec             `json:"schedule"`
	ConcurrencyPolicy   string                   `json:"concurrencyPolicy"`
	OutputLanguage      string                   `json:"outputLanguage,omitempty"`
	GitHub              orchestrateGitHubRequest `json:"github"`
	ExpectedOutputs     []string                 `json:"expectedOutputs"`
	RequiredPermissions []string                 `json:"requiredPermissions"`
	Notes               []string                 `json:"notes,omitempty"`
}

func (s *Server) handleSchedules(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.requireAutomationPermission(w, r, user, "schedules.read", "schedules", "", "") {
			return
		}
		schedules, err := listSchedules()
		if err != nil {
			http.Error(w, "list schedules: "+err.Error(), http.StatusInternalServerError)
			return
		}
		refreshScheduleRunStatuses(schedules)
		_ = json.NewEncoder(w).Encode(schedules) //nolint:errcheck // best-effort response
	case http.MethodPost:
		if !s.requireAutomationPermission(w, r, user, "schedules.create", "schedules", "", "") {
			return
		}
		schedule, err := decodeSchedule(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		now := time.Now().UTC()
		schedule.ID = "schedule-" + strings.TrimPrefix(generateID(), "run-")
		schedule.Actor = actorLogin(user)
		schedule.Status = scheduleStatusActive
		schedule.CreatedAt = now
		schedule.UpdatedAt = now
		if err := prepareSchedule(schedule, now); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveSchedule(schedule); err != nil {
			http.Error(w, "save schedule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = appendAuditEvent(&auditEvent{Actor: actorLogin(user), Action: "schedules.create", Target: "schedule/" + schedule.ID, Repo: schedule.Repo, Outcome: auditOutcomeSuccess}) //nolint:errcheck // best-effort audit
		_ = json.NewEncoder(w).Encode(schedule)                                                                                                                                      //nolint:errcheck // best-effort response
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleScheduleDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/schedules/")
	if strings.Trim(path, "/") == "templates" {
		s.handleScheduleTemplates(w, r, user)
		return
	}
	id, action, _ := strings.Cut(strings.Trim(path, "/"), "/")
	if !isValidScheduleID(id) {
		http.Error(w, "invalid schedule id", http.StatusBadRequest)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "schedules.manage", "schedule/"+id, "", "") {
		return
	}
	schedule, err := readSchedule(id)
	if err != nil {
		http.Error(w, "schedule not found: "+id, http.StatusNotFound)
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		refreshScheduleRunStatus(schedule)
		_ = json.NewEncoder(w).Encode(schedule) //nolint:errcheck // best-effort response
	case action == "" && r.Method == http.MethodPut:
		updated, err := decodeSchedule(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated.ID = schedule.ID
		updated.Actor = schedule.Actor
		updated.Status = schedule.Status
		updated.CreatedAt = schedule.CreatedAt
		updated.Executions = schedule.Executions
		updated.LastRunAt = schedule.LastRunAt
		updated.LastRunID = schedule.LastRunID
		updated.LastRunStatus = schedule.LastRunStatus
		updated.UpdatedAt = time.Now().UTC()
		if err := prepareSchedule(updated, updated.UpdatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := saveSchedule(updated); err != nil {
			http.Error(w, "save schedule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(updated) //nolint:errcheck // best-effort response
	case action == "" && r.Method == http.MethodDelete:
		if err := deleteSchedule(id); err != nil {
			http.Error(w, "delete schedule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = appendAuditEvent(&auditEvent{Actor: actorLogin(user), Action: "schedules.delete", Target: "schedule/" + id, Repo: schedule.Repo, Outcome: auditOutcomeSuccess}) //nolint:errcheck // best-effort audit
		w.WriteHeader(http.StatusNoContent)
	case action == "pause" && r.Method == http.MethodPost:
		schedule.Status = scheduleStatusPaused
		schedule.UpdatedAt = time.Now().UTC()
		if err := saveSchedule(schedule); err != nil {
			http.Error(w, "save schedule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(schedule) //nolint:errcheck // best-effort response
	case action == "resume" && r.Method == http.MethodPost:
		schedule.Status = scheduleStatusActive
		schedule.UpdatedAt = time.Now().UTC()
		next, err := nextScheduleRun(schedule, schedule.UpdatedAt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		schedule.NextRunAt = next
		if err := saveSchedule(schedule); err != nil {
			http.Error(w, "save schedule: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(schedule) //nolint:errcheck // best-effort response
	case action == "run" && r.Method == http.MethodPost:
		execution, err := s.triggerSchedule(schedule, time.Now().UTC(), "manual")
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		_ = json.NewEncoder(w).Encode(execution) //nolint:errcheck // best-effort response
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleScheduleTemplates(w http.ResponseWriter, r *http.Request, user *authUser) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "schedules.templates.list", "schedules", "", "") {
		return
	}
	_ = json.NewEncoder(w).Encode(builtInScheduledWorkflowTemplates(s.agentReg)) //nolint:errcheck // best-effort response
}

func (s *Server) startScheduler() {
	s.schedulerMu.Lock()
	defer s.schedulerMu.Unlock()
	if s.schedulerCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.schedulerCancel = cancel
	go s.schedulerLoop(ctx)
}

func (s *Server) stopScheduler() {
	s.schedulerMu.Lock()
	cancel := s.schedulerCancel
	s.schedulerCancel = nil
	s.schedulerMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *Server) schedulerLoop(ctx context.Context) {
	s.runDueSchedules(time.Now().UTC(), "missed")
	s.runAutomaticStorageCleanup("startup")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	lastCleanup := time.Now().UTC()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			now = now.UTC()
			s.runDueSchedules(now, "scheduled")
			if now.Sub(lastCleanup) >= 6*time.Hour {
				s.runAutomaticStorageCleanup("scheduled")
				lastCleanup = now
			}
		}
	}
}

func (s *Server) runDueSchedules(now time.Time, reason string) {
	schedules, err := listSchedules()
	if err != nil {
		slog.Warn("list schedules failed", "error", err)
		return
	}
	for _, schedule := range schedules {
		if schedule == nil || schedule.Status != scheduleStatusActive {
			continue
		}
		if schedule.NextRunAt.IsZero() {
			if err := prepareSchedule(schedule, now); err != nil {
				slog.Warn("prepare schedule failed", "id", schedule.ID, "error", err)
				continue
			}
			_ = saveSchedule(schedule)
		}
		if schedule.NextRunAt.After(now) {
			continue
		}
		if _, err := s.triggerSchedule(schedule, now, reason); err != nil {
			slog.Warn("trigger schedule failed", "id", schedule.ID, "error", err)
		}
	}
}

func (s *Server) triggerSchedule(schedule *scheduleDefinition, now time.Time, reason string) (*scheduleExecution, error) {
	if schedule.Status != scheduleStatusActive && reason != "manual" {
		return nil, fmt.Errorf("schedule is paused")
	}
	if schedule.ConcurrencyPolicy == schedulePolicyForbid && scheduleHasActiveRun(schedule) {
		execution := schedule.newExecution(now, "skipped", reason, "previous run is still active")
		schedule.Executions = appendScheduleExecution(schedule.Executions, &execution)
		schedule.LastRunAt = now
		schedule.LastRunStatus = execution.Status
		schedule.UpdatedAt = now
		next, err := nextScheduleRun(schedule, now)
		if err == nil {
			schedule.NextRunAt = next
		}
		_ = saveSchedule(schedule)
		s.notifyScheduleExecution(schedule, &execution)
		return &execution, nil
	}
	req := schedule.orchestrateRequest()
	record, err := s.createOrchestration(req, orchestrationStartOptions{
		ScheduleID: schedule.ID,
		Actor:      schedule.Actor,
	})
	if err != nil {
		execution := schedule.newExecution(now, "failed", reason, err.Error())
		schedule.Executions = appendScheduleExecution(schedule.Executions, &execution)
		schedule.LastRunAt = now
		schedule.LastRunStatus = execution.Status
		schedule.UpdatedAt = now
		_ = saveSchedule(schedule)
		s.notifyScheduleExecution(schedule, &execution)
		return &execution, err
	}
	execution := schedule.newExecution(now, "started", reason, "")
	execution.RunID = record.ID
	schedule.Executions = appendScheduleExecution(schedule.Executions, &execution)
	schedule.LastRunAt = now
	schedule.LastRunID = record.ID
	schedule.LastRunStatus = record.Status
	schedule.UpdatedAt = now
	next, err := nextScheduleRun(schedule, now)
	if err != nil {
		return &execution, err
	}
	schedule.NextRunAt = next
	if err := saveSchedule(schedule); err != nil {
		return &execution, err
	}
	s.notifyScheduleExecution(schedule, &execution)
	return &execution, nil
}

func decodeSchedule(body io.Reader) (*scheduleDefinition, error) {
	var schedule scheduleDefinition
	if err := json.NewDecoder(body).Decode(&schedule); err != nil {
		return nil, fmt.Errorf("parse body: %w", err)
	}
	return &schedule, nil
}

func prepareSchedule(schedule *scheduleDefinition, now time.Time) error {
	schedule.TemplateID = strings.TrimSpace(schedule.TemplateID)
	schedule.Name = strings.TrimSpace(schedule.Name)
	schedule.Repo = strings.TrimSpace(schedule.Repo)
	schedule.BaseBranch = defaultBaseBranch(schedule.BaseBranch)
	schedule.Task = strings.TrimSpace(schedule.Task)
	schedule.Strategy = strings.TrimSpace(schedule.Strategy)
	if schedule.Strategy == "" {
		schedule.Strategy = "sequential"
	}
	schedule.ConcurrencyPolicy = strings.ToLower(strings.TrimSpace(schedule.ConcurrencyPolicy))
	if schedule.ConcurrencyPolicy == "" {
		schedule.ConcurrencyPolicy = schedulePolicyForbid
	}
	if schedule.ConcurrencyPolicy != schedulePolicyForbid && schedule.ConcurrencyPolicy != schedulePolicyAllow {
		return fmt.Errorf("concurrencyPolicy must be forbid or allow")
	}
	if schedule.Name == "" {
		schedule.Name = scheduleShortText(schedule.Task, 80)
	}
	if schedule.Repo == "" {
		schedule.Repo = "."
	}
	if len(schedule.Agents) == 0 || schedule.Task == "" {
		return fmt.Errorf("agents and task are required")
	}
	if schedule.Strategy != "sequential" && schedule.Strategy != "parallel" {
		return fmt.Errorf("strategy must be sequential or parallel")
	}
	limits, err := normalizeGovernanceLimits(schedule.Limits)
	if err != nil {
		return err
	}
	schedule.Limits = limits
	schedule.Notification.Triggers = normalizedNotificationTriggers(schedule.Notification.Triggers)
	schedule.Notification.Destinations = normalizedNotificationDestinations(schedule.Notification.Destinations)
	if _, err := scheduleLocation(schedule); err != nil {
		return err
	}
	next, err := nextScheduleRun(schedule, now)
	if err != nil {
		return err
	}
	schedule.NextRunAt = next
	return nil
}

func builtInScheduledWorkflowTemplates(registry *agent.Registry) []scheduledWorkflowTemplate {
	templates := []scheduledWorkflowTemplate{
		{
			ID:                "daily-failed-run-report",
			Name:              "Daily Failed-Run Report",
			Description:       "Summarize failed or blocked orchestrations and identify recurring causes.",
			Category:          "reporting",
			Agents:            availableAgentNames(registry, "analyst", "reporter"),
			Strategy:          "sequential",
			Schedule:          scheduleSpec{Type: "cron", Cron: "0 9 * * *", Timezone: "UTC"},
			ConcurrencyPolicy: schedulePolicyForbid,
			GitHub: orchestrateGitHubRequest{
				CreateIssue:   true,
				IssueTitle:    "Daily AgentOS failed-run report",
				IssueTemplate: "default",
			},
			ExpectedOutputs: []string{
				"Markdown report with failed run IDs, status, and timeline evidence.",
				"Root-cause grouping and concrete follow-up actions.",
				"Comparison against previous repository memory or run history when available.",
			},
			RequiredPermissions: []string{"Read orchestration records", "Read repository context", "Create GitHub Issue when enabled"},
			Task: `Create a daily failed-run report for {{repo}} on {{baseBranch}}.

Use repository context search, orchestration history, run artifacts, and available GitHub evidence. Separate confirmed evidence from inference. Include:
- failed, canceled, pending approval, or blocked runs since the last report
- recurring failure causes and affected agents
- source evidence links or run IDs
- recommended owner actions
- explicit no-data sections when no failed runs are found

Produce a concise Markdown report in the requested output language.`,
		},
		{
			ID:                "weekly-repository-health-report",
			Name:              "Weekly Repository Health Report",
			Description:       "Inspect repository health signals and produce a maintenance report.",
			Category:          "reporting",
			Agents:            availableAgentNames(registry, "analyst", "reporter"),
			Strategy:          "sequential",
			Schedule:          scheduleSpec{Type: "cron", Cron: "0 9 * * 1", Timezone: "UTC"},
			ConcurrencyPolicy: schedulePolicyForbid,
			GitHub: orchestrateGitHubRequest{
				CreateIssue:   true,
				IssueTitle:    "Weekly repository health report",
				IssueTemplate: "default",
			},
			ExpectedOutputs: []string{
				"Markdown health report covering CI, open issues/PRs, recent failures, stale context, and risks.",
				"Evidence links to GitHub issues, PRs, checks, workflow runs, or run artifacts.",
				"Prioritized maintenance recommendations.",
			},
			RequiredPermissions: []string{"Read GitHub issues and PRs", "Read workflow/check evidence", "Create GitHub Issue when enabled"},
			Task: `Create a weekly repository health report for {{repo}} on {{baseBranch}}.

Review live GitHub evidence, recent orchestration runs, repository memory, and guidelines. Cover CI health, stale issues or PRs, recurring quality gate failures, missing documentation, and operational risk. Include a previous-run comparison when history exists and state clearly when evidence is unavailable.

Produce a structured Markdown report in the requested output language.`,
		},
		{
			ID:                "weekly-security-triage",
			Name:              "Weekly Security Triage",
			Description:       "Review security-sensitive signals and produce a triage report.",
			Category:          "security",
			Agents:            availableAgentNames(registry, "security", "analyst", "reporter"),
			Strategy:          "sequential",
			Schedule:          scheduleSpec{Type: "cron", Cron: "0 10 * * 1", Timezone: "UTC"},
			ConcurrencyPolicy: schedulePolicyForbid,
			GitHub: orchestrateGitHubRequest{
				CreateIssue:   true,
				IssueTitle:    "Weekly security triage report",
				IssueTemplate: "default",
			},
			ExpectedOutputs: []string{
				"Markdown security triage report with evidence and severity notes.",
				"Findings for dependency, auth, secret-handling, and workflow risks.",
				"Recommended remediation issues or PR follow-ups.",
			},
			RequiredPermissions: []string{"Read repository code and security-relevant files", "Read GitHub evidence", "Create GitHub Issue when enabled"},
			Task: `Create a weekly security triage report for {{repo}} on {{baseBranch}}.

Review security-sensitive repository areas, recent GitHub issues/PRs/checks, dependency or workflow signals, and existing repository guidelines. Distinguish confirmed vulnerabilities from risk indicators. Include evidence links, severity, recommended next steps, and no-data sections for unavailable sources.

Produce a structured Markdown report in the requested output language.`,
		},
		{
			ID:                "weekly-dependency-update",
			Name:              "Weekly Dependency Update",
			Description:       "Prepare dependency updates with CI validation and review notes.",
			Category:          "maintenance",
			Agents:            availableAgentNames(registry, "dependency-updater", "ci-fixer", "reviewer"),
			Strategy:          "sequential",
			Schedule:          scheduleSpec{Type: "cron", Cron: "0 11 * * 1", Timezone: "UTC"},
			ConcurrencyPolicy: schedulePolicyForbid,
			GitHub: orchestrateGitHubRequest{
				CreatePullRequest: true,
				PRTitle:           "Weekly dependency maintenance",
				PRTemplate:        "default",
			},
			ExpectedOutputs: []string{
				"Dependency update diff or a no-update report.",
				"CI and quality gate validation summary.",
				"Pull request when update changes are produced and PR creation is enabled.",
			},
			RequiredPermissions: []string{"Read and update dependency manifests", "Run validation commands", "Create GitHub PR when enabled"},
			Task: `Perform weekly dependency maintenance for {{repo}} on {{baseBranch}}.

Inspect dependency manifests and lockfiles, propose conservative updates, run relevant validation, and summarize risk. If no update is appropriate, produce a no-change report with evidence. If updates are made, prepare PR-ready notes with validation results and rollback considerations.

Use the requested output language for reports and GitHub artifacts.`,
		},
		{
			ID:                "monthly-release-readiness",
			Name:              "Monthly Release Readiness",
			Description:       "Assess release readiness, blockers, and rollout risk.",
			Category:          "release",
			Agents:            availableAgentNames(registry, "release-manager", "analyst", "reporter"),
			Strategy:          "sequential",
			Schedule:          scheduleSpec{Type: "cron", Cron: "0 9 1 * *", Timezone: "UTC"},
			ConcurrencyPolicy: schedulePolicyForbid,
			GitHub: orchestrateGitHubRequest{
				CreateIssue:   true,
				IssueTitle:    "Monthly release readiness report",
				IssueTemplate: "default",
			},
			ExpectedOutputs: []string{
				"Markdown readiness report with blockers, completed work, and residual risk.",
				"Release checklist, validation status, and rollout notes.",
				"Evidence links to issues, PRs, checks, and recent orchestration records.",
			},
			RequiredPermissions: []string{"Read release notes and changelog", "Read GitHub issues/PRs/checks", "Create GitHub Issue when enabled"},
			Task: `Create a monthly release readiness report for {{repo}} on {{baseBranch}}.

Review changelog, release notes, open issues, merged PRs, CI status, repository memory, and recent orchestration outcomes. Identify release blockers, validation gaps, documentation gaps, rollback risks, and recommended next actions. Include evidence links and previous-run comparison when available.

Produce a structured Markdown report in the requested output language.`,
		},
		{
			ID:                "memory-guideline-stale-check",
			Name:              "Memory and Guideline Stale Check",
			Description:       "Review repository memory and guidelines for stale or conflicting context.",
			Category:          "maintenance",
			Agents:            availableAgentNames(registry, "analyst", "reporter"),
			Strategy:          "sequential",
			Schedule:          scheduleSpec{Type: "cron", Cron: "0 9 * * 5", Timezone: "UTC"},
			ConcurrencyPolicy: schedulePolicyForbid,
			GitHub: orchestrateGitHubRequest{
				CreateIssue:   true,
				IssueTitle:    "Repository memory and guideline stale-context report",
				IssueTemplate: "default",
			},
			ExpectedOutputs: []string{
				"Markdown report listing stale, duplicated, or conflicting memory/guideline entries.",
				"Evidence for why each item should be kept, archived, or updated.",
				"Recommended cleanup actions.",
			},
			RequiredPermissions: []string{"Read repository memory and guidelines", "Read run history", "Create GitHub Issue when enabled"},
			Task: `Review repository memory and guidelines for {{repo}} on {{baseBranch}}.

Find stale, duplicated, overly broad, or conflicting entries by comparing repository context, recent run history, and current files. Do not archive automatically. Provide evidence and recommended cleanup actions, including no-data sections when no stale context is found.

Produce a concise Markdown report in the requested output language.`,
		},
	}

	filtered := templates[:0]
	for i := range templates {
		template := &templates[i]
		if len(template.Agents) == 0 {
			continue
		}
		filtered = append(filtered, *template)
	}
	return filtered
}

func scheduleShortText(value string, size int) string {
	value = strings.TrimSpace(value)
	if size <= 0 || len(value) <= size {
		return value
	}
	return value[:size-1] + "..."
}

func (schedule *scheduleDefinition) orchestrateRequest() *orchestrateRequest {
	return &orchestrateRequest{
		Agents:         append([]string{}, schedule.Agents...),
		CustomAgents:   append([]agent.Definition{}, schedule.CustomAgents...),
		Scenario:       schedule.Scenario,
		Repo:           schedule.Repo,
		BaseBranch:     schedule.BaseBranch,
		Task:           schedule.Task,
		Strategy:       schedule.Strategy,
		LLMPreset:      schedule.LLMPreset,
		OutputLanguage: schedule.OutputLanguage,
		GitHub:         schedule.GitHub,
		Limits:         schedule.Limits,
	}
}

func (schedule *scheduleDefinition) newExecution(now time.Time, status, reason, message string) scheduleExecution {
	return scheduleExecution{
		ID:        "exec-" + strings.TrimPrefix(generateID(), "run-"),
		Status:    status,
		Reason:    reason,
		Message:   safety.NewRedactor().RedactString(message),
		StartedAt: now.UTC(),
	}
}

func appendScheduleExecution(executions []scheduleExecution, execution *scheduleExecution) []scheduleExecution {
	if execution == nil {
		return executions
	}
	executions = append(executions, *execution)
	if len(executions) > 50 {
		return executions[len(executions)-50:]
	}
	return executions
}

func scheduleHasActiveRun(schedule *scheduleDefinition) bool {
	if schedule == nil || schedule.LastRunID == "" {
		return false
	}
	record, err := readOrchestrationRecord(schedule.LastRunID)
	return err == nil && orchestrationInProgress(record.Status)
}

func refreshScheduleRunStatuses(schedules []*scheduleDefinition) {
	for _, schedule := range schedules {
		refreshScheduleRunStatus(schedule)
	}
}

func refreshScheduleRunStatus(schedule *scheduleDefinition) {
	if schedule == nil || schedule.LastRunID == "" {
		return
	}
	record, err := readOrchestrationRecord(schedule.LastRunID)
	if err != nil {
		return
	}
	schedule.LastRunStatus = record.Status
	for i := range schedule.Executions {
		if schedule.Executions[i].RunID == record.ID {
			schedule.Executions[i].Status = record.Status
		}
	}
	_ = saveSchedule(schedule)
}

func schedulesDir() string {
	return filepath.Join(apphome.Dir(), "schedules")
}

func saveSchedule(schedule *scheduleDefinition) error {
	if !isValidScheduleID(schedule.ID) {
		return fmt.Errorf("invalid schedule id")
	}
	if err := os.MkdirAll(schedulesDir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(schedulesDir(), schedule.ID+".json")
	data, err := json.MarshalIndent(safety.NewRedactor().RedactValue(schedule), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readSchedule(id string) (*scheduleDefinition, error) {
	if !isValidScheduleID(id) {
		return nil, fmt.Errorf("invalid schedule id")
	}
	data, err := os.ReadFile(filepath.Join(schedulesDir(), id+".json"))
	if err != nil {
		return nil, err
	}
	var schedule scheduleDefinition
	if err := json.Unmarshal(data, &schedule); err != nil {
		return nil, err
	}
	return &schedule, nil
}

func deleteSchedule(id string) error {
	if !isValidScheduleID(id) {
		return fmt.Errorf("invalid schedule id")
	}
	err := os.Remove(filepath.Join(schedulesDir(), id+".json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func listSchedules() ([]*scheduleDefinition, error) {
	entries, err := os.ReadDir(schedulesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []*scheduleDefinition{}, nil
		}
		return nil, err
	}
	schedules := make([]*scheduleDefinition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		schedule, err := readSchedule(id)
		if err != nil {
			slog.Warn("skip unreadable schedule", "id", id, "error", err)
			continue
		}
		schedules = append(schedules, schedule)
	}
	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].CreatedAt.After(schedules[j].CreatedAt)
	})
	return schedules, nil
}

func isValidScheduleID(id string) bool {
	return scheduleIDPattern.MatchString(id)
}

func nextScheduleRun(schedule *scheduleDefinition, after time.Time) (time.Time, error) {
	spec := schedule.Schedule
	spec.Type = strings.ToLower(strings.TrimSpace(spec.Type))
	if spec.Type == "" {
		if strings.TrimSpace(spec.Cron) != "" {
			spec.Type = "cron"
		} else {
			spec.Type = "interval"
		}
	}
	loc, err := scheduleLocation(schedule)
	if err != nil {
		return time.Time{}, err
	}
	switch spec.Type {
	case "interval":
		interval, err := time.ParseDuration(strings.TrimSpace(spec.Interval))
		if err != nil || interval <= 0 {
			return time.Time{}, fmt.Errorf("schedule.interval must be a positive duration")
		}
		return after.UTC().Add(interval), nil
	case "cron":
		return nextCronRun(strings.TrimSpace(spec.Cron), after, loc)
	default:
		return time.Time{}, fmt.Errorf("schedule.type must be interval or cron")
	}
}

func scheduleLocation(schedule *scheduleDefinition) (*time.Location, error) {
	tz := "UTC"
	if schedule != nil && strings.TrimSpace(schedule.Schedule.Timezone) != "" {
		tz = strings.TrimSpace(schedule.Schedule.Timezone)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone: %s", tz)
	}
	return loc, nil
}

func nextCronRun(expr string, after time.Time, loc *time.Location) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron must have 5 fields")
	}
	minutes, err := parseCronField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron minute: %w", err)
	}
	hours, err := parseCronField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron hour: %w", err)
	}
	days, err := parseCronField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron day: %w", err)
	}
	months, err := parseCronField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron month: %w", err)
	}
	weekdays, err := parseCronField(fields[4], 0, 7)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron weekday: %w", err)
	}
	cursor := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	limit := cursor.AddDate(5, 0, 0)
	for !cursor.After(limit) {
		weekday := int(cursor.Weekday())
		if weekdays[7] && weekday == 0 {
			weekday = 7
		}
		if minutes[cursor.Minute()] && hours[cursor.Hour()] && days[cursor.Day()] && months[int(cursor.Month())] && weekdays[weekday] {
			return cursor.UTC(), nil
		}
		cursor = cursor.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("cron has no run time within 5 years")
}

func parseCronField(field string, minValue, maxValue int) (map[int]bool, error) {
	values := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty field")
		}
		step := 1
		if base, rawStep, ok := strings.Cut(part, "/"); ok {
			part = base
			parsed, err := strconv.Atoi(rawStep)
			if err != nil || parsed <= 0 {
				return nil, fmt.Errorf("invalid step")
			}
			step = parsed
		}
		start, end, err := cronFieldRange(part, minValue, maxValue)
		if err != nil {
			return nil, err
		}
		for i := start; i <= end; i += step {
			values[i] = true
		}
	}
	if maxValue == 7 && values[7] {
		values[0] = true
	}
	return values, nil
}

func cronFieldRange(part string, minValue, maxValue int) (start, end int, err error) {
	if part == "*" {
		return minValue, maxValue, nil
	}
	if startRaw, endRaw, ok := strings.Cut(part, "-"); ok {
		start, err := strconv.Atoi(startRaw)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range start")
		}
		end, err := strconv.Atoi(endRaw)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range end")
		}
		if start < minValue || end > maxValue || start > end {
			return 0, 0, fmt.Errorf("range out of bounds")
		}
		return start, end, nil
	}
	value, err := strconv.Atoi(part)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid value")
	}
	if value < minValue || value > maxValue {
		return 0, 0, fmt.Errorf("value out of bounds")
	}
	return value, value, nil
}
