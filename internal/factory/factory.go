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

type Factory struct {
	profiles   map[string]*profile.Profile
	llmConfig  llm.Config
	workDir    string
}

func NewFactory(workDir string) *Factory {
	return &Factory{
		profiles:  make(map[string]*profile.Profile),
		llmConfig: llm.DefaultConfig(),
		workDir:   workDir,
	}
}

func (f *Factory) RegisterProfile(name string, prof *profile.Profile) {
	f.profiles[name] = prof
}

func (f *Factory) LoadProfile(path string) (*profile.Profile, error) {
	return profile.Load(path)
}

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
	secretDetector := safety.NewSecretDetector()
	_ = secretDetector

	allowed := make(map[string]bool)
	for _, t := range prof.Tools.Allow {
		allowed[t] = true
	}

	if allowed["read_file"] || len(prof.Tools.Allow) == 0 {
		registry.Register(tools.NewReadFileTool(f.workDir))
	}
	if allowed["write_file"] || len(prof.Tools.Allow) == 0 {
		registry.Register(tools.NewWriteFileTool(f.workDir))
	}
	if allowed["search"] || len(prof.Tools.Allow) == 0 {
		registry.Register(tools.NewSearchTool(f.workDir))
	}
	if allowed["shell"] || len(prof.Tools.Allow) == 0 {
		registry.Register(tools.NewShellTool(policy, f.workDir))
	}
	if allowed["git"] || len(prof.Tools.Allow) == 0 {
		registry.Register(tools.NewGitTool(f.workDir))
	}
	if allowed["test"] || len(prof.Tools.Allow) == 0 {
		registry.Register(tools.NewTestTool(f.workDir))
	}

	return &AgentInstance{
		Def:      def,
		Profile:  prof,
		LLM:      llmClient,
		Registry: registry,
	}, nil
}

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

func (f *Factory) CreateAgentsFromFile(path string) ([]*AgentInstance, error) {
	tmpl, err := LoadTemplate(path)
	if err != nil {
		return nil, err
	}
	return f.CreateAgentsFromTemplate(tmpl)
}

func (f *Factory) DefaultLLMConfig() llm.Config {
	return f.llmConfig
}

func (f *Factory) WorkDir() string {
	return f.workDir
}

func (f *Factory) ListAgents() ([]string, error) {
	var names []string
	for k := range f.profiles {
		names = append(names, k)
	}
	return names, nil
}

type AgentRunner struct {
	factory *Factory
}

func NewAgentRunner(factory *Factory) *AgentRunner {
	return &AgentRunner{factory: factory}
}

func (r *AgentRunner) RunAgent(ctx context.Context, def *AgentDef, taskDesc string) error {
	agent, err := r.factory.CreateAgent(def)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	_ = agent

	fmt.Fprintf(os.Stdout, "Running agent %s (role: %s)\n", def.Name, def.Role)
	fmt.Fprintf(os.Stdout, "  Model: %s\n", def.Profile)
	fmt.Fprintf(os.Stdout, "  Tools: %v\n", def.Tools)
	fmt.Fprintf(os.Stdout, "  Task: %s\n", taskDesc)

	return nil
}
