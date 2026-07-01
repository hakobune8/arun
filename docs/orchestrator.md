# Multi-Agent Orchestrator

The Orchestrator coordinates multiple agents to work on a complex task.
It supports sequential and parallel execution strategies.

## Architecture

```
Task Description
       ↓
   Orchestrator.Plan()  →  LLM breaks task into subtasks
       ↓
   Orchestrator.Execute() → runs subtasks via Runtime
       ↓
   Orchestrator.MergeResults() → combined report
```

## Sequential Strategy

Subtask results (diffs) are passed as context to the next subtask.

```go
o.SetStrategy(orchestrator.StrategySequential)
```

## Parallel Strategy

Subtasks run concurrently via goroutines.

```go
o.SetStrategy(orchestrator.StrategyParallel)
```

## Usage

```go
llmClient := llm.NewLiteLLMClient(llm.DefaultConfig())
ws := sandbox.NewWorkspace(".")
agents := map[string]runtime.Agent{
    "go-backend": goBackendAgent,
    "reviewer":   reviewAgent,
}

o := orchestrator.NewOrchestrator(llmClient, ws, agents, &runtime.Config{})
plan, _ := o.Plan(ctx, "Implement user authentication")
results, _ := o.Execute(ctx, plan)
summary := o.MergeResults(results)
```

## CLI

```bash
agentos orchestrate --agents "go-backend,reviewer" --repo ./local-repo --task "..." --strategy parallel
```

## Web UI Remote Repository Workflow

The Web UI is designed around remote repository orchestration in deployed
environments:

1. Open **Orchestrate**.
2. Select a GitHub repository from the repository picker.
3. Confirm the base branch, usually the repository default branch or `main`.
4. Describe the task or apply a scenario template.
5. Load repository agents or ask AgentOS to suggest built-in specialists.
6. Choose `Sequential` or `Parallel` and start orchestration.

AgentOS clones each request into an isolated workspace under
`AGENTOS_HOME/workspaces/orchestrate`. This keeps concurrent runs against
different repositories from sharing a mutable checkout. Private GitHub
repositories require GitHub App installation credentials or a GitHub token in
the AgentOS deployment environment. The repository picker uses authenticated
GitHub repository listing and stores the selected repository as `owner/repo`.

## Scheduled Runs

The Web UI Schedules page and `/api/schedules` API can persist recurring
orchestration jobs. A schedule stores the repository, base branch, task, agents,
LLM preset, output controls, GitHub artifact settings, an interval or five-field
cron expression, timezone, concurrency policy, and execution history.

Built-in scheduled workflow templates are available from
`/api/schedules/templates` and the Schedules page. Templates provide practical
maintenance/reporting defaults such as daily failed-run reports, weekly
repository health reports, weekly security triage, weekly dependency updates,
monthly release readiness reports, and memory/guideline stale-context checks.
Each template includes recommended agents, a schedule example, expected
Markdown outputs, and permission notes. Users can still adjust the output
language, Issue/PR creation settings, and repository/default artifact templates
before saving the schedule.

AgentOS starts due schedules in-process when the Web server is running. Missed
runs after restart are checked on startup. By default, `concurrencyPolicy:
forbid` skips a due execution when the previous schedule run is still planning
or running. Each orchestration started by a schedule records `scheduleId`, and
the schedule history stores the linked run ID and latest run status.

## Task Recommendations

The Web UI can run a recommend-only pass before starting an orchestration. The
recommendation classifies the task and returns a preset, confidence, rationale,
agent set, execution strategy, whether a PR is likely appropriate, and whether a
human approval gate is recommended.

The classifier is deterministic and uses the task text plus lightweight
repository file signals. For `.` it inspects the current checkout. For GitHub
repositories it uses the same validated shallow-clone path as orchestration so
file signals such as `package.json`, `Dockerfile`, `charts/`,
`.github/workflows`, `SECURITY.md`, and lockfiles can influence routing before
the run starts.

Common presets include:

- `frontend`
- `backend`
- `ci-fix`
- `ops`
- `docs`
- `security`
- `release`
- `dependency`
- `qa`
- `reporting`
- `bugfix`

Users can apply a recommendation in the New Orchestrate form and still override
agents, strategy, and artifact choices before starting the run.

Built-in agent recommendations now include specialized repository workflow
agents:

