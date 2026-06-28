package profile

// LLMConfig defines the language model settings used by an agent profile.
type LLMConfig struct {
	Provider    string  `yaml:"provider"`
	Model       string  `yaml:"model"`
	Temperature float64 `yaml:"temperature"`
	MaxTokens   int     `yaml:"max_tokens"`
}

// ToolsConfig controls which tools are allowed and which command patterns are
// denied.
type ToolsConfig struct {
	Allow        []string `yaml:"allow"`
	DenyCommands []string `yaml:"deny_commands"`
}

// CommandsConfig holds the shell commands used for testing, linting, and
// building.
type CommandsConfig struct {
	Test  string `yaml:"test"`
	Lint  string `yaml:"lint"`
	Build string `yaml:"build"`
}

// LimitsConfig defines resource and iteration limits for a run.
type LimitsConfig struct {
	MaxIterations    int `yaml:"max_iterations"`
	MaxRetries       int `yaml:"max_retries"`
	MaxChangedFiles  int `yaml:"max_changed_files"`
	MaxRuntimeMinute int `yaml:"max_runtime_minutes"`
}

// OutputConfig controls how agent output is formatted (e.g. patch mode).
type OutputConfig struct {
	Mode string `yaml:"mode"`
}

// Profile defines the complete configuration for an agent, including its
// role, LLM settings, tool permissions, commands, limits, and output mode.
type Profile struct {
	Name     string         `yaml:"name"`
	Role     string         `yaml:"role"`
	LLM      LLMConfig      `yaml:"llm"`
	Tools    ToolsConfig    `yaml:"tools"`
	Commands CommandsConfig `yaml:"commands"`
	Limits   LimitsConfig   `yaml:"limits"`
	Output   OutputConfig   `yaml:"output"`
}

// DefaultProfile returns a Profile with sensible defaults (coder model, 8
// max iterations, patch output mode).
func DefaultProfile() Profile {
	return Profile{
		Name: "default",
		Role: "coding agent",
		LLM: LLMConfig{
			Provider:    "litellm",
			Model:       "coder",
			Temperature: 0.2,
			MaxTokens:   8192,
		},
		Limits: LimitsConfig{
			MaxIterations:    8,
			MaxRetries:       3,
			MaxChangedFiles:  20,
			MaxRuntimeMinute: 30,
		},
		Output: OutputConfig{
			Mode: "patch",
		},
	}
}
