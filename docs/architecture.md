# Architecture

## Overview

ARUN is a Go runtime for autonomous coding agents. It provides a stable
agent interface, lifecycle orchestration, tool execution, memory, eventing,
GitHub integration, and a Kubernetes-ready Web UI.

```
CLI / Web UI / REST API
        |
        v
Runtime + Server
        |
        +--> Agent Registry / Agent Factory
        |        |
        |        v
        |   runtime.Agent implementations
        |
        +--> Planner / Executor / Reviewer
        |        |
        |        v
        |   Tool Registry + Sandbox
        |
        +--> Event Bus + Run Store + Memory
        |
        v
LiteLLM / GitHub / Qdrant / MCP
```

## Entry Points

- `cmd/arun/main.go` starts the CLI.
- `internal/cli/` defines commands for run, review, issue, PR, checks, CI fix,
  search, memory, MCP, serve, agents, orchestration, guidelines, completion,
  and version output.
- `internal/server/` serves the Web UI, REST APIs, GitHub OAuth session flow,
  LLM presets, run history, and remote repository orchestration.

## Runtime

The runtime layer owns the coding run lifecycle:

1. Load task YAML.
2. Load a profile or versioned agent definition.
3. Resolve the target repository and sandbox.
4. Create or select a task branch.
5. Ask the agent to plan.
6. Execute tool-backed plan steps.
7. Run test and lint commands.
8. Retry failed execution when the profile allows it.
9. Review the resulting diff.
10. Persist artifacts and emit events.

Key packages:

- `internal/runtime/` contains run context, lifecycle configuration, and result
  types.
- `internal/agent/` contains the base agent, planner, reviewer, retry handler,
  built-in agents, and registry.
- `internal/profile/` and `internal/task/` load YAML configuration.

## Agent Registry And Factory

ARUN v1.0 supports two ways to create agents:

- Built-in registry entries: `go-backend`, `reviewer`, `ci-fixer`, `docs`,
  `security`, `release-manager`, `dependency-updater`, and `qa`.
- Versioned definitions using `apiVersion: arun.io/v1`.

`internal/factory/` converts definition YAML into runnable agents by wiring the
LLM client, tool registry, command policy, and sandbox. This keeps external
agent definitions declarative while preserving the same runtime interface used
by built-in agents.

## Tools And Sandbox

Tools are registered through `internal/tools/` and expose a stable description
and execution contract. Built-in tools cover filesystem access, search, shell,
git, and test execution.

The sandbox abstraction lives in `internal/sandbox/`. The local sandbox is the
active v1.0 backend. A Docker backend interface exists as an extension point for
future isolated execution.

## Safety

`internal/safety/` enforces command deny rules and secret-file protections.
Filesystem tools block sensitive paths such as `.env`, private keys, and PEM
files. Shell execution applies profile-defined deny commands before running.

## State, Events, And Memory

- `internal/state/` persists run state, tool logs, LLM logs, diffs, summaries,
  and PR body drafts under `ARUN_HOME`.
- `internal/event/` provides a typed event bus and JSONL file store for
  observability and replay.
- `internal/memory/` and `internal/vector/` provide local JSON and Qdrant-backed
  vector memory.
- `internal/search/` unifies search across memory, guidelines, and past PR data.

## Multi-Agent Orchestration

`internal/orchestrator/` coordinates multiple `runtime.Agent` implementations
against one task. The planner decomposes the task into subtasks, assigns each
subtask to a named agent, and executes them sequentially or in parallel.

The Web UI stores orchestration records under `ARUN_HOME/orchestrates` and
tracks per-subtask status so long-running runs can be observed before the whole
orchestration completes.

## Integrations

- `internal/llm/` talks to LiteLLM or any OpenAI-compatible endpoint.
- `internal/github/` supports issues, pull requests, check runs, and CI-fixer
  workflows.
- `internal/mcp/` connects JSON-RPC stdio MCP servers and adapts their tools
  into the ARUN tool system.
- `internal/embedding/` provides embedding requests for memory and search.

## Deployment

The Helm chart in `charts/arun/` deploys the Web UI server with persistent
storage, optional ingress, optional network policy, GitHub OAuth settings, and
administrator-defined LLM presets. Docker images are published to
`ghcr.io/hakobune8/arun`.
