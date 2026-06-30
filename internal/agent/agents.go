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
		Description:   "Go backend coding agent — plans, codes, tests, and lints Go projects while preserving repository structure",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		ArchitectureGuidance: []string{
			"Inspect existing layout before editing and follow established package, cmd/, internal/, pkg/, api/, router, and middleware conventions when present.",
			"Prefer idiomatic standard-library Go for small services; introduce frameworks or new top-level layout only when task complexity warrants it.",
			"Separate handlers, configuration, and tests when the repository already uses that structure; avoid over-engineering small repositories.",
		},
		OutputExpectations: []string{
			"Changed Go source and tests are formatted with gofmt.",
			"go test ./... and go vet ./... pass, with go build ./... when a build command is configured.",
			"Architecture choices are reflected in code organization or summarized when new structure is introduced.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("go-backend", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "reviewer",
		Description:   "Code review agent — reviews diffs for correctness, tests, security, maintainability, and release readiness",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "git"},
		ArchitectureGuidance: []string{
			"Evaluate whether changes preserve existing repository conventions before judging style preferences.",
			"Flag over-engineered layouts, unnecessary dependencies, and convention-breaking rewrites when a smaller change would satisfy the task.",
			"Review tests, security-sensitive behavior, maintainability, and release readiness with severity and file references.",
		},
		OutputExpectations: []string{
			"Findings are structured by severity and include file references where applicable.",
			"Review states whether validation or test coverage is sufficient for release.",
			"Approval is withheld for correctness, security, or convention-breaking regressions.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("reviewer", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "ci-fixer",
		Description:   "CI fix agent — analyzes CI failures and applies conventional GitHub Actions and validation fixes",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		ArchitectureGuidance: []string{
			"Inspect existing workflow names, jobs, matrices, and branch-protection expectations before replacing CI structure.",
			"Prefer de facto GitHub Actions patterns such as actions/checkout, actions/setup-go, cache-aware Go setup, go test, and go vet.",
			"Keep lint, test, and optional security steps explicit and compatible with the repository's existing Go version and module layout.",
		},
		OutputExpectations: []string{
			"CI changes are minimal and preserve existing job intent.",
			"Local validation commands mirror the workflow where practical.",
			"Workflow YAML remains branch-protection friendly and runnable on pull_request.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("ci-fixer", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "docs",
		Description:   "Documentation agent — generates and updates practical repository documentation that matches existing style",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "git"},
		ArchitectureGuidance: []string{
			"Inspect README and docs structure before adding new sections or files.",
			"Prefer practical OSS documentation structure: overview, quickstart, configuration, endpoints, testing, deployment, and troubleshooting.",
			"Preserve existing tone, headings, examples, and link conventions.",
		},
		OutputExpectations: []string{
			"Documentation covers the user-visible behavior changed by the task.",
			"Commands and examples are copy-pasteable from the repository root.",
			"Links point to existing files or newly added docs.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("docs", llmClient)
	})

	return r
}
