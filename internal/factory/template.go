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
		Schema: "agentos/v1",
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
