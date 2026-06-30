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
		Domains:       []string{"backend", "go", "api", "service", "frontend-fallback"},
		TriggerKeywords: []string{
			"go", "backend", "api", "server", "handler", "endpoint", "database", "service", "react", "frontend", "ui",
		},
		TriggerFiles: []string{"go.mod", "go.sum", "cmd/", "internal/", "pkg/", "api/", "package.json", "vite.config.ts", "vite.config.js"},
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
		Domains:       []string{"review", "quality", "release-readiness"},
		TriggerKeywords: []string{
			"review", "diff", "approval", "risk", "maintainability", "release readiness",
		},
		TriggerFiles:     []string{".github/", "go.mod", "package.json", "Dockerfile", "charts/", "k8s/", "deploy/"},
		RecommendedAfter: []string{"go-backend", "ci-fixer", "docs", "security", "release-manager", "dependency-updater", "qa"},
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
		Domains:       []string{"ci", "github-actions", "validation"},
		TriggerKeywords: []string{
			"ci", "github actions", "workflow", "check failed", "lint", "build failure", "test failure",
		},
		TriggerFiles:     []string{".github/workflows/", ".github/workflows/*.yaml", ".github/workflows/*.yml"},
		RecommendedAfter: []string{"go-backend", "dependency-updater"},
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
		Domains:       []string{"documentation", "developer-experience", "release-notes"},
		TriggerKeywords: []string{
			"docs", "documentation", "readme", "guide", "manual", "quickstart", "changelog",
		},
		TriggerFiles:     []string{"README.md", "docs/", "CHANGELOG.md", ".agentos/config.yaml"},
		RecommendedAfter: []string{"go-backend", "release-manager"},
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

	r.MustRegister(&Info{
		Name:          "security",
		Description:   "Security agent — reviews dependencies, auth/session handling, secrets, and security-sensitive diffs",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"security", "auth", "secrets", "dependencies"},
		TriggerKeywords: []string{
			"security", "vulnerability", "cve", "secret", "xss", "csrf", "sql injection", "permission", "authz", "codeql",
		},
		TriggerFiles:     []string{"SECURITY.md", ".github/workflows/codeql.yml", ".github/dependabot.yml", "go.sum", "package-lock.json"},
		RecommendedAfter: []string{"go-backend", "dependency-updater"},
		ArchitectureGuidance: []string{
			"Inspect authentication, authorization, session, secret-handling, dependency, and CI security conventions before proposing changes.",
			"Prefer small defensive fixes, safer defaults, and standard library or existing dependency patterns over broad rewrites.",
			"Document residual risk and validation scope when a finding cannot be fully fixed in the current task.",
		},
		OutputExpectations: []string{
			"Security-sensitive changes include tests or explicit manual verification notes.",
			"Dependency or configuration findings identify the affected package, file, workflow, or setting.",
			"go test ./... and go vet ./... pass when code is changed.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("security", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "release-manager",
		Description:   "Release manager agent — prepares changelogs, version notes, release checklists, and readiness validation",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git"},
		Domains:       []string{"release", "deployment", "helm", "kubernetes", "docker"},
		TriggerKeywords: []string{
			"release", "changelog", "version", "rollback", "helm", "kubernetes", "k8s", "docker", "deployment", "ingress",
		},
		TriggerFiles:     []string{"CHANGELOG.md", "charts/", "Chart.yaml", "values.yaml", "Dockerfile", "k8s/", "deploy/", "deployment.yaml", "ingress.yaml"},
		RecommendedAfter: []string{"go-backend", "ci-fixer", "qa", "security"},
		ArchitectureGuidance: []string{
			"Inspect existing changelog, release note, versioning, and Helm chart conventions before editing release artifacts.",
			"Keep version changes explicit and avoid publishing or tagging releases unless the task asks for it.",
			"Summarize release readiness, known gaps, and deployment or rollback considerations.",
		},
		OutputExpectations: []string{
			"CHANGELOG.md or release documentation is updated when release notes are requested.",
			"Version and chart changes are consistent when release packaging is in scope.",
			"Release checklist items are concrete and traceable to validation commands or manual checks.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("release-manager", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "dependency-updater",
		Description:   "Dependency updater agent — updates Go modules, package locks, and GitHub Actions versions with compatibility checks",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"dependencies", "go-modules", "package-locks", "github-actions"},
		TriggerKeywords: []string{
			"dependency", "dependencies", "upgrade", "bump", "go mod", "go.sum", "package-lock", "pnpm-lock", "yarn.lock",
		},
		TriggerFiles:     []string{"go.mod", "go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", ".github/dependabot.yml"},
		RecommendedAfter: []string{"security"},
		ArchitectureGuidance: []string{
			"Inspect existing dependency managers, lockfiles, toolchain versions, and CI compatibility before updating versions.",
			"Prefer narrow updates requested by the task; avoid broad upgrades unless the task calls for them.",
			"Keep generated files such as go.sum or lockfiles consistent with the manifest that changed.",
		},
		OutputExpectations: []string{
			"Manifests and lockfiles remain synchronized after updates.",
			"go mod tidy and go test ./... pass for Go dependency work.",
			"Compatibility or breaking-change notes are included when versions move across major or security-sensitive boundaries.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("dependency-updater", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "qa",
		Description:   "QA agent — adds scenario tests, smoke checks, regression coverage, and manual verification notes",
		Version:       "1.0.0",
		Author:        "AgentOS",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"qa", "tests", "smoke", "regression", "frontend-validation"},
		TriggerKeywords: []string{
			"qa", "quality assurance", "smoke", "scenario test", "regression", "manual verification", "browser", "responsive",
		},
		TriggerFiles:     []string{"*_test.go", "test/", "tests/", "package.json", "playwright.config.ts", "cypress.config.ts"},
		RecommendedAfter: []string{"go-backend", "ci-fixer", "security", "release-manager", "dependency-updater"},
		ArchitectureGuidance: []string{
			"Inspect existing test layout, fixtures, and documented verification workflows before adding new checks.",
			"Prefer focused regression and smoke coverage that exercises user-visible behavior changed by the task.",
			"Record manual verification steps when behavior cannot be fully automated.",
		},
		OutputExpectations: []string{
			"New or updated tests fail without the intended behavior and pass with it.",
			"go test ./... passes when Go code or tests are in scope.",
			"Manual verification notes include concrete commands, URLs, or scenarios.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("qa", llmClient)
	})

	return r
}
