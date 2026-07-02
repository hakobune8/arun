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
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/apphome"
	agentosgh "github.com/kazyamaz200/agentos/internal/github"
	"github.com/kazyamaz200/agentos/internal/safety"
)

var notificationIDPattern = regexp.MustCompile(`^notification-[0-9a-f]{16}$`)

const (
	notificationTriggerStarted            = "started"
	notificationTriggerCompleted          = "completed"
	notificationTriggerFailed             = "failed"
	notificationTriggerSkipped            = "skipped"
	notificationTriggerPRCreated          = "pr_created"
	notificationTriggerQualityGateFailed  = "quality_gate_failed"
	notificationTriggerManualIntervention = "manual_intervention"

	notificationDestinationInbox         = "inbox"
	notificationDestinationWebhook       = "webhook"
	notificationDestinationGitHubIssue   = "github_issue"
	notificationDestinationGitHubPR      = "github_pr"
	notificationDeliverySuccess          = "success"
	notificationDeliveryFailure          = "failure"
	notificationDeliverySkipped          = "skipped"
	defaultNotificationWebhookRetryCount = 3
)

type notificationRecord struct {
	ID           string                        `json:"id"`
	ScheduleID   string                        `json:"scheduleId,omitempty"`
	RunID        string                        `json:"runId,omitempty"`
	Trigger      string                        `json:"trigger"`
	Title        string                        `json:"title"`
	Message      string                        `json:"message"`
	Status       string                        `json:"status,omitempty"`
	Repo         string                        `json:"repo,omitempty"`
	RunURL       string                        `json:"runUrl,omitempty"`
	Destinations []string                      `json:"destinations,omitempty"`
	Deliveries   []notificationDeliveryAttempt `json:"deliveries,omitempty"`
	CreatedAt    time.Time                     `json:"createdAt"`
}

type notificationDeliveryAttempt struct {
	Destination string    `json:"destination"`
	Status      string    `json:"status"`
	Target      string    `json:"target,omitempty"`
	URL         string    `json:"url,omitempty"`
	Attempts    int       `json:"attempts,omitempty"`
	Error       string    `json:"error,omitempty"`
	DeliveredAt time.Time `json:"deliveredAt,omitempty"`
}