- `frontend` for UI components, pages, responsive layout, accessibility,
  framework-aware frontend changes, and npm/pnpm/yarn/bun validation.
- `security` for dependency, auth/session, secret-handling, and
  security-sensitive changes.
- `release-manager` for changelogs, release notes, version readiness, and
  deployment checklist work.
- `dependency-updater` for Go module, lockfile, and GitHub Actions version
  updates.
- `qa` for regression tests, smoke checks, scenario coverage, and manual
  verification notes.

Recommended frontend tasks:

- Use `frontend` for React/Vite/Next.js, Vue/Nuxt, Svelte/SvelteKit, or plain
  HTML/CSS/JavaScript UI work such as components, pages, layout, responsive
  fixes, accessibility basics, and styling updates.
- Pair `frontend` with `qa` when browser smoke checks, screenshots, responsive
  validation, or manual visual verification are expected.
- Keep `go-backend` for API, service, and Go implementation work; frontend
  routing no longer falls back to the backend agent when the frontend agent is
  available.

Example task descriptions:

- `Update the React dashboard cards so they remain readable at mobile and desktop widths.`
- `Add an accessible empty state to the Vite search page and run available package scripts.`
- `Fix the Tailwind navigation layout without introducing a new component library.`

Planner prompts include structured capabilities for every selected agent:
domains, task keywords, repository files, recommended dependency order,
architecture guidance, and output expectations. This helps LLM planning choose
specialists instead of assigning subtasks from name-only descriptions. If LLM
planning fails, the fallback planner still routes common tasks by domain:
frontend work goes through the frontend application agent, QA, docs, and
review; Docker, Helm, and Kubernetes work goes through release/deployment,
security, QA, docs, and review; security, documentation, backend, dependency,
CI, and release tasks have similar deterministic dependency templates.

## Issue-Triggered Orchestration

The GitHub panel can start an orchestration from an existing GitHub Issue. Load
repository Issues in the run detail view, then use `Run` next to an Issue. The
server converts the Issue title, body, URL, and labels into the orchestration
task, stores the source Issue on the orchestration record, and applies the same
recommendation logic used by the New Orchestrate form.

Issue labels can adjust launch behavior:

- `agentos:create-pr` enables PR creation.
- `agentos:report-only` disables PR creation.
- `agentos:parallel` selects parallel orchestration.
- `agentos:sequential` selects sequential orchestration.
- `agentos:close-never` prevents automatic source Issue close.
- `agentos:close-on-quality-gate-pass` closes the source Issue after successful
  completion and passing quality gates.
- `agentos:close-on-pr-merge` records a conservative PR-merge close policy.
- `agentos:approval-required` requires human approval before closing.

Issue text can also include a slash command line:

```text
/agentos run agents=docs,reviewer strategy=parallel create_pr=false close_policy=after_human_approval approval=true
```

The slash command overrides matching label-derived controls. The server records
a trigger ID when one is provided and rejects duplicate in-flight runs for the
same source Issue, which keeps repeated label or command events from starting
parallel duplicate orchestrations.

Supported close policies are `never`, `on_pr_merge`, `on_quality_gate_pass`,
and `after_human_approval`. Webhook-based PR merge detection is intentionally
not required for v1.1; `on_pr_merge` is recorded as a conservative policy for
manual follow-up. `after_human_approval` puts a completed run into
`pending_approval` until an authorized user approves or rejects it from the Web
UI or approval API.

For Issue-sourced runs, AgentOS posts a start comment and one final status
comment back to the source Issue when GitHub write credentials are configured.
The final comment includes the run status, any PR URL, error text, and summary.
Set `AGENTOS_PUBLIC_URL` to include a stable AgentOS run link in these comments;
otherwise the run ID is shown without a public link.

## GitHub Artifacts

New orchestrations can request GitHub artifacts from the Web UI:

- `Create tracking Issue` creates an issue at the start of the orchestration.
- `Create Pull Request` creates a PR after the orchestration completes.
- `Branch name` defaults to `agentos/<run-id>`.
- `PR base branch` defaults to `main`.
- Issue and PR titles default to the task description.

The orchestration record stores the target branch, Issue URL, PR URL, and any
GitHub API error so the Web UI can show the automation outcome alongside the
run status. PR creation expects the selected head branch to exist in the remote
repository; branch push automation is tracked separately in the GitHub
automation roadmap.
