// Package agent provides core agent interfaces and base implementations for coding agents.
package agent

import (
	"github.com/kazyamaz200/agentos/internal/runtime"
)

// Agent defines the interface for a coding agent that can plan, execute, and review tasks.
type Agent interface {
	Name() string
	Plan(ctx *runtime.RunContext) (*runtime.Plan, error)
	Execute(ctx *runtime.RunContext, plan *runtime.Plan) (*runtime.ExecutionResult, error)
	Review(ctx *runtime.RunContext, result *runtime.ExecutionResult) (*runtime.ReviewResult, error)
}

// BaseAgent provides a default implementation of the Agent interface with a name field.
type BaseAgent struct {
	name string
}

// NewBaseAgent creates a new BaseAgent with the given name.
func NewBaseAgent(name string) *BaseAgent {
	return &BaseAgent{name: name}
}

// Name returns the agent's name.
func (a *BaseAgent) Name() string { return a.name }
