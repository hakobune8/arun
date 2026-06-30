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
2. Select one or more agents.
3. Choose `Sequential` or `Parallel`.
4. Enter a repository as `owner/repo` or
   `https://github.com/owner/repo.git`.
5. Enter the base branch, usually `main`.
6. Describe the task and start orchestration.

AgentOS clones each request into an isolated workspace under
`AGENTOS_HOME/workspaces/orchestrate`. This keeps concurrent runs against
different repositories from sharing a mutable checkout. Private GitHub
repositories require GitHub App installation credentials or `GITHUB_TOKEN` in
the AgentOS deployment environment.

## Task Recommendations

The Web UI can run a recommend-only pass before starting an orchestration. The
recommendation classifies the task and returns a preset, confidence, rationale,
agent set, execution strategy, whether a PR is likely appropriate, and whether a
human approval gate is recommended.

The classifier is deterministic and uses the task text plus lightweight local
repository file signals when the repository is `.`. Remote GitHub repositories
are not cloned for recommendation; they are classified from the task text only.

Common presets include:

- `frontend`
- `backend`
- `ci-fix`
- `ops`
- `docs`
- `security`
- `dependency`
- `reporting`
- `bugfix`

Users can apply a recommendation in the New Orchestrate form and still override
agents, strategy, and artifact choices before starting the run.

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
