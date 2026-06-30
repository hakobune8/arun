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
	"sort"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/runtime"
)

// Info describes an agent plugin's metadata.
type Info struct {
	Name                 string
	Description          string
	Version              string
	Author               string
	RequiredTools        []string
	ArchitectureGuidance []string
	OutputExpectations   []string
}

// Factory creates a new runtime.Agent instance given an LLM client.
type Factory func(llm llm.LLMClient) runtime.Agent

// Registry is a registry of available agent types that can be
// instantiated by name at runtime. Adding a new agent requires zero
// changes to runtime code — only a call to Register().
type Registry struct {
	factories map[string]Factory
	infos     map[string]Info
}

// NewRegistry creates an empty agent registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
		infos:     make(map[string]Info),
	}
}

// Register adds an agent type to the registry with the given metadata.
// The factory is used to create agent instances when requested.
func (r *Registry) Register(info *Info, factory Factory) error {
	if info.Name == "" {
		return fmt.Errorf("agent name is required")
	}
	if factory == nil {
		return fmt.Errorf("agent factory for %q is nil", info.Name)
	}
	if _, exists := r.factories[info.Name]; exists {
		return fmt.Errorf("agent %q is already registered", info.Name)
	}
	r.factories[info.Name] = factory
	r.infos[info.Name] = *info
	return nil
}

// MustRegister is like Register but panics on error.
func (r *Registry) MustRegister(info *Info, factory Factory) {
	if err := r.Register(info, factory); err != nil {
		panic(err)
	}
}

// Create instantiates an agent by name using the given LLM client.
// Returns an error if the agent name is not registered.
func (r *Registry) Create(name string, llmClient llm.LLMClient) (runtime.Agent, error) {
	factory, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent %q", name)
	}
	return factory(llmClient), nil
}

// List returns metadata for all registered agents, sorted by name.
func (r *Registry) List() []Info {
	result := make([]Info, 0, len(r.infos))
	for name := range r.infos {
		result = append(result, r.infos[name])
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// Has returns true if an agent with the given name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.factories[name]
	return ok
}
