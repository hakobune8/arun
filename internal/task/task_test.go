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

package task

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "task.yaml")
	content := []byte(`id: test-1
type: bugfix
repo: org/repo
base_branch: main
branch: fix/thing
title: Fix the thing
description: This fixes the thing
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	task, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if task.ID != "test-1" {
		t.Errorf("ID = %q, want %q", task.ID, "test-1")
	}
	if task.Type != "bugfix" {
		t.Errorf("Type = %q, want %q", task.Type, "bugfix")
	}
	if task.Repo != "org/repo" {
		t.Errorf("Repo = %q, want %q", task.Repo, "org/repo")
	}
	if task.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", task.BaseBranch, "main")
	}
	if task.Branch != "fix/thing" {
		t.Errorf("Branch = %q, want %q", task.Branch, "fix/thing")
	}
	if task.Title != "Fix the thing" {
		t.Errorf("Title = %q, want %q", task.Title, "Fix the thing")
	}
	if task.Description != "This fixes the thing" {
		t.Errorf("Description = %q, want %q", task.Description, "This fixes the thing")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := Load("/nonexistent/path.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(`: invalid yaml [`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_DefaultsApplied(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.yaml")
	content := []byte(`id: test-2
repo: org/repo
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	task, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if task.Type != "issue_to_patch" {
		t.Errorf("Type = %q, want %q", task.Type, "issue_to_patch")
	}
	if task.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", task.BaseBranch, "main")
	}
	if task.Branch != "agent/test-2" {
		t.Errorf("Branch = %q, want %q", task.Branch, "agent/test-2")
	}
}

func TestLoad_MissingID(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "noid.yaml")
	content := []byte(`repo: org/repo
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestLoad_MissingRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "norepo.yaml")
	content := []byte(`id: test-3
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
}
