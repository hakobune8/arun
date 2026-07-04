# Changelog

## [Unreleased]

## [v1.5.17] - 2026-07-04

### Fixed
- Copied static frontend assets into deterministic fallback Docker runtime
  images when a browser UI exists, so container `/` serves the same primary UI
  as local `go run .`.
- Added a canonical fallback `docs/product-brief.md` and strengthened fallback
  game behavior with a gravity-lane mechanic that is implemented in the UI and
  source code rather than only described in docs.
- Tightened `implementation-heavy-scrum` gates for duplicate product briefs,
  README H1 product naming, container `/` smoke checks, and implemented
  differentiating mechanics.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.17.

## [v1.5.16] - 2026-07-04

### Fixed
- Refreshed remote tracking refs before publishing run branches with
  `--force-with-lease` and updated the tracking ref after successful pushes.
  This prevents later implementation-heavy scrum Sprint 2 and Sprint 3
  checkpoint publishes from being rejected as stale after the Sprint 1 PR branch
  is created.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.16.

## [v1.5.15] - 2026-07-04

### Fixed
- Made deterministic Go fallback serve an existing static `index.html` from `/`
  instead of returning unrelated placeholder text. This keeps generated backend
  and frontend artifacts connected when recovery paths are used.
- Stabilized invader-style static frontend fallback naming around one product
  concept so package metadata, HTML title, and visible UI labels agree.
- Tightened `implementation-heavy-scrum` guidance to use
  `docs/product-brief.md` as the single product brief and treat duplicate
  product brief files as release-blocking product coherence gaps.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.15.

## [v1.5.14] - 2026-07-04

### Fixed
- Published implementation-heavy scrum sprint checkpoints only after the
  checkpoint report subtask has completed successfully and the checkpoint commit
  has been created. This prevents pre-checkpoint branch publishing from racing
  the intended checkpoint push.
- Reduced `/api/orchestrates` to lightweight list summaries so the Web UI List
  tab stays responsive when many implementation-heavy runs have large subtasks,
  results, events, and summaries. Full records remain available from
  `/api/orchestrates/{id}`.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.14.

## [v1.5.13] - 2026-07-04

### Fixed
- Published implementation-heavy scrum sprint checkpoint commits to the remote
  branch after each sprint so partial work is recoverable before the final PR
  publish step.
- Forced GitHub Issue and PR artifacts for `implementation-heavy-scrum` runs so
  the remote branch and PR become the source of truth for generated work.
- Created the pull request after the Sprint 1 checkpoint and updated the same
  PR with a concise final body at orchestration completion.
- Added workflow-scope publish recovery: when GitHub rejects generated
  `.github/workflows/**` files because the OAuth token lacks `workflow` scope,
  ARUN moves the workflow definitions into `docs/arun-generated-workflows.md`,
  amends the unpublished checkpoint commit, and retries the push.
- Marked final GitHub publish failures as `completed_with_publish_error`
  instead of a clean `completed` state.
- Shortened generated PR bodies for readability and linked readers to run
  artifacts, generated repository docs, and sprint checkpoint commits.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.13.

## [v1.5.12] - 2026-07-04

### Changed
- Strengthened the built-in `implementation-heavy-scrum` template and sprint
  prompts to keep a single source-of-truth product brief across planning,
  frontend, backend, QA, documentation, and review stages.
- Added release-blocking product coherence checks for concept drift, alternate
  product names, unserved frontend trees, placeholder root responses, broken
  documentation links, and missing differentiating mechanics.
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.12.

## [v1.5.11] - 2026-07-04

### Fixed
- Added a deterministic repository hygiene pass before implementation-heavy
  scrum sprint checkpoint commits and final pull request branch publishing.
- Removed compiled binary artifacts such as ELF, Mach-O, and PE outputs from
  generated target repositories before `git add .`.
- Cleaned copied parent prompt blocks from generated Markdown files when they
  include the observed `Parent task`, `Operating mode`, `Quality bar`, and
  `Expected output` contamination markers.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.11.

## [v1.5.10] - 2026-07-04

### Fixed
- Capped generated orchestration pull request bodies before GitHub API
  submission so long implementation-heavy scrum summaries no longer fail PR
  creation with GitHub body size validation errors.

### Changed
- Strengthened the built-in `implementation-heavy-scrum` planning gate so
  qualitative user requirements are translated into product/design acceptance
  criteria before implementation.
- Added game and UX-heavy guidance requiring a concrete differentiating
  mechanic, interaction, or content choice when the user request calls for one.
- Added artifact hygiene guidance to avoid committing parent prompt text, ARUN
  workspace archives, generated run artifacts, or compiled binaries into target
  repositories.
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.10.

## [v1.5.9] - 2026-07-04

### Changed
- Increased the built-in `implementation-heavy-scrum` template default
  `maxDuration` from `120m` to `180m` to allow roughly one hour per sprint for
  three-sprint live runs.
- Strengthened the `implementation-heavy-scrum` template and sprint-stage
  subtask prompts with acceptance criteria, reviewer-facing quality bars,
  fresh-checkout validation, residual-risk reporting, and release-blocking gap
  guidance.
- Added explicit guidance to keep frontend, backend, deployment, and
  documentation concerns separated in generated repository layouts and to avoid
  duplicated long-form documentation.
- Clarified that generated outcome documentation should be product-centered,
  emphasizing delivered behavior, user journeys, implementation decisions, and
  product gaps before command lists.
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.9.

## [v1.5.8] - 2026-07-04

### Changed
- Collapsed long orchestration task text, run descriptions, parent task
  context, and run output by default so active runs are easier to scan.
- Added a status-colored subtask timeline with dependency tags in the Runs tab
  so the execution flow and task relationships are visible.
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.8.

