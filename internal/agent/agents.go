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
	"github.com/kazyamaz200/agentos/internal/llm"
	"github.com/kazyamaz200/agentos/internal/runtime"
)

// DefaultRegistry returns a registry pre-populated with all built-in agents.
// This is the primary registry used by the CLI and runtime.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	r.MustRegister(&Info{
		Name:          "go-backend",
		Description:   "Go backend coding agent — plans, codes, tests, and lints Go projects",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("go-backend", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "reviewer",
		Description:   "Code review agent — reviews diffs and provides structured feedback",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "git"},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("reviewer", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "ci-fixer",
		Description:   "CI fix agent — analyzes CI failures and generates fix suggestions",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("ci-fixer", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "docs",
		Description:   "Documentation agent — generates and updates documentation",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "git"},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("docs", llmClient)
	})

	return r
}
