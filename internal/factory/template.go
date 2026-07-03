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

package factory

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadTemplate reads and parses an agent template from a YAML file.
func LoadTemplate(path string) (*AgentTemplate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}

	var tmpl AgentTemplate
	if err := yaml.Unmarshal(data, &tmpl); err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	if len(tmpl.Agents) == 0 {
		return nil, fmt.Errorf("no agents defined in template")
	}

	return &tmpl, nil
}

// DefaultTemplate returns a default agent template with coder and reviewer agents.
func DefaultTemplate() *AgentTemplate {
	return &AgentTemplate{
		Schema: "arun/v1",
		Agents: []AgentDef{
			{
				Name:  "coder",
				Role:  "coding agent",
				Model: "coder",
				Tools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
			},
			{
				Name:  "reviewer",
				Role:  "code reviewer",
				Model: "coder",
				Tools: []string{"read_file", "search", "git"},
			},
		},
		Coordination: struct {
			Strategy string `yaml:"strategy"`
		}{
			Strategy: "sequential",
		},
	}
}
