# Changelog

## [Unreleased]

## [v1.4.0] - 2026-07-02

### Added
- Added orchestration governance controls for maximum duration, subtasks,
  retries, repository concurrency, organization concurrency, LLM token budgets,
  and GitHub request budgets, with server-side enforcement for duration,
  subtask count, and concurrency limits.
- Added storage retention policies with usage reporting, dry-run cleanup,
  execution history, archive-before-delete, and safe skips for active or
  GitHub-linked orchestration records.
- Added Web UI controls for storage usage, retention policy preview, cleanup
  execution, and cleanup report review.
- Added the `agentos evals` orchestration evaluation suite with deterministic
  scenario coverage, functional coverage reporting, JSON/Markdown reports, and
  opt-in live smoke checks for environments that provide credentials.

### Fixed
- Fixed mobile storage cleanup controls so cleanup requires a previewed
  selection, reflects busy and empty states, and keeps the bottom navigation
  labels from overlapping on narrow screens.

### Notes
- Live orchestration evals are intentionally opt-in. The default suite remains
  secret-free and deterministic for CI.
- Helm chart `version`, `appVersion`, and default `image.tag` are aligned to
  `v1.4.0`.
- v1.4.x follow-up work tracks authenticated Web UI E2E, live GitHub and
  Kubernetes operational scenarios, schedule-to-notification coverage, real
  LLM smoke coverage, LiteLLM preset tuning, and agile scrum simulation.

## [v1.3.0] - 2026-07-01

### Added
- Added built-in `analyst` and `reporter` agents for investigation workflows,
  structured reports, evidence provenance, and no-data reporting expectations.
- Added repository-scoped live GitHub evidence search for issues, pull
  requests, checks, and workflow run logs with provenance metadata and secret
  redaction.
- Added explicit Kubernetes log evidence search through configured kubectl,
  namespace, and label selector settings.
- Added recurring orchestration schedules with interval or cron timing,
  timezone-aware next-run calculation, pause/resume, run-now, execution
  history, and overlap prevention.
- Added built-in scheduled maintenance and reporting workflow templates for
  failed-run reports, repository health, security triage, dependency updates,
  release readiness, and stale memory/guideline checks.
- Added scheduled orchestration outcome notifications with inbox history,
  webhook delivery retries, and optional GitHub Issue or PR comments.
- Added built-in Docker, Helm, Kubernetes, and DevOps operations agents with
  planner routing, validation gates, and deployment-safety expectations.

### Fixed
- Fixed multi-arch Docker image builds by running the frontend build on the
  BuildKit build platform and cross-compiling the Go binary for each target
  architecture.

### Notes
- Scheduled operations run in-process with the Web server. Keep persistent
  storage enabled for schedules, notification history, orchestration records,
  repository memory, and guidelines.
- Webhook delivery for schedule notifications is outbound-only. GitHub-to-
  AgentOS webhook delivery remains optional for issue-triggered workflows.
- Helm chart `version`, `appVersion`, and default `image.tag` are aligned to
  `v1.3.0`.

## [v1.2.0] - 2026-07-01

### Added
- Expanded built-in agent registry for broader repository workflows, including
  frontend, security, release, dependency, QA, and convention-aware planning
  guidance.
- Repository-defined custom agent profiles through `.agentos/agents/*.yaml`.
- Reusable scenario templates, including repository-defined
  `.agentos/scenarios/*.yaml` templates.
- Repository-scoped continuous improvement memory with approval before reuse.
- Repository-specific guideline management and retrieval.
- Repository-scoped context search across memory, guidelines, orchestration
  runs, run artifacts, GitHub artifacts, and code/files.
- React, TypeScript, Vite, and Tailwind CSS Web UI with mobile-first
  orchestration, agent, audit, GitHub, memory, guideline, and search views.
- GitHub repository picker API for authenticated Web UI repository selection.

### Changed
- Orchestration routing now uses stronger built-in agent metadata, repository
  signals, scenario templates, and task recommendations when assigning
  specialist agents.
- The Web UI is served from built React assets instead of the legacy static
  HTML implementation.
- Frontend build, lint, and responsive smoke checks are part of CI and Docker
  image builds.

### Deferred
- Built-in Docker, Helm, and Kubernetes operations agents were moved to the
  v1.3.0 milestone.

### Notes
- GitHub-to-AgentOS webhook delivery is still not required for v1.2.0 unless a
  later release task changes that before tagging.
- The `on_pr_merge` close policy remains recorded for conservative follow-up;
  automatic PR merge detection remains deferred.

## [v1.1] - 2026-06-30

