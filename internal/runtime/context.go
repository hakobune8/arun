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

// Package runtime manages the execution lifecycle of coding tasks, including
// planning, execution, testing, linting, review, and result generation.
package runtime

import (
	"context"

	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/profile"
	"github.com/kazyamaz200/agentos/internal/sandbox"
	"github.com/kazyamaz200/agentos/internal/task"
	"github.com/kazyamaz200/agentos/internal/tools"
	"github.com/kazyamaz200/agentos/internal/state"
)

// RunContext holds the context for a single task execution, including task, profile, LLM, sandbox, and configuration.
type RunContext struct {
	Context    context.Context
	Task       *task.Task
	Profile    *profile.Profile
	LLM        llm.LLMClient
	Workspace  sandbox.Sandbox
	Registry  *tools.Registry
	Logger    *state.Logger
	Store     *state.RunStore
	Config    *Config
	Iteration int
	MaxRetries int
}

// Config holds runtime configuration options such as dry-run and verbose modes.
type Config struct {
	DryRun  bool
	Verbose bool
}

// NewRunContext creates a new RunContext from a base context, task, and Runtime instance.
func NewRunContext(ctx context.Context, tk *task.Task, r *Runtime) *RunContext {
	return &RunContext{
		Context:    ctx,
		Task:       tk,
		Profile:    r.Profile,
		LLM:        r.LLM,
		Workspace:  r.Workspace,
		Registry:   r.Registry,
		Logger:     r.Logger,
		Store:      r.Store,
		Config:     r.Config,
		MaxRetries: r.Profile.Limits.MaxRetries,
	}
}
