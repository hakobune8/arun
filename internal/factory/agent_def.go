// Package factory provides agent creation and configuration from templates.
package factory

import (
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/tools"
)

// AgentDef defines an agent's configuration from a YAML template.
type AgentDef struct {
	Name        string   `yaml:"name"`
	Profile     string   `yaml:"profile"`
	Role        string   `yaml:"role"`
	Model       string   `yaml:"model"`
	SystemPrompt string  `yaml:"system_prompt"`
	Tools       []string `yaml:"tools"`
	Limits      struct {
		MaxIterations int `yaml:"max_iterations"`
		MaxRetries    int `yaml:"max_retries"`
	} `yaml:"limits"`
}

// AgentInstance is a fully initialized agent ready for execution.
type AgentInstance struct {
	Def       *AgentDef
	Profile   *profile.Profile
	LLM       llm.LLMClient
	Registry  *tools.Registry
}

// AgentTemplate defines a multi-agent configuration template.
type AgentTemplate struct {
	Schema  string                 `yaml:"schema"`
	Agents  []AgentDef             `yaml:"agents"`
	Coordination struct {
		Strategy string `yaml:"strategy"`
	} `yaml:"coordination,omitempty"`
}
