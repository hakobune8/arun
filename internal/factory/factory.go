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
	"context"
	"fmt"
	"os"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/safety"
	"github.com/kazyamaz200/agentos/internal/tools"
)

// Factory creates and manages agent instances from configuration templates.
type Factory struct {
	profiles   map[string]*profile.Profile
	llmConfig  llm.Config
	workDir    string
}

// NewFactory creates a new Factory with the given working directory.
func NewFactory(workDir string) *Factory {
	return &Factory{
		profiles:  make(map[string]*profile.Profile),
		llmConfig: llm.DefaultConfig(),
		workDir:   workDir,
	}
}

// RegisterProfile registers a named profile for agent creation.
func (f *Factory) RegisterProfile(name string, prof *profile.Profile) {
	f.profiles[name] = prof
}

// LoadProfile loads a profile from a file path.
func (f *Factory) LoadProfile(path string) (*profile.Profile, error) {
	return profile.Load(path)
}

// CreateAgent creates a fully initialized agent instance from an agent definition.
func (f *Factory) CreateAgent(def *AgentDef) (*AgentInstance, error) {
	var prof *profile.Profile
	var ok bool
	if def.Profile != "" {
		prof, ok = f.profiles[def.Profile]
	}
	if !ok || def.Profile == "" {
		p := profile.DefaultProfile()
		prof = &p
	}
	prof.Name = def.Name
	if def.Role != "" {
		prof.Role = def.Role
	}

	if def.Model != "" {
		prof.LLM.Model = def.Model
	}
	if len(def.Tools) > 0 {
		prof.Tools.Allow = def.Tools
	}
	if def.Limits.MaxIterations > 0 {
		prof.Limits.MaxIterations = def.Limits.MaxIterations
	}
	if def.Limits.MaxRetries > 0 {
		prof.Limits.MaxRetries = def.Limits.MaxRetries
	}

	llmClient := llm.NewLiteLLMClient(f.llmConfig)

	registry := tools.NewRegistry()
	policy := safety.NewCommandPolicy(prof.Tools.DenyCommands)

	allowed := make(map[string]bool)
	for _, t := range prof.Tools.Allow {
		allowed[t] = true
	}

	if allowed["read_file"] || len(prof.Tools.Allow) == 0 {
		registry.MustRegister(tools.NewReadFileTool(f.workDir))
	}
	if allowed["write_file"] || len(prof.Tools.Allow) == 0 {
		registry.MustRegister(tools.NewWriteFileTool(f.workDir))
	}
	if allowed["search"] || len(prof.Tools.Allow) == 0 {
		registry.MustRegister(tools.NewSearchTool(f.workDir))
	}
	if allowed["shell"] || len(prof.Tools.Allow) == 0 {
		registry.MustRegister(tools.NewShellTool(policy, f.workDir))
	}
	if allowed["git"] || len(prof.Tools.Allow) == 0 {
		registry.MustRegister(tools.NewGitTool(f.workDir))
	}
	if allowed["test"] || len(prof.Tools.Allow) == 0 {
		registry.MustRegister(tools.NewTestTool(f.workDir))
	}

	return &AgentInstance{
		Def:      def,
		Profile:  prof,
		LLM:      llmClient,
		Registry: registry,
	}, nil
}

// CreateAgentsFromTemplate creates agent instances from a template.
func (f *Factory) CreateAgentsFromTemplate(tmpl *AgentTemplate) ([]*AgentInstance, error) {
	var agents []*AgentInstance
	for _, def := range tmpl.Agents {
		agent, err := f.CreateAgent(&def)
		if err != nil {
			return nil, fmt.Errorf("create agent %s: %w", def.Name, err)
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

// CreateAgentsFromFile loads a template from a file and creates agent instances.
func (f *Factory) CreateAgentsFromFile(path string) ([]*AgentInstance, error) {
	tmpl, err := LoadTemplate(path)
	if err != nil {
		return nil, err
	}
	return f.CreateAgentsFromTemplate(tmpl)
}

// DefaultLLMConfig returns the default LLM configuration used by the factory.
func (f *Factory) DefaultLLMConfig() llm.Config {
	return f.llmConfig
}

// WorkDir returns the working directory for tool execution.
func (f *Factory) WorkDir() string {
	return f.workDir
}

// ListAgents returns the names of all registered profiles.
func (f *Factory) ListAgents() ([]string, error) {
	var names []string
	for k := range f.profiles {
		names = append(names, k)
	}
	return names, nil
}

// AgentRunner executes agents created by the factory.
type AgentRunner struct {
	factory *Factory
}

// NewAgentRunner creates a new AgentRunner.
func NewAgentRunner(factory *Factory) *AgentRunner {
	return &AgentRunner{factory: factory}
}

// RunAgent runs an agent with the given configuration and task description.
func (r *AgentRunner) RunAgent(ctx context.Context, def *AgentDef, taskDesc string) error {
	_, err := r.factory.CreateAgent(def)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	fmt.Fprintf(os.Stdout, "Running agent %s (role: %s)\n", def.Name, def.Role)
	fmt.Fprintf(os.Stdout, "  Model: %s\n", def.Profile)
	fmt.Fprintf(os.Stdout, "  Tools: %v\n", def.Tools)
	fmt.Fprintf(os.Stdout, "  Task: %s\n", taskDesc)

	return nil
}
