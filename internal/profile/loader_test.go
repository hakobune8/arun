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

package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	content := []byte(`name: test-agent
role: tester
llm:
  provider: openai
  model: gpt-4
  temperature: 0.5
  max_tokens: 4096
tools:
  allow:
    - read
    - write
  deny_commands:
    - rm -rf
commands:
  test: go test ./...
  lint: golangci-lint run
  build: go build
limits:
  max_iterations: 10
  max_retries: 5
  max_changed_files: 15
  max_runtime_minutes: 60
output:
  mode: diff
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	prof, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if prof.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", prof.Name, "test-agent")
	}
	if prof.Role != "tester" {
		t.Errorf("Role = %q, want %q", prof.Role, "tester")
	}
	if prof.LLM.Provider != "openai" {
		t.Errorf("LLM.Provider = %q, want %q", prof.LLM.Provider, "openai")
	}
	if prof.LLM.Model != "gpt-4" {
		t.Errorf("LLM.Model = %q, want %q", prof.LLM.Model, "gpt-4")
	}
	if prof.LLM.Temperature != 0.5 {
		t.Errorf("LLM.Temperature = %f, want %f", prof.LLM.Temperature, 0.5)
	}
	if prof.LLM.MaxTokens != 4096 {
		t.Errorf("LLM.MaxTokens = %d, want %d", prof.LLM.MaxTokens, 4096)
	}
	if len(prof.Tools.Allow) != 2 || prof.Tools.Allow[0] != "read" {
		t.Errorf("Tools.Allow = %v, want [read write]", prof.Tools.Allow)
	}
	if len(prof.Tools.DenyCommands) != 1 || prof.Tools.DenyCommands[0] != "rm -rf" {
		t.Errorf("Tools.DenyCommands = %v, want [rm -rf]", prof.Tools.DenyCommands)
	}
	if prof.Commands.Test != "go test ./..." {
		t.Errorf("Commands.Test = %q, want %q", prof.Commands.Test, "go test ./...")
	}
	if prof.Commands.Lint != "golangci-lint run" {
		t.Errorf("Commands.Lint = %q, want %q", prof.Commands.Lint, "golangci-lint run")
	}
	if prof.Commands.Build != "go build" {
		t.Errorf("Commands.Build = %q, want %q", prof.Commands.Build, "go build")
	}
	if prof.Limits.MaxIterations != 10 {
		t.Errorf("Limits.MaxIterations = %d, want %d", prof.Limits.MaxIterations, 10)
	}
	if prof.Limits.MaxRetries != 5 {
		t.Errorf("Limits.MaxRetries = %d, want %d", prof.Limits.MaxRetries, 5)
	}
	if prof.Limits.MaxChangedFiles != 15 {
		t.Errorf("Limits.MaxChangedFiles = %d, want %d", prof.Limits.MaxChangedFiles, 15)
	}
	if prof.Limits.MaxRuntimeMinute != 60 {
		t.Errorf("Limits.MaxRuntimeMinute = %d, want %d", prof.Limits.MaxRuntimeMinute, 60)
	}
	if prof.Output.Mode != "diff" {
		t.Errorf("Output.Mode = %q, want %q", prof.Output.Mode, "diff")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	t.Parallel()

	_, err := Load("/nonexistent/profile.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte(`: invalid: [yaml`), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoad_MinimalValidProfile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "minimal.yaml")
	content := []byte(`name: minimal
llm:
  model: claude-3
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	prof, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if prof.Name != "minimal" {
		t.Errorf("Name = %q, want %q", prof.Name, "minimal")
	}
	if prof.LLM.Model != "claude-3" {
		t.Errorf("LLM.Model = %q, want %q", prof.LLM.Model, "claude-3")
	}
	if prof.LLM.Provider != "litellm" {
		t.Errorf("LLM.Provider = %q, want %q", prof.LLM.Provider, "litellm")
	}
	if prof.Limits.MaxRetries != 3 {
		t.Errorf("Limits.MaxRetries = %d, want %d", prof.Limits.MaxRetries, 3)
	}
	if prof.Limits.MaxIterations != 8 {
		t.Errorf("Limits.MaxIterations = %d, want %d", prof.Limits.MaxIterations, 8)
	}
}

func TestLoad_MissingName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "noname.yaml")
	content := []byte(`name: ""
llm:
  model: gpt-4
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoad_MissingModel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "nomodel.yaml")
	content := []byte(`name: test
llm:
  model: ""
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing llm.model")
	}
}