## [v1.5.7] - 2026-07-04

### Changed
- Increased the built-in `implementation-heavy-scrum` template default
  `maxDuration` from `60m` to `120m` so three-sprint live runs have enough
  time to complete after Sprint 2 validation and reporting.
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.7.

## [v1.5.6] - 2026-07-03

### Fixed
- Added Node.js and npm to the runtime image so live frontend and QA
  validation can execute generated package scripts during orchestration.
- Allowed frontend and QA quality gates to fall back to static smoke evidence
  only when the local JavaScript runtime or selected package manager is
  unavailable.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.6.

## [v1.5.5] - 2026-07-03

### Fixed
- Added deterministic recovery for implementation-heavy frontend validation
  failures after a canonical Go service has already been generated.
- Prevented Go-service frontend/static stage validation failures from cascading
  all remaining scrum subtasks.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.5.

## [v1.5.4] - 2026-07-03

### Fixed
- Recognized Japanese implementation-heavy scrum template wording such as
  `minimal Go HTTP server` when deciding whether Go QA recovery applies.
- Kept deterministic Go QA recovery active for Japanese UI runs that fail
  runtime QA validation despite a valid generated Go service.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.4.

## [v1.5.3] - 2026-07-03

### Fixed
- Added deterministic recovery for built-in implementation-heavy scrum analyst
  planning subtasks when runtime planner output is empty or unparsable.
- Prevented empty planning output from cascading all scrum subtasks before the
  first implementation stage can run.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.3.

## [v1.5.2] - 2026-07-03

### Fixed
- Added deterministic recovery for implementation-heavy Go QA validation
  failures so a transient or recoverable `go test`/`go vet` failure does not
  cascade through the remaining scrum subtasks.
- Repaired generated Go Dockerfiles that assume `go.sum` exists when the
  generated service has no external module dependencies.
- Added QA recovery artifacts that document local test and smoke-test commands.

### Changed
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.2.

## [v1.5.1] - 2026-07-03

### Changed
- Localized built-in scenario task-template bodies for Japanese Web UI mode.
- Localized built-in agent descriptions when the Web UI is set to Japanese.
- Refreshed the README Web UI screenshot after the ARUN production rollout.
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.1.

## [v1.5.0] - 2026-07-03

### Added
- Added user-selectable English/Japanese Web UI language support with persisted
  browser preference.
- Added Japanese built-in scenario template names, descriptions, default output
  language, and task-template output instructions.

### Changed
- Transferred the repository from `kazyamaz200/agentos` to
  `hakobune8/arun`.
- Updated release, Docker image, Helm chart, GitHub Pages chart repository, and
  public documentation defaults for the new repository location.
- Kept the Go module path, CLI name, Helm chart name, environment variables,
  state directories, and cookie names compatible with v1.4.x.
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for v1.5.0.

## [v1.4.1] - 2026-07-03

### Changed
- Tuned the built-in scrum scenario templates so stakeholder reports use the
  selected output language or the repository's usual language instead of
  forcing Japanese.
- Updated the Web UI workspace label, Helm chart `version`, chart
  `appVersion`, and default image tag for the v1.4.1 patch release.

### Fixed
- Fixed implementation-heavy scrum fallback paths so empty or sandbox
  repositories can complete app, Docker, Helm, Kubernetes, review, and report
  stages with concrete validation artifacts.
- Fixed Web UI orchestration progress counts and run status labels so
  completed subtasks count as passed even when older records do not populate a
  result `success` field.
- Improved Stage Presets display to show stage, agent, and preset explicitly.

### Notes
- v1.4.1 was verified against a private empty repository using the
  `implementation-heavy-scrum` template. The run completed 25/25 subtasks,
  created a tracking Issue and Pull Request, and validated Go, Docker, Helm,
  and Kubernetes artifacts.

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
- Added the `arun evals` orchestration evaluation suite with deterministic
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
  ARUN webhook delivery remains optional for issue-triggered workflows.
- Helm chart `version`, `appVersion`, and default `image.tag` are aligned to
  `v1.3.0`.

## [v1.2.0] - 2026-07-01

### Added
- Expanded built-in agent registry for broader repository workflows, including
  frontend, security, release, dependency, QA, and convention-aware planning
  guidance.
- Repository-defined custom agent profiles through `.arun/agents/*.yaml`.
- Reusable scenario templates, including repository-defined
  `.arun/scenarios/*.yaml` templates.
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
- GitHub-to-ARUN webhook delivery is still not required for v1.2.0 unless a
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
- GitHub-to-ARUN webhook delivery is not required for v1.1 because deployments may not be reachable from GitHub.
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
- Versioned Agent definition schema (apiVersion: arun.io/v1) (#97)
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
- CLI commands: `arun agent list/create/run`, `arun orchestrate`
- Agent template system with coder/reviewer/tester template
- Package-level Go doc comments (ongoing)

### Changed
- Profile loading uses var instead of value receiver for DefaultProfile

## [v0.4] - 2026-06-27

### Added
- MCP client (JSON-RPC stdio) with tool registration
- Docker sandbox interface stub for future isolated execution
- Web UI dashboard (`arun serve`)
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
- `arun issue`, `arun pr`, `arun checks` commands
- Auto-PR creation on `arun run --pr`
- CI Fix Agent prototype

## [v0.1] - 2026-06-24

### Added
- Initial ARUN implementation
- CLI with `run`, `review`, `version` commands
- LLM client with LiteLLM integration
- Tool system: filesystem, shell, git, search, test tools
- Safety layer: command denylist, secret detection, branch protection
- Task/profile YAML loading
- Runtime orchestration with plan/execute/review/retry lifecycle
- Run state persistence and JSONL logging
