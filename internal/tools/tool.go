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

// Package tools provides the tool abstraction for agent actions including file
// I/O, shell commands, git operations, code search, and test execution.
package tools

import (
	"context"
	"fmt"
)

// ToolInput is a flexible input map passed to a Tool when it is Run.
type ToolInput map[string]interface{}

// ToolOutput is the result returned by a Tool after execution.
type ToolOutput struct {
	Success bool
	Data    interface{}
	Error   string
}

// ToolSpec describes a tool's metadata for documentation and selection.
type ToolSpec struct {
	Name        string
	Description string
}

// Tool is the interface that all agent tools must implement.
type Tool interface {
	Name() string
	Run(ctx context.Context, input ToolInput) ToolOutput
}

// ToolWithDescription is an optional interface that tools can implement to
// provide a human-readable description for LLM tool selection and docs.
type ToolWithDescription interface {
	Tool
	Description() string
}

// ToolWithLifecycle is an optional interface for tools that need setup
// and teardown (e.g., network connections, file handles).
type ToolWithLifecycle interface {
	Tool
	Init(ctx context.Context) error
	Cleanup(ctx context.Context) error
}

// Validate checks a Tool for basic correctness at registration time.
func Validate(t Tool) error {
	if t == nil {
		return fmt.Errorf("tool cannot be nil")
	}
	if t.Name() == "" {
		return fmt.Errorf("tool must have a non-empty Name")
	}
	return nil
}

// Spec returns the tool's spec including description if available.
func Spec(t Tool) ToolSpec {
	s := ToolSpec{Name: t.Name()}
	if d, ok := t.(ToolWithDescription); ok {
		s.Description = d.Description()
	}
	return s
}

// Registry holds a set of named Tool instances and provides lookup,
// registration, and iteration operations.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a Tool to the registry under its Name. Returns an error
// if the tool is nil, has an empty name, or a tool with that name is
// already registered.
func (r *Registry) Register(t Tool) error {
	if err := Validate(t); err != nil {
		return fmt.Errorf("register %q: %w", t.Name(), err)
	}
	if _, exists := r.tools[t.Name()]; exists {
		return fmt.Errorf("tool %q already registered", t.Name())
	}
	if l, ok := t.(ToolWithLifecycle); ok {
		if err := l.Init(context.Background()); err != nil {
			return fmt.Errorf("init %q: %w", t.Name(), err)
		}
	}
	r.tools[t.Name()] = t
	return nil
}

// MustRegister is like Register but panics on error. Useful for init-time
// registration where errors are unexpected.
func (r *Registry) MustRegister(t Tool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// Get returns the Tool associated with name, and a boolean indicating whether
// it was found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns the ToolSpec for all registered tools.
func (r *Registry) List() []ToolSpec {
	var specs []ToolSpec
	for _, t := range r.tools {
		specs = append(specs, Spec(t))
	}
	return specs
}

// Names returns the names of all registered tools.
func (r *Registry) Names() []string {
	var names []string
	for n := range r.tools {
		names = append(names, n)
	}
	return names
}

// Cleanup calls Cleanup on all tools that implement ToolWithLifecycle.
func (r *Registry) Cleanup(ctx context.Context) {
	for _, t := range r.tools {
		if l, ok := t.(ToolWithLifecycle); ok {
			_ = l.Cleanup(ctx) //nolint:errcheck // best-effort cleanup
		}
	}
}
