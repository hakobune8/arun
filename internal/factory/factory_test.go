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

package factory

import (
	"testing"

	"github.com/kazyamaz200/agentos/internal/agent"
)

func TestAgentDef_Defaults(t *testing.T) {
	t.Parallel()

	def := AgentDef{}
	if def.Name != "" {
		t.Errorf("Name = %q, want empty", def.Name)
	}
	if def.Role != "" {
		t.Errorf("Role = %q, want empty", def.Role)
	}
	if def.Model != "" {
		t.Errorf("Model = %q, want empty", def.Model)
	}
	if def.Limits.MaxIterations != 0 {
		t.Errorf("MaxIterations = %d, want 0", def.Limits.MaxIterations)
	}
	if def.Limits.MaxRetries != 0 {
		t.Errorf("MaxRetries = %d, want 0", def.Limits.MaxRetries)
	}
}

func TestAgentDef_FullDefinition(t *testing.T) {
	t.Parallel()

	def := AgentDef{
		Name:         "test-agent",
		Profile:      "default",
		Role:         "tester",
		Model:        "gpt-4",
		SystemPrompt: "You are a test agent",
		Tools:        []string{"read_file", "write_file"},
	}
	def.Limits.MaxIterations = 25
	def.Limits.MaxRetries = 3

	if def.Name != "test-agent" {
		t.Errorf("Name = %q, want %q", def.Name, "test-agent")
	}
	if def.Profile != "default" {
		t.Errorf("Profile = %q, want %q", def.Profile, "default")
	}
	if def.Role != "tester" {
		t.Errorf("Role = %q, want %q", def.Role, "tester")
	}
	if def.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", def.Model, "gpt-4")
	}
	if def.SystemPrompt != "You are a test agent" {
		t.Errorf("SystemPrompt = %q, want %q", def.SystemPrompt, "You are a test agent")
	}
	if len(def.Tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(def.Tools))
	}
	if def.Limits.MaxIterations != 25 {
		t.Errorf("MaxIterations = %d, want 25", def.Limits.MaxIterations)
	}
	if def.Limits.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", def.Limits.MaxRetries)
	}
}

func TestAgentTemplate_Defaults(t *testing.T) {
	t.Parallel()

	tmpl := AgentTemplate{}
	if tmpl.Schema != "" {
		t.Errorf("Schema = %q, want empty", tmpl.Schema)
	}
	if len(tmpl.Agents) != 0 {
		t.Errorf("got %d agents, want 0", len(tmpl.Agents))
	}
	if tmpl.Coordination.Strategy != "" {
		t.Errorf("Coordination.Strategy = %q, want empty", tmpl.Coordination.Strategy)
	}
}

func TestDefaultTemplate(t *testing.T) {
	t.Parallel()

	tmpl := DefaultTemplate()
	if tmpl == nil {
		t.Fatal("DefaultTemplate returned nil")
		return
	}
	if tmpl.Schema != "agentos/v1" {
		t.Errorf("Schema = %q, want %q", tmpl.Schema, "agentos/v1")
	}
	if len(tmpl.Agents) != 2 {
		t.Fatalf("got %d agents, want 2", len(tmpl.Agents))
	}
	if tmpl.Agents[0].Name != "coder" {
		t.Errorf("Agents[0].Name = %q, want %q", tmpl.Agents[0].Name, "coder")
	}
	if tmpl.Agents[1].Name != "reviewer" {
		t.Errorf("Agents[1].Name = %q, want %q", tmpl.Agents[1].Name, "reviewer")
	}
	if tmpl.Coordination.Strategy != "sequential" {
		t.Errorf("Coordination.Strategy = %q, want %q", tmpl.Coordination.Strategy, "sequential")
	}
}

func TestNewFactory(t *testing.T) {
	t.Parallel()

	f := NewFactory("/work/dir")
	if f == nil {
		t.Fatal("NewFactory returned nil")
	}
	if f.WorkDir() != "/work/dir" {
		t.Errorf("WorkDir() = %q, want %q", f.WorkDir(), "/work/dir")
	}
}

func TestFactory_ListAgents_Empty(t *testing.T) {
	t.Parallel()

	f := NewFactory(t.TempDir())
	names, err := f.ListAgents()
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("got %d agents, want 0", len(names))
	}
}

func TestNewAgentRunner(t *testing.T) {
	t.Parallel()

	f := NewFactory(t.TempDir())
	r := NewAgentRunner(f)
	if r == nil {
		t.Fatal("NewAgentRunner returned nil")
	}
}

func TestProfileFromDefinition(t *testing.T) {
	t.Parallel()

	def := &agent.Definition{
		APIVersion: agent.CurrentSchemaVersion,
		Kind:       "Agent",
		Metadata: agent.DefinitionMetadata{
			Name:   "go-backend",
			Labels: map[string]string{"role": "backend"},
		},
		Spec: agent.DefinitionSpec{
			LLM: agent.LLMConfig{
				Model:       "coder",
				Temperature: 0.1,
				MaxTokens:   4096,
			},
			Tools: agent.ToolsConfig{Allow: []string{"read_file", "test"}},
			Safety: agent.SafetyConfig{
				DenyCommands: []string{"sudo"},
			},
			Commands: agent.CommandsConfig{
				Test: "go test ./...",
				Lint: "go vet ./...",
			},
			Limits: agent.LimitsConfig{
				MaxRetries:    2,
				MaxIterations: 5,
			},
		},
	}

	prof := ProfileFromDefinition(def)
	if prof.Name != "go-backend" {
		t.Fatalf("Name = %q", prof.Name)
	}
	if prof.Role != "backend" {
		t.Fatalf("Role = %q", prof.Role)
	}
	if prof.LLM.Model != "coder" || prof.LLM.MaxTokens != 4096 {
		t.Fatalf("LLM not mapped: %+v", prof.LLM)
	}
	if prof.Commands.Test != "go test ./..." || prof.Commands.Lint != "go vet ./..." {
		t.Fatalf("commands not mapped: %+v", prof.Commands)
	}
	if prof.Limits.MaxRetries != 2 || prof.Limits.MaxIterations != 5 {
		t.Fatalf("limits not mapped: %+v", prof.Limits)
	}
	if len(prof.Tools.Allow) != 2 || prof.Tools.Allow[1] != "test" {
		t.Fatalf("tools not mapped: %+v", prof.Tools.Allow)
	}
	if len(prof.Tools.DenyCommands) != 1 || prof.Tools.DenyCommands[0] != "sudo" {
		t.Fatalf("deny commands not mapped: %+v", prof.Tools.DenyCommands)
	}
}
