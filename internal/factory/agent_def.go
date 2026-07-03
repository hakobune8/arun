// Copyright 2026 ARUN Authors
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

// Package factory provides agent creation and configuration from templates.
package factory

import (
	"github.com/hakobune8/arun/internal/llm"
	"github.com/hakobune8/arun/internal/profile"
	"github.com/hakobune8/arun/internal/tools"
)

// AgentDef defines an agent's configuration from a YAML template.
type AgentDef struct {
	Name         string   `yaml:"name"`
	Profile      string   `yaml:"profile"`
	Role         string   `yaml:"role"`
	Model        string   `yaml:"model"`
	SystemPrompt string   `yaml:"system_prompt"`
	Tools        []string `yaml:"tools"`
	Limits       struct {
		MaxIterations int `yaml:"max_iterations"`
		MaxRetries    int `yaml:"max_retries"`
	} `yaml:"limits"`
}

// AgentInstance is a fully initialized agent ready for execution.
type AgentInstance struct {
	Def      *AgentDef
	Profile  *profile.Profile
	LLM      llm.LLMClient
	Registry *tools.Registry
}

// AgentTemplate defines a multi-agent configuration template.
type AgentTemplate struct {
	Schema       string     `yaml:"schema"`
	Agents       []AgentDef `yaml:"agents"`
	Coordination struct {
		Strategy string `yaml:"strategy"`
	} `yaml:"coordination,omitempty"`
}
