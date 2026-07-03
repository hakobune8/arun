// Copyright 2026 ARUN Authors
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
	"github.com/hakobune8/arun/internal/llm"
	"github.com/hakobune8/arun/internal/runtime"
)

// DefaultRegistry returns a registry pre-populated with all built-in agents.
// This is the primary registry used by the CLI and runtime.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	r.MustRegister(&Info{
		Name:          "go-backend",
		Description:   "Go backend coding agent — plans, codes, tests, and lints Go projects while preserving repository structure",
		Version:       "1.0.0",
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"backend", "go", "api", "service"},
		TriggerKeywords: []string{
			"go", "backend", "api", "server", "handler", "endpoint", "database", "service",
		},
		TriggerFiles: []string{"go.mod", "go.sum", "cmd/", "internal/", "pkg/", "api/"},
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
		Name:          "frontend",
		Description:   "Frontend application agent — implements UI, layout, responsive, accessibility, and frontend validation work",
		Version:       "1.0.0",
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"frontend", "ui", "web-app", "responsive", "accessibility", "visual-validation"},
		TriggerKeywords: []string{
			"frontend", "ui", "react", "vite", "next.js", "nextjs", "vue", "nuxt", "svelte", "sveltekit", "tailwind", "css", "layout", "responsive", "accessibility", "browser", "component", "page",
		},
		TriggerFiles: []string{"package.json", "vite.config.ts", "vite.config.js", "next.config.js", "next.config.mjs", "nuxt.config.ts", "svelte.config.js", "tailwind.config.js", "src/", "app/", "pages/", "components/", "public/", "index.html"},
		ArchitectureGuidance: []string{
			"Inspect package.json, framework config, routing, component structure, styling conventions, and state management before editing UI code.",
			"Preserve existing framework and design-system patterns; prefer existing components, utilities, tokens, and CSS conventions over new dependencies.",
			"Keep layouts responsive and accessible with semantic markup, keyboard-friendly controls, sensible labels, and mobile plus desktop verification where practical.",
		},
		OutputExpectations: []string{
			"UI implementation changes touch the relevant component, page, style, or asset files instead of reporting no-op success.",
			"Available package scripts such as lint, typecheck, test, and build are detected and run when present.",
			"Browser, screenshot, responsive, or manual verification notes are included when visual behavior cannot be fully automated.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("frontend", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "reviewer",
		Description:   "Code review agent — reviews diffs for correctness, tests, security, maintainability, and release readiness",
		Version:       "1.0.0",
		Author:        "ARUN",
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
		Author:        "ARUN",
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
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "write_file", "search", "git"},
		Domains:       []string{"documentation", "developer-experience", "release-notes"},
		TriggerKeywords: []string{
			"docs", "documentation", "readme", "guide", "manual", "quickstart", "changelog",
		},
		TriggerFiles:     []string{"README.md", "docs/", "CHANGELOG.md", ".arun/config.yaml"},
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
		Author:        "ARUN",
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
		Author:        "ARUN",
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
		Author:        "ARUN",
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
		Author:        "ARUN",
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

	r.MustRegister(&Info{
		Name:          "docker",
		Description:   "Docker ops agent — maintains Dockerfiles, image builds, .dockerignore, runtime config, and container safety defaults",
		Version:       "1.0.0",
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"docker", "containers", "images", "compose", "container-security"},
		TriggerKeywords: []string{
			"docker", "dockerfile", "container", "image", "buildkit", "compose", ".dockerignore", "multi-stage", "healthcheck",
		},
		TriggerFiles:     []string{"Dockerfile", "Dockerfile.*", ".dockerignore", "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"},
		RecommendedAfter: []string{"security", "dependency-updater"},
		ArchitectureGuidance: []string{
			"Inspect the existing Dockerfile, compose files, build context, entrypoint, exposed ports, and CI image-build flow before editing container files.",
			"Prefer multi-stage builds, non-root runtime users, minimal copied context, deterministic package installation, and explicit health checks when the application supports them.",
			"Keep secrets out of image layers, build args, labels, logs, and compose files; use runtime environment variables or orchestrator-managed secrets instead.",
		},
		OutputExpectations: []string{
			"Container changes touch Dockerfile, .dockerignore, compose, or related build configuration instead of reporting no-op success.",
			"`docker build` or the repository image-build command is run when Docker is available; otherwise static Dockerfile checks and the reason Docker was unavailable are reported.",
			"Security and runtime notes cover user, ports, health checks, secret handling, image size, and rollback considerations when relevant.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("docker", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "helm",
		Description:   "Helm ops agent — maintains charts, templates, values, schema, chart linting, and release-safe Helm packaging",
		Version:       "1.0.0",
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"helm", "charts", "templates", "values", "kubernetes-packaging"},
		TriggerKeywords: []string{
			"helm", "chart", "charts", "values.yaml", "values.schema.json", "template", "helm lint", "helm template", "appversion",
		},
		TriggerFiles:     []string{"charts/", "Chart.yaml", "values.yaml", "values.schema.json", "templates/", "Chart.lock"},
		RecommendedAfter: []string{"docker", "kubernetes", "security"},
		ArchitectureGuidance: []string{
			"Inspect chart layout, helper templates, values defaults, schema, labels, annotations, and release versioning before changing templates.",
			"Preserve existing values structure and helper conventions; avoid hard-coded environment-specific data in templates.",
			"Prefer conservative Kubernetes defaults, schema-backed values, stable labels/selectors, and explicit upgrade or rollback notes for chart changes.",
		},
		OutputExpectations: []string{
			"Helm changes touch Chart.yaml, values, templates, schema, or chart tests instead of reporting no-op success.",
			"`helm lint` and `helm template` are run when Helm is available; otherwise YAML/template structure checks and the missing-tool reason are reported.",
			"Chart version, appVersion, values compatibility, and upgrade impact are summarized when packaging or release behavior changes.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("helm", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "kubernetes",
		Description:   "Kubernetes ops agent — maintains manifests, deployments, services, ingress, probes, resources, rollout checks, and cluster-safe defaults",
		Version:       "1.0.0",
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"kubernetes", "k8s", "manifests", "deployments", "services", "ingress", "rollouts"},
		TriggerKeywords: []string{
			"kubernetes", "k8s", "manifest", "deployment", "service", "ingress", "configmap", "secret", "probe", "resources", "rollout", "kubectl",
		},
		TriggerFiles:     []string{"k8s/", "kubernetes/", "manifests/", "deploy/", "deployment.yaml", "service.yaml", "ingress.yaml", "configmap.yaml", "secret.yaml"},
		RecommendedAfter: []string{"docker", "helm", "security"},
		ArchitectureGuidance: []string{
			"Inspect existing manifest layout, namespace assumptions, labels/selectors, service ports, ingress, probes, resources, and rollout conventions before editing.",
			"Prefer standard Kubernetes objects, conservative resource/security defaults, stable selectors, readiness/liveness probes, and non-privileged containers unless explicitly required.",
			"Keep secrets out of manifests and logs; use Secret references, sealed/external secret systems, or documented runtime configuration instead.",
		},
		OutputExpectations: []string{
			"Kubernetes changes touch manifests, kustomize, Helm-rendered resources, or deployment docs instead of reporting no-op success.",
			"YAML parses successfully and `kubectl apply --dry-run=client` or server-side dry-run is run when kubectl and context are available.",
			"Rollout, rollback, probe, resource, securityContext, and secret-handling verification notes are included for deployment changes.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("kubernetes", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "devops",
		Description:   "DevOps umbrella agent — coordinates Docker, Helm, Kubernetes, deployment debugging, and release hardening tasks",
		Version:       "1.0.0",
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "write_file", "search", "shell", "git", "test"},
		Domains:       []string{"devops", "ops", "deployment", "docker", "helm", "kubernetes", "release-hardening"},
		TriggerKeywords: []string{
			"devops", "ops", "deployment", "release hardening", "rollout", "rollback", "docker", "helm", "kubernetes", "k8s", "cluster",
		},
		TriggerFiles:     []string{"Dockerfile", "charts/", "k8s/", "kubernetes/", "deploy/", ".github/workflows/"},
		RecommendedAfter: []string{"docker", "helm", "kubernetes", "security", "qa"},
		ArchitectureGuidance: []string{
			"Inspect the full delivery path from image build to chart/manifests and rollout before choosing the smallest safe operational change.",
			"Coordinate specialist findings from Docker, Helm, Kubernetes, security, and QA work without duplicating low-level edits unnecessarily.",
			"Prefer conservative release-hardening changes with explicit validation, rollback, and residual-risk notes.",
		},
		OutputExpectations: []string{
			"Ops work is decomposed into the relevant Docker, Helm, Kubernetes, security, QA, and review steps when the task spans multiple layers.",
			"Validation results cite image build, chart render/lint, manifest dry-run, smoke checks, or the reason a tool was unavailable.",
			"Final notes include deployment impact, rollback path, manual verification, and follow-up risks.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("devops", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "analyst",
		Description:   "Analyst agent — investigates logs, runs, artifacts, GitHub context, and repository evidence to identify findings and next actions",
		Version:       "1.0.0",
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "search", "shell", "git"},
		Domains:       []string{"analysis", "investigation", "logs", "artifacts", "github", "observability", "root-cause"},
		TriggerKeywords: []string{
			"analyze", "investigate", "investigation", "root cause", "rca", "logs", "artifacts", "run history", "failure pattern", "trend", "evidence", "findings",
		},
		TriggerFiles: []string{"logs/", "artifacts/", ".github/workflows/", "README.md", "docs/", "CHANGELOG.md", ".arun/"},
		ArchitectureGuidance: []string{
			"Gather evidence from run records, artifacts, logs, GitHub issues or PRs, repository files, memory, and guidelines before drawing conclusions.",
			"Separate observed facts from inferences, include confidence level, and call out missing or unavailable sources explicitly.",
			"Redact obvious secrets and keep log excerpts short while preserving enough provenance to verify each finding.",
		},
		OutputExpectations: []string{
			"Investigation output includes summary, scope, evidence, findings, root cause or likely causes, impact, recommendations, and open questions.",
			"Each finding cites source provenance such as run IDs, file paths, issue or PR numbers, timestamps, or short log excerpts.",
			"No-data cases state which sources were checked and what could not be determined.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("analyst", llmClient)
	})

	r.MustRegister(&Info{
		Name:          "reporter",
		Description:   "Reporter agent — turns findings into structured Markdown reports, stakeholder summaries, and optional GitHub-ready updates",
		Version:       "1.0.0",
		Author:        "ARUN",
		RequiredTools: []string{"read_file", "write_file", "search", "git"},
		Domains:       []string{"reporting", "markdown", "incident-report", "release-readiness", "repository-health", "stakeholder-summary"},
		TriggerKeywords: []string{
			"report", "summary", "summarize", "stakeholder", "incident report", "release readiness", "repository health", "findings", "recommendations", "write-up",
		},
		TriggerFiles:     []string{"README.md", "docs/", "CHANGELOG.md", "reports/", ".arun/"},
		RecommendedAfter: []string{"analyst", "security", "qa", "release-manager"},
		ArchitectureGuidance: []string{
			"Use the requested output language and repository templates when provided, preserving existing Markdown and documentation conventions.",
			"Convert analyst findings into audience-appropriate reports without overstating uncertain evidence.",
			"Prepare GitHub issue or PR comment text only when enabled by the orchestration settings.",
		},
		OutputExpectations: []string{
			"Reports include summary, scope, evidence, findings, recommendations, risks, and open questions.",
			"Incident reports include timeline, impact, detection, root cause, mitigation, and prevention when those facts are available.",
			"Repository health and release readiness reports distinguish blockers, risks, validation status, and recommended next actions.",
		},
	}, func(llmClient llm.LLMClient) runtime.Agent {
		return NewBaseAgent("reporter", llmClient)
	})

	return r
}
