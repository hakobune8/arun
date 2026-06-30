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

package agent

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// CurrentSchemaVersion is the current version of the agent definition schema.
const CurrentSchemaVersion = "agentos.io/v1"

// Definition is a versioned, declarative agent configuration that the
// runtime can load and use to instantiate a working agent.
//
// Target YAML:
//
//	apiVersion: agentos.io/v1
//	kind: Agent
//	metadata:
//	  name: go-backend
//	  labels:
//	    role: backend
//	spec:
//	  llm:
//	    model: coder
//	    temperature: 0.2
//	    maxTokens: 8192
//	  tools:
//	    allow:
//	      - read_file
//	      - write_file
//	      - search
//	      - shell
//	      - git
//	      - test
//	  safety:
//	    denyCommands:
//	      - sudo
//	      - rm -rf
//	  commands:
//	    test: go test ./...
//	    lint: go vet ./...
//	  limits:
//	    maxRetries: 3
//	  guidance:
//	    architecture:
//	      - Preserve existing repository layout.
//	    outputExpectations:
//	      - Tests pass.
type Definition struct {
	APIVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Metadata   DefinitionMetadata `yaml:"metadata"`
	Spec       DefinitionSpec     `yaml:"spec"`
}

// DefinitionMetadata holds identifying information about the agent.
type DefinitionMetadata struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

// DefinitionSpec holds the full agent configuration.
type DefinitionSpec struct {
	LLM      LLMConfig      `yaml:"llm"`
	Tools    ToolsConfig    `yaml:"tools"`
	Safety   SafetyConfig   `yaml:"safety,omitempty"`
	Commands CommandsConfig `yaml:"commands,omitempty"`
	Limits   LimitsConfig   `yaml:"limits,omitempty"`
	Guidance GuidanceConfig `yaml:"guidance,omitempty"`
}

// LLMConfig configures the language model provider.
type LLMConfig struct {
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature,omitempty"`
	MaxTokens   int     `yaml:"maxTokens,omitempty"`
}

// ToolsConfig specifies which tools are allowed for this agent.
type ToolsConfig struct {
	Allow []string `yaml:"allow,omitempty"`
}

// SafetyConfig specifies denied commands and other safety restrictions.
type SafetyConfig struct {
	DenyCommands []string `yaml:"denyCommands,omitempty"`
}

// CommandsConfig specifies custom commands for the agent.
type CommandsConfig struct {
	Test  string `yaml:"test,omitempty"`
	Lint  string `yaml:"lint,omitempty"`
	Build string `yaml:"build,omitempty"`
}

// LimitsConfig specifies resource limits for the agent.
type LimitsConfig struct {
	MaxRetries    int `yaml:"maxRetries,omitempty"`
	MaxIterations int `yaml:"maxIterations,omitempty"`
}

// GuidanceConfig documents convention-aware behavior expected from an agent.
type GuidanceConfig struct {
	Architecture       []string `yaml:"architecture,omitempty"`
	OutputExpectations []string `yaml:"outputExpectations,omitempty"`
}

// LoadDefinition reads a YAML file and returns a validated Definition.
func LoadDefinition(path string) (*Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read definition: %w", err)
	}

	var def Definition
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse definition: %w", err)
	}

	if err := def.Validate(); err != nil {
		return nil, fmt.Errorf("validate definition: %w", err)
	}

	return &def, nil
}

// Validate checks the definition for required fields and valid values.
func (d *Definition) Validate() error {
	if d.APIVersion == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if d.Kind == "" {
		return fmt.Errorf("kind is required (expected Agent)")
	}
	if d.Kind != "Agent" {
		return fmt.Errorf("unsupported kind: %q (expected Agent)", d.Kind)
	}
	if d.Metadata.Name == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if d.Spec.LLM.Model == "" {
		return fmt.Errorf("spec.llm.model is required")
	}
	return nil
}

// DefaultDefinition returns a Definition with sensible defaults.
func DefaultDefinition() *Definition {
	return &Definition{
		APIVersion: CurrentSchemaVersion,
		Kind:       "Agent",
		Metadata: DefinitionMetadata{
			Name: "default",
		},
		Spec: DefinitionSpec{
			LLM: LLMConfig{
				Model:       "coder",
				Temperature: 0.2,
				MaxTokens:   8192,
			},
			Tools: ToolsConfig{
				Allow: []string{"read_file", "write_file", "search", "shell", "git", "test"},
			},
			Limits: LimitsConfig{
				MaxRetries:    3,
				MaxIterations: 8,
			},
		},
	}
}