func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "notifications.read", "notifications", "", "") {
		return
	}
	records, err := listNotificationRecords(100)
	if err != nil {
		http.Error(w, "list notifications: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(records) //nolint:errcheck // best-effort response
}

func (s *Server) handleNotificationDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	user, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/notifications/"), "/")
	if !isValidNotificationID(id) {
		http.Error(w, "invalid notification id", http.StatusBadRequest)
		return
	}
	if !s.requireAutomationPermission(w, r, user, "notifications.manage", "notification/"+id, "", "") {
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := deleteNotificationRecord(id); err != nil {
			http.Error(w, "delete notification: "+err.Error(), http.StatusInternalServerError)
			return
		}
		_ = appendAuditEvent(&auditEvent{Actor: actorLogin(user), Action: "notifications.delete", Target: "notification/" + id, Outcome: auditOutcomeSuccess}) //nolint:errcheck // best-effort audit
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) notifyScheduledRun(record *orchestrationRecord) {
	if record == nil || record.ScheduleID == "" {
		return
	}
	schedule, err := readSchedule(record.ScheduleID)
	if err != nil {
		slog.Warn("read schedule for notification failed", "schedule", record.ScheduleID, "error", err)
		return
	}
	triggers := notificationTriggersForRecord(record)
	if len(triggers) == 0 || !schedule.Notification.Enabled {
		return
	}
	for _, trigger := range triggers {
		if !schedule.Notification.matches(trigger) {
			continue
		}
		if notificationRecordExists(schedule.ID, record.ID, trigger) {
			continue
		}
		notification := buildRunNotification(schedule, record, trigger)
		if err := s.deliverNotification(schedule, record, notification); err != nil {
			slog.Warn("deliver notification failed", "notification", notification.ID, "error", err)
		}
	}
}

func (s *Server) notifyScheduleExecution(schedule *scheduleDefinition, execution *scheduleExecution) {
	if schedule == nil || execution == nil || !schedule.Notification.Enabled || !schedule.Notification.matches(execution.Status) {
		return
	}
	notification := buildExecutionNotification(schedule, execution)
	if err := s.deliverNotification(schedule, nil, notification); err != nil {
		slog.Warn("deliver schedule notification failed", "notification", notification.ID, "error", err)
	}
}

func (s *Server) deliverNotification(schedule *scheduleDefinition, record *orchestrationRecord, notification *notificationRecord) error {
	if notification == nil {
		return nil
	}
	notification.Destinations = normalizedNotificationDestinations(schedule.Notification.Destinations)
	if len(notification.Destinations) == 0 {
		notification.Destinations = []string{notificationDestinationInbox}
	}
	for _, destination := range notification.Destinations {
		switch destination {
		case notificationDestinationInbox:
			notification.Deliveries = append(notification.Deliveries, notificationDeliveryAttempt{
				Destination: destination,
				Status:      notificationDeliverySuccess,
				DeliveredAt: time.Now().UTC(),
			})
		case notificationDestinationWebhook:
			notification.Deliveries = append(notification.Deliveries, deliverWebhookNotification(schedule.Notification.WebhookURL, notification))
		case notificationDestinationGitHubIssue, notificationDestinationGitHubPR:
			notification.Deliveries = append(notification.Deliveries, deliverGitHubNotification(record, destination, notification))
		default:
			notification.Deliveries = append(notification.Deliveries, notificationDeliveryAttempt{
				Destination: destination,
				Status:      notificationDeliverySkipped,
				Error:       "unsupported destination",
				DeliveredAt: time.Now().UTC(),
			})
		}
	}
	return saveNotificationRecord(notification)
}

func deliverWebhookNotification(webhookURL string, notification *notificationRecord) notificationDeliveryAttempt {
	attempt := notificationDeliveryAttempt{
		Destination: notificationDestinationWebhook,
		Target:      safety.NewRedactor().RedactString(webhookURL),
		Status:      notificationDeliveryFailure,
	}
	webhookURL = strings.TrimSpace(webhookURL)
	if webhookURL == "" {
		attempt.Error = "webhook URL is required"
		attempt.DeliveredAt = time.Now().UTC()
		return attempt
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		attempt.Error = err.Error()
		attempt.DeliveredAt = time.Now().UTC()
		return attempt
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for i := 1; i <= defaultNotificationWebhookRetryCount; i++ {
		attempt.Attempts = i
		req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(payload))
		if err != nil {
			attempt.Error = err.Error()
			break
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			attempt.Error = err.Error()
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			attempt.Status = notificationDeliverySuccess
			attempt.Error = ""
			attempt.DeliveredAt = time.Now().UTC()
			return attempt
		}
		attempt.Error = fmt.Sprintf("webhook status %d", resp.StatusCode)
	}
	attempt.DeliveredAt = time.Now().UTC()
	attempt.Error = safety.NewRedactor().RedactString(attempt.Error)
	return attempt
}

func deliverGitHubNotification(record *orchestrationRecord, destination string, notification *notificationRecord) notificationDeliveryAttempt {
	attempt := notificationDeliveryAttempt{
		Destination: destination,
		Status:      notificationDeliverySkipped,
		DeliveredAt: time.Now().UTC(),
	}
	if record == nil || record.GitHub == nil {
		attempt.Error = "missing GitHub state"
		return attempt
	}
	number := record.GitHub.IssueNumber
	targetURL := record.GitHub.IssueURL
	if destination == notificationDestinationGitHubPR {
		number = record.GitHub.PullRequestNumber
		targetURL = record.GitHub.PullRequestURL
	}
	attempt.Target = targetURL
	if number <= 0 {
		attempt.Error = "GitHub target is not available"
		return attempt
	}
	owner, name, _, ok := githubRepoForAPI(record.GitHub.Repo)
	if !ok {
		attempt.Status = notificationDeliveryFailure
		attempt.Error = "invalid GitHub repository"
		return attempt
	}
	body := notificationGitHubBody(notification)
	comment, err := agentosgh.NewClient(owner, name).CreateIssueComment(number, agentosgh.CreateIssueCommentRequest{Body: body})
	attempt.Attempts = 1
	if err != nil {
		attempt.Status = notificationDeliveryFailure
		attempt.Error = safety.NewRedactor().RedactString(err.Error())
		return attempt
	}
	attempt.Status = notificationDeliverySuccess
	attempt.URL = comment.HTMLURL
	return attempt
}

func notificationTriggersForRecord(record *orchestrationRecord) []string {
	switch record.Status {
	case "completed":
		triggers := []string{notificationTriggerCompleted}
		if record.GitHub != nil && record.GitHub.PullRequestURL != "" {
			triggers = append(triggers, notificationTriggerPRCreated)
		}
		if !orchestrationQualityGatePassed(record) {
			triggers = append(triggers, notificationTriggerQualityGateFailed)
		}
		return triggers
	case "pending_approval":
		return []string{notificationTriggerManualIntervention}
	case "failed", "canceled":
		return []string{notificationTriggerFailed}
	default:
		return nil
	}
}

func buildRunNotification(schedule *scheduleDefinition, record *orchestrationRecord, trigger string) *notificationRecord {
	title := fmt.Sprintf("%s: %s", schedule.Name, strings.ReplaceAll(trigger, "_", " "))
	message := fmt.Sprintf("Scheduled run %s finished with status %s.", record.ID, record.Status)
	if record.Error != "" {
		message += " Error: " + record.Error
	}
	if record.Summary != "" {
		message += "\n\n" + scheduleShortText(record.Summary, 800)
	}
	return &notificationRecord{
		ID:         "notification-" + strings.TrimPrefix(generateID(), "run-"),
		ScheduleID: schedule.ID,
		RunID:      record.ID,
		Trigger:    trigger,
		Title:      safety.NewRedactor().RedactString(title),
		Message:    safety.NewRedactor().RedactString(message),
		Status:     record.Status,
		Repo:       record.Repo,
		RunURL:     orchestrationRunReference(record),
		CreatedAt:  time.Now().UTC(),
	}
}

func buildExecutionNotification(schedule *scheduleDefinition, execution *scheduleExecution) *notificationRecord {
	return &notificationRecord{
		ID:         "notification-" + strings.TrimPrefix(generateID(), "run-"),
		ScheduleID: schedule.ID,
		RunID:      execution.RunID,
		Trigger:    execution.Status,
		Title:      safety.NewRedactor().RedactString(fmt.Sprintf("%s: %s", schedule.Name, execution.Status)),
		Message:    safety.NewRedactor().RedactString(execution.Message),
		Status:     execution.Status,
		Repo:       schedule.Repo,
		CreatedAt:  time.Now().UTC(),
	}
}

func notificationGitHubBody(notification *notificationRecord) string {
	var b strings.Builder
	fmt.Fprintf(&b, "AgentOS scheduled orchestration notification.\n\n")
	fmt.Fprintf(&b, "- Trigger: %s\n", notification.Trigger)
	fmt.Fprintf(&b, "- Status: %s\n", notification.Status)
	if notification.RunURL != "" {
		fmt.Fprintf(&b, "- Run: %s\n", notification.RunURL)
	}
	if notification.Message != "" {
		fmt.Fprintf(&b, "\n%s\n", notification.Message)
	}
	return strings.TrimSpace(b.String())
}

func (notification scheduleNotification) matches(trigger string) bool {
	trigger = strings.ToLower(strings.TrimSpace(trigger))
	for _, candidate := range normalizedNotificationTriggers(notification.Triggers) {
		if candidate == trigger {
			return true
		}
	}
	return false
}

func normalizedNotificationTriggers(values []string) []string {
	if len(values) == 0 {
		values = []string{notificationTriggerFailed, notificationTriggerQualityGateFailed, notificationTriggerManualIntervention}
	}
	return normalizedNotificationValues(values)
}

func normalizedNotificationDestinations(values []string) []string {
	if len(values) == 0 {
		return []string{notificationDestinationInbox}
	}
	return normalizedNotificationValues(values)
}

func normalizedNotificationValues(values []string) []string {
	seen := map[string]bool{}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	return normalized
}

func notificationsDir() string {
	return filepath.Join(apphome.Dir(), "notifications")
}

func isValidNotificationID(id string) bool {
	return notificationIDPattern.MatchString(id)
}

func saveNotificationRecord(notification *notificationRecord) error {
	if notification == nil {
		return nil
	}
	if err := os.MkdirAll(notificationsDir(), 0o755); err != nil {
		return err
	}
	path := filepath.Join(notificationsDir(), notification.ID+".json")
	data, err := json.MarshalIndent(safety.NewRedactor().RedactValue(notification), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func deleteNotificationRecord(id string) error {
	if !isValidNotificationID(id) {
		return fmt.Errorf("invalid notification id")
	}
	err := os.Remove(filepath.Join(notificationsDir(), id+".json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func listNotificationRecords(limit int) ([]notificationRecord, error) {
	entries, err := os.ReadDir(notificationsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []notificationRecord{}, nil
		}
		return nil, err
	}
	records := make([]notificationRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(notificationsDir(), entry.Name()))
		if err != nil {
			slog.Warn("skip unreadable notification", "file", entry.Name(), "error", err)
			continue
		}
		var record notificationRecord
		if err := json.Unmarshal(data, &record); err != nil {
			slog.Warn("skip invalid notification", "file", entry.Name(), "error", err)
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func notificationRecordExists(scheduleID, runID, trigger string) bool {
	records, err := listNotificationRecords(0)
	if err != nil {
		return false
	}
	for i := range records {
		record := &records[i]
		if record.ScheduleID == scheduleID && record.RunID == runID && record.Trigger == trigger {
			return true
		}
	}
	return false
}
