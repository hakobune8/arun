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

package state

import (
	"os"
	"testing"
	"time"
)

func TestRunStore_SaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewRunStore(dir)

	now := time.Now().Truncate(time.Second)
	original := &RunRecord{
		TaskID:      "task-1",
		Status:      RunStatusPlanning,
		StartedAt:   now,
		FinishedAt:  now.Add(5 * time.Minute),
		ProfileName: "default",
		Branch:      "main",
		Error:       "",
		Iteration:   3,
	}

	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.TaskID != original.TaskID {
		t.Errorf("TaskID = %q, want %q", loaded.TaskID, original.TaskID)
	}
	if loaded.Status != original.Status {
		t.Errorf("Status = %q, want %q", loaded.Status, original.Status)
	}
	if loaded.ProfileName != original.ProfileName {
		t.Errorf("ProfileName = %q, want %q", loaded.ProfileName, original.ProfileName)
	}
	if loaded.Branch != original.Branch {
		t.Errorf("Branch = %q, want %q", loaded.Branch, original.Branch)
	}
	if loaded.Iteration != original.Iteration {
		t.Errorf("Iteration = %d, want %d", loaded.Iteration, original.Iteration)
	}
	if !loaded.StartedAt.Equal(original.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", loaded.StartedAt, original.StartedAt)
	}
	if !loaded.FinishedAt.Equal(original.FinishedAt) {
		t.Errorf("FinishedAt = %v, want %v", loaded.FinishedAt, original.FinishedAt)
	}
}

func TestRunStore_StatusUpdates(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewRunStore(dir)

	record := &RunRecord{
		TaskID:      "task-2",
		Status:      RunStatusPending,
		StartedAt:   time.Now(),
		ProfileName: "test",
		Branch:      "feature",
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	loaded, _ := store.Load()
	if loaded.Status != RunStatusPending {
		t.Errorf("Status = %q, want %q", loaded.Status, RunStatusPending)
	}

	record.Status = RunStatusExecuting
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	loaded, _ = store.Load()
	if loaded.Status != RunStatusExecuting {
		t.Errorf("Status = %q, want %q", loaded.Status, RunStatusExecuting)
	}

	record.Status = RunStatusCompleted
	now := time.Now()
	record.FinishedAt = now
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	loaded, _ = store.Load()
	if loaded.Status != RunStatusCompleted {
		t.Errorf("Status = %q, want %q", loaded.Status, RunStatusCompleted)
	}
	if !loaded.FinishedAt.Equal(now) {
		t.Errorf("FinishedAt = %v, want %v", loaded.FinishedAt, now)
	}
}

func TestRunStore_LoadNonExistent(t *testing.T) {
	t.Parallel()

	store := NewRunStore(t.TempDir())
	_, err := store.Load()
	if err == nil {
		t.Fatal("expected error loading from empty directory")
	}
}

func TestRunStore_ErrorStatus(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewRunStore(dir)

	record := &RunRecord{
		TaskID: "task-3",
		Status: RunStatusFailed,
		Error:  "something went wrong",
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	loaded, _ := store.Load()
	if loaded.Error != "something went wrong" {
		t.Errorf("Error = %q, want %q", loaded.Error, "something went wrong")
	}
}

func TestRunStore_AllStatuses(t *testing.T) {
	t.Parallel()

	statuses := []RunStatus{
		RunStatusPending,
		RunStatusPlanning,
		RunStatusExecuting,
		RunStatusTesting,
		RunStatusReviewing,
		RunStatusCompleted,
		RunStatusFailed,
		RunStatusCancelled,
	}

	dir := t.TempDir()
	store := NewRunStore(dir)

	for _, s := range statuses {
		record := &RunRecord{
			TaskID: "status-test",
			Status: s,
		}
		if err := store.Save(record); err != nil {
			t.Fatalf("Save with status %q: %v", s, err)
		}
		loaded, err := store.Load()
		if err != nil {
			t.Fatalf("Load with status %q: %v", s, err)
		}
		if loaded.Status != s {
			t.Errorf("Status = %q, want %q", loaded.Status, s)
		}
	}
}

func TestRunStore_FilePermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewRunStore(dir)

	record := &RunRecord{
		TaskID: "perms",
		Status: RunStatusPending,
	}
	if err := store.Save(record); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir + "/run_state.json")
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o444 == 0 {
		t.Error("file should be readable")
	}
}
