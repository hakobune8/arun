# ARUN

[![CI](https://github.com/hakobune8/arun/actions/workflows/ci.yml/badge.svg)](https://github.com/hakobune8/arun/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.22-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/hakobune8/arun)](https://goreportcard.com/report/github.com/hakobune8/arun)
[![Release](https://img.shields.io/github/v/release/hakobune8/arun)](https://github.com/hakobune8/arun/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/hakobune8/arun)](https://pkg.go.dev/github.com/hakobune8/arun)

**A Go runtime for autonomous coding agents.**

> Define agents. Run agents. Scale agents.

*Write Agents by defining them, not by implementing them.*

ARUN is not another coding agent — it is the operating system layer for autonomous coding agents. It provides a runtime, lifecycle, execution model, tool system, memory abstraction, and safety model. It uses [LiteLLM](https://github.com/BerriAI/litellm) as the LLM gateway, providing a unified interface to various LLM backends.

ARUN is model-agnostic. For repeatable open-weight validation, the current
recommended validation model is Qwen3.6-35B-A3B; see
[Configuration](docs/configuration.md#validation-model-guidance).

Designed for Kubernetes deployment. Manage runs, review diffs, and
search across agents through the [Web UI](docs/api.md).

![ARUN Web UI orchestrates screen](docs/images/arun-webui-orchestrates.jpg)

```bash
helm repo add arun https://hakobune8.github.io/arun
helm install arun arun/arun \
  --set env.LITELLM_BASE_URL=http://litellm:4000
```

## Features

- **Task Planning** — LLM generates structured execution plans from task descriptions
- **Tool Execution** — Read, write, search, shell, git, and test tools
- **Review & Retry** — Automated code review with retry on test/lint failure
- **GitHub Automation** — Issue-triggered runs, PR creation, source issue comments, close policies, and approval gates
- **GitHub App Tokens** — Installation-token support for repository write operations
- **Specialized Built-In Agents** — Backend, frontend, CI, docs, security, release, dependency, QA, Docker, Helm, Kubernetes, DevOps, analyst, reporter, and review workflows
- **Repository Agents** — Load safe custom agent profiles from `.arun/agents/*.yaml`
- **Scenario Templates** — Apply built-in and repository `.arun/scenarios/*.yaml` orchestration templates
- **Vector Search** — Local (JSON) or Qdrant vector store for semantic search
- **Agent Memory** — Persistent memory with vector-based retrieval
- **Repository Memory** — Approved repository-scoped lessons reused during planning
- **Coding Guidelines** — YAML-defined guidelines with semantic search
- **Repository Guidelines** — Branch-scoped rules loaded from repository files or the Web UI
- **Past PR Search** — Search across previous runs and PRs
- **Storage Retention** — Web UI/API policy, usage reporting, dry-run cleanup, archive-before-delete, and safe skips for active or GitHub-linked runs
- **MCP Integration** — Connect to MCP servers for external tools
- **Sandbox Interface** — Local execution today, Docker backend planned
- **Web UI** — React/Tailwind dashboard for orchestration, agents, audit, GitHub, memory, guidelines, and repository context search
- **Agent Factory** — Create agents dynamically from profile templates
- **Multi-Agent Orchestration** — Coordinate multiple agents on complex tasks
- **Quality Gates** — Validate expected outputs, tests, lint, diffs, and generated artifacts before completion
- **Safety First** — Command denylist, secret redaction, RBAC, audit logs, and main branch protection
- **Full Audit Trail** — All LLM calls, tool executions, and artifacts saved per run
- **Extensible** — Interface-based design for tools, LLM clients, and agents

## Requirements

- Go 1.22+
- [LiteLLM](https://github.com/BerriAI/litellm) proxy running and accessible

## Installation

```bash
git clone https://github.com/hakobune8/arun.git
cd arun
go build -o arun ./cmd/arun
```

## Setup

### 1. Start LiteLLM

```bash
pip install litellm
litellm --model ollama/codellama --port 4000
# Or any OpenAI-compatible model
```

### 2. Set Environment Variables

```bash
export LITELLM_BASE_URL=http://localhost:4000
export LITELLM_API_KEY=sk-local
export ARUN_MODEL_CODER=coder
```

## Quick Start

See the [Quick Start Guide](docs/quickstart.md) for a step-by-step walkthrough.

### CLI Reference

```bash
# Deploy on Kubernetes
helm install arun arun/arun \
  --set env.LITELLM_BASE_URL=http://litellm:4000

# Run a coding task
arun run --task task.yaml --profile profiles/go_backend.yaml

# Run using a definition file (v1.0 format)
arun run --task task.yaml --definition definitions/go-backend.yaml

# Start Web UI (local dev)
arun serve --port 8080

# List registered agents
arun agent list

# Multi-agent orchestration
arun orchestrate \
  --agents "go-backend,reviewer,docs" \
  --strategy parallel \
  --repo . \
  --task "Implement user auth, tests, and documentation"

# Start an issue-sourced orchestration from the Web/API
# Supports close policies such as never, on_quality_gate_pass,
# on_pr_merge, and after_human_approval.

# Shell completion
arun completion zsh

# GitHub operations
arun issue list --repo owner/repo

# View version
arun version
```

## Task YAML

```yaml
id: "task-001"
type: "issue_to_patch"
repo: "./my-repo"
base_branch: "main"
branch: "agent/task-001"
title: "Add input validation to API"
description: |
  Add input validation to the users API.
  Do not break existing tests.
```

## Profile YAML

```yaml
name: "go-backend-agent"
role: "Go backend coding agent"

llm:
  provider: "litellm"
  model: "coder"
  temperature: 0.2
  max_tokens: 8192

tools:
  allow:
    - read_file
    - write_file
    - search
    - shell
    - git
    - test
  deny_commands:
    - "rm -rf"
    - "sudo"
    - "curl"

commands:
  test: "go test ./..."
  lint: "go vet ./..."

limits:
  max_iterations: 8
  max_retries: 3
  max_changed_files: 20
  max_runtime_minutes: 30
```

## Run Artifacts

Each run saves to `${ARUN_HOME}/runs/{task_id}/`. If `ARUN_HOME` is not set, ARUN uses `~/.arun`.

```
task.yaml         # Original task
profile.yaml      # Original profile
plan.json         # LLM-generated plan
tool_log.jsonl    # All tool executions
llm_log.jsonl     # All LLM API calls
test.log          # Test output
lint.log          # Lint output
diff.patch        # Git diff of changes
summary.md        # Run summary
pr_body.md        # Pull request body draft
```

## Safety

- **Command denylist**: `rm -rf`, `sudo`, `curl`, `wget`, `ssh`, `scp` are denied by default
- **Secret detection**: `.env`, `*.pem`, `id_rsa*`, `id_ed25519*` are blocked by filesystem tools
- **Branch handling**: Runs create and work on the task branch when possible
- **Governance limits**: Orchestrations and schedules support maximum duration, subtasks, retries, repository concurrency, organization concurrency, LLM token budgets, and GitHub request budgets. Duration, subtask, and concurrency limits are enforced by the server.
- **Retry limits**: Agent profiles and orchestration governance metadata expose configurable retry limits

## Documentation

- [Quick Start](docs/quickstart.md) — Get up and running in 5 minutes
- [Deployment](docs/deployment.md) — Kubernetes deployment via Helm
- [Product Rename and Repository Transfer Plan](docs/product-rename-migration-plan.md) — v1.5.0 transfer scope, rename risks, compatibility, and staged migration
- [Pre-merge Verification](docs/pre-merge-verification.md) — PR image checks with registry and BuildKit
- [Architecture](docs/architecture.md) — System architecture overview
- [Configuration](docs/configuration.md) — LiteLLM, Qdrant, Docker, MCP, templates
- [Upgrade to v1.4](docs/upgrade-v1.4.md) — Governance limits, storage retention, cleanup, and orchestration evals
- [Upgrade to v1.3](docs/upgrade-v1.3.md) — Scheduled operations, reporting, notifications, and ops agents
- [Profiles](docs/profiles.md) — Profile YAML schema reference
- [Agent Definitions](docs/agent-definitions.md) — Versioned Agent YAML format (arun.io/v1)
- [Repository Agents](docs/repository-agents.md) — Custom `.arun/agents/*.yaml` profiles for target repositories
- [Scenario Templates](docs/scenario-templates.md) — Reusable Orchestrate templates and repository `.arun/scenarios/*.yaml`
- [Safety](docs/safety.md) — Safety mechanisms and command policies
- [Event Bus](docs/event-bus.md) — Structured events for observability
- [Factory](docs/factory.md) — Creating agents from definitions
- [Memory](docs/memory.md) — Pluggable memory backends (vector, JSON)
- [Sandbox](docs/sandbox.md) — Execution isolation (local, Docker)
- [Orchestrator](docs/orchestrator.md) — Multi-agent coordination
- [Orchestration Evals](docs/orchestration-evals.md) — Deterministic and opt-in live scenario evaluation reports
- [Embedding](docs/embedding.md) — LLM embedding service
- [Search](docs/search.md) — Unified search across sources
- [Guidelines](docs/guidelines.md) — Coding guidelines management
- [MCP](docs/mcp.md) — Model Context Protocol integration
- [API](docs/api.md) — REST API reference for web UI
- [React Web UI Migration](docs/webui-react-tailwind-migration.md) — Web UI implementation notes

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LITELLM_BASE_URL` | `http://localhost:4000` | LiteLLM proxy URL |
| `LITELLM_API_KEY` | `sk-local` | API key for LiteLLM |
| `ARUN_MODEL_CODER` | `coder` | Model for coding tasks |
| `ARUN_HOME` | `~/.arun` | State directory for run artifacts and local vector indexes |
| `GITHUB_TOKEN` | - | GitHub personal access token for API operations |
| `GH_TOKEN` | - | Alternative GitHub token (fallback) |
| `ARUN_MODEL_EMBEDDING` | `text-embedding-ada-002` | Model for embeddings |
| `QDRANT_URL` | `http://localhost:6333` | Qdrant vector database URL |
| `QDRANT_API_KEY` | - | Qdrant API key |
| `ARUN_AUTH_REQUIRED` | `false` | Require GitHub login for work-triggering APIs |
| `ARUN_ORCHESTRATE_SUBTASK_TIMEOUT` | `10m` | Timeout for each orchestration subtask |
| `ARUN_ORCHESTRATE_PLAN_TIMEOUT` | `90s` | Timeout for orchestration planning |
| `GITHUB_OAUTH_CLIENT_ID` | - | GitHub OAuth App client ID |
| `GITHUB_OAUTH_CLIENT_SECRET` | - | GitHub OAuth App client secret |
| `GITHUB_OAUTH_CALLBACK_URL` | - | GitHub OAuth callback URL |

## Release Notes

The Helm chart workflow skips already published chart versions. Before a
release that changes `charts/arun/**`, update both `version` and
`appVersion` in `charts/arun/Chart.yaml` intentionally so chart-releaser can
publish a new immutable chart release.

- [v1.6.0 release notes](docs/release-v1.6.0.md) — Rerun reliability,
  interrupted orchestration recovery, and device-flow audit evidence.
- [v1.5.0 release notes](docs/release-v1.5.0.md) — repository transfer to
  `hakobune8/arun` and Japanese Web UI language support.
- [v1.4.1 release notes](docs/release-v1.4.1.md) — Web UI polish,
  implementation-heavy scrum hardening, and changes since v1.4.0.

## Roadmap

### Near Term
- [x] Repository transfer and ARUN rename foundation.
- [x] Implementation-heavy scrum generation hardening for mergeable PRs.
- [x] Device-flow login and operational E2E eval foundations.
- [x] Interrupted orchestration recovery after server restarts.
- [ ] Continue staged rename cleanup beyond public surfaces.
- [ ] Promote proven eval suites and LiteLLM presets into release gates.
- [ ] Continue release automation, deployment safety, and rollback hardening.

### Completed Foundations
- [x] GitHub issue/PR automation, quality gates, audit logging, and run UX.
- [x] Repository memory, guidelines, custom agents, and reusable scenarios.
- [x] Scheduled operations, notifications, storage cleanup, and governance limits.
- [x] Docker, Helm, Kubernetes, frontend, docs, QA, reviewer, and reporting agents.
- [x] Web UI, Helm chart, CLI, eval suite, and release workflow foundations.

## License

Apache-2.0
