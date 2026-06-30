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
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kazyamaz200/agentos/internal/apphome"
	"github.com/kazyamaz200/agentos/internal/safety"
)

type auditOutcome string

const (
	auditOutcomeAllowed auditOutcome = "allowed"
	auditOutcomeDenied  auditOutcome = "denied"
	auditOutcomeSuccess auditOutcome = "success"
	auditOutcomeFailure auditOutcome = "failure"
)

type auditEvent struct {
	Timestamp time.Time    `json:"timestamp"`
	Actor     string       `json:"actor"`
	Action    string       `json:"action"`
	Target    string       `json:"target"`
	Outcome   auditOutcome `json:"outcome"`
	RunID     string       `json:"runId,omitempty"`
	Repo      string       `json:"repo,omitempty"`
	Message   string       `json:"message,omitempty"`
}

func auditPath() string {
	return filepath.Join(apphome.Dir(), "audit", "audit.jsonl")
}

func appendAuditEvent(event *auditEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	redactor := safety.NewRedactor()
	event.Target = redactor.RedactString(event.Target)
	event.Repo = redactor.RedactString(event.Repo)
	event.Message = redactor.RedactString(event.Message)

	path := auditPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

func listAuditEvents(limit int) ([]auditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	f, err := os.Open(auditPath())
	if err != nil {
		if os.IsNotExist(err) {
			return []auditEvent{}, nil
		}
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()

	var events []auditEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event auditEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read audit log: %w", err)
	}
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}