### Added
- GitHub App installation token support for repository write operations.
- First-class Issue and Pull Request creation in orchestration records.
- RBAC checks and audit logs for automation actions.
- Centralized secret redaction for logs, reports, and generated artifacts.
- Explicit orchestration quality gates for expected outputs, tests, lint, and diffs.
- Live Web UI orchestration progress with logs, timeline events, and cancellation.
- Language and template controls for generated artifacts and GitHub output.
- Responsive Web UI improvements for mobile and narrow viewports.
- Issue-triggered orchestration through labels, slash-style commands, and manual import.
- Source issue status comments for issue-sourced orchestration runs.
- Issue close policies and human approval gates for conservative automation.
- Task-context recommendations for agent sets, templates, quality gates, and close policy defaults.

### Changed
- Web UI orchestration creation now exposes recommendations, GitHub output controls, quality gates, and approval state.
- GitHub automation defaults favor human approval for higher-risk or operations-oriented tasks.
- Orchestration completion records include GitHub metadata such as source issue, branch, pull request, close policy, approval status, and source close state.

### Notes
- GitHub-to-AgentOS webhook delivery is not required for v1.1 because deployments may not be reachable from GitHub.
- The `on_pr_merge` close policy is recorded for manual follow-up; webhook-based automatic PR merge detection is deferred.

## [v1.0.1] - 2026-06-30

### Fixed
- Empty remote repositories now complete multi-agent orchestration through deterministic fallback artifacts.
- `go-backend`, `docs`, and `ci-fixer` agents create expected fallback files when LLM execution returns no usable outputs.
- Timed-out contexts can still produce deterministic fallback artifacts.
- No-op orchestration success is prevented when expected outputs are missing.

## [v1.0.0] - 2026-06-29

### Added
- Runtime Agent interface (Plan, Execute, Review) with lifecycle hooks (#91)
- Versioned Agent definition schema (apiVersion: agentos.io/v1) (#97)
- Agent plugin registry with built-in agents (go-backend, reviewer, ci-fixer, docs) (#93)
- Structured event bus with typed events and file store persistence (#94)
- JSON memory store backend (zero dependencies) (#95)
- Sandbox interface abstraction with LocalSandbox and Docker stub (#96)
- Agent Factory from versioned Definition YAML (#98)
- Multi-agent orchestration wired to actual runtime execution (#99)
- Tool Description() method on all built-in tools and MCP adapter (#92)
- Registry validation, lifecycle support, and duplicate detection (#92)
- Helm chart for Kubernetes deployment (#104)
- Documentation for Event Bus, Agent Definitions, Factory, Memory, Sandbox,
  Orchestrator, Embedding, Search, Guidelines, and MCP (#102)

### Changed
- Runtime delegates planning/execution/review to Agent interface
- MemoryStore renamed to VectorStore implementing Store interface
- Workspace renamed to LocalSandbox implementing Sandbox interface
- Orchestrator uses runtime.Agent interface and agent registry

### Fixed
- BuildAgentFromDefinition now returns LLM client properly

## [v0.5] - 2026-06-28

### Added
- Agent Factory: create agent instances from YAML template definitions
- Multi-agent orchestration with sequential/parallel strategies
- CLI commands: `agentos agent list/create/run`, `agentos orchestrate`
- Agent template system with coder/reviewer/tester template
- Package-level Go doc comments (ongoing)

### Changed
- Profile loading uses var instead of value receiver for DefaultProfile

## [v0.4] - 2026-06-27

### Added
- MCP client (JSON-RPC stdio) with tool registration
- Docker sandbox interface stub for future isolated execution
- Web UI dashboard (`agentos serve`)
- GitHub CI checks integration
- CI Fix Agent for automated CI failure resolution

### Changed
- Internal: safety package structure improvements

## [v0.3] - 2026-06-26

### Added
- Vector search with local JSON store and Qdrant backend
- Agent memory system for cross-run context retention
- Coding guidelines management
- LiteLLM embedding support
- Unified search across memory, guidelines, and PRs

### Changed
- LLM client interface extended for embedding support

## [v0.2] - 2026-06-25

### Added
- GitHub API client for issue/PR/checks operations
- `agentos issue`, `agentos pr`, `agentos checks` commands
- Auto-PR creation on `agentos run --pr`
- CI Fix Agent prototype

## [v0.1] - 2026-06-24

### Added
- Initial AgentOS implementation
- CLI with `run`, `review`, `version` commands
- LLM client with LiteLLM integration
- Tool system: filesystem, shell, git, search, test tools
- Safety layer: command denylist, secret detection, branch protection
- Task/profile YAML loading
- Runtime orchestration with plan/execute/review/retry lifecycle
- Run state persistence and JSONL logging
