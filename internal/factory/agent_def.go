package factory

import (
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/tools"
)

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

type AgentInstance struct {
	Def       *AgentDef
	Profile   *profile.Profile
	LLM       llm.LLMClient
	Registry  *tools.Registry
}

type AgentTemplate struct {
	Schema  string                 `yaml:"schema"`
	Agents  []AgentDef             `yaml:"agents"`
	Coordination struct {
		Strategy string `yaml:"strategy"`
	} `yaml:"coordination,omitempty"`
}
