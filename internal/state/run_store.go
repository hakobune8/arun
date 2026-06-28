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

// Package state provides types for persisting run state and logging agent
// activity including tool calls and LLM interactions.
package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RunStatus represents the current phase of an agent run.
type RunStatus string

// Standard run status values.
const (
	RunStatusPending    RunStatus = "pending"
	RunStatusPlanning   RunStatus = "planning"
	RunStatusExecuting  RunStatus = "executing"
	RunStatusTesting    RunStatus = "testing"
	RunStatusReviewing  RunStatus = "reviewing"
	RunStatusCompleted  RunStatus = "completed"
	RunStatusFailed     RunStatus = "failed"
	RunStatusCancelled  RunStatus = "canceled"
)

// RunRecord stores the metadata and current status of an agent run.
type RunRecord struct {
	TaskID      string    `json:"task_id"`
	Status      RunStatus `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	ProfileName string    `json:"profile_name"`
	Branch      string    `json:"branch"`
	Error       string    `json:"error,omitempty"`
	Iteration   int       `json:"iteration"`
}

// RunStore persists and loads RunRecord data as JSON in a specified
// directory.
type RunStore struct {
	runDir string
}

// NewRunStore returns a RunStore that stores records in runDir.
func NewRunStore(runDir string) *RunStore {
	return &RunStore{runDir: runDir}
}

// Save writes the run record as run_state.json inside the store directory.
func (s *RunStore) Save(record *RunRecord) error {
	path := filepath.Join(s.runDir, "run_state.json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal run record: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// Load reads the run record from run_state.json inside the store directory.
func (s *RunStore) Load() (*RunRecord, error) {
	path := filepath.Join(s.runDir, "run_state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read run record: %w", err)
	}
	var record RunRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("parse run record: %w", err)
	}
	return &record, nil
}
