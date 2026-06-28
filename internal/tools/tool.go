// Package tools provides the tool abstraction for agent actions including file
// I/O, shell commands, git operations, code search, and test execution.
package tools

import "context"

// ToolInput is a flexible input map passed to a Tool when it is Run.
type ToolInput map[string]interface{}

// ToolOutput is the result returned by a Tool after execution.
type ToolOutput struct {
	Success  bool
	Data     interface{}
	Error    string
}

// Tool is the interface that all agent tools must implement.
type Tool interface {
	Name() string
	Run(ctx context.Context, input ToolInput) ToolOutput
}

// Registry holds a set of named Tool instances and provides lookup and
// registration operations.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a Tool to the registry under its Name.
func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

// Get returns the Tool associated with name, and a boolean indicating whether
// it was found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns the names of all registered tools.
func (r *Registry) List() []string {
	var names []string
	for n := range r.tools {
		names = append(names, n)
	}
	return names
}
