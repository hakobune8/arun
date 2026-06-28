# AgentOS

AgentOS is a coding agent execution platform for safely producing and running coding agents at scale. It uses [LiteLLM](https://github.com/BerriAI/litellm) as the LLM gateway, providing a unified interface to various LLM backends.

## Features

- **Task Planning** — LLM generates structured execution plans from task descriptions
- **Tool Execution** — Read, write, search, shell, git, and test tools
- **Review & Retry** — Automated code review with retry on test/lint failure
- **GitHub Integration** — Issue fetching, PR creation, CI check inspection
- **CI Fix Agent** — Automatic analysis and fix suggestions for CI failures
- **Safety First** — Command denylist, secret detection, main branch protection
- **Full Audit Trail** — All LLM calls, tool executions, and artifacts saved per run
- **Extensible** — Interface-based design for tools, LLM clients, and agents

## Requirements

- Go 1.22+
- [LiteLLM](https://github.com/BerriAI/litellm) proxy running and accessible

## Installation

```bash
git clone https://github.com/kazyamaz200/agentos.git
cd agentos
go build -o agentos ./cmd/agentos
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
export AGENTOS_MODEL_CODER=coder
```

## Quick Start

```bash
# Run a coding task
agentos run --task examples/task.issue.yaml --profile profiles/go_backend.yaml

# Run and create a PR
agentos run --task examples/task.issue.yaml --profile profiles/go_backend.yaml --pr --pr-repo owner/repo

# Review code changes
agentos review --repo ./my-project --profile profiles/reviewer.yaml

# GitHub Issue operations
agentos issue list --repo owner/repo
agentos issue fetch 42 --repo owner/repo

# GitHub Pull Request operations
agentos pr list --repo owner/repo
agentos pr create --repo owner/repo --title "Fix bug" --head agent/fix --body "PR description"

# CI check operations
agentos checks list --repo owner/repo --ref main

# CI Fix Agent
agentos ci-fix --repo owner/repo --ref main

# Check version
agentos version
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

Each run saves to `.agentos/runs/{task_id}/`:

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
- **Secret detection**: `.env`, `*.pem`, `id_rsa*`, `id_ed25519*` are never read
- **Branch protection**: Direct changes to `main` are prohibited
- **File limits**: Maximum changed files enforced per run
- **Retry limits**: Automatic retry with configurable maximum

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LITELLM_BASE_URL` | `http://localhost:4000` | LiteLLM proxy URL |
| `LITELLM_API_KEY` | `sk-local` | API key for LiteLLM |
| `AGENTOS_MODEL_CODER` | `coder` | Model for coding tasks |
| `GITHUB_TOKEN` | - | GitHub personal access token for API operations |
| `GH_TOKEN` | - | Alternative GitHub token (fallback) |

## Roadmap

### v0.1 — Done
- [x] CLI with run, review, version commands
- [x] Task YAML loading
- [x] Profile YAML loading
- [x] LLM-based planning
- [x] Tool execution (file, search, shell, git, test)
- [x] Test/lint with retry
- [x] Run artifact persistence
- [x] Safety policies

### v0.2 — Current
- [x] GitHub Issue fetching
- [x] Pull Request creation
- [x] CI result fetching
- [x] CI Fix Agent

### v0.3 — Planned
- [ ] Qdrant integration
- [ ] Past PR search
- [ ] Coding guideline retrieval
- [ ] Agent memory

### v0.4 — Planned
- [ ] MCP integration
- [ ] Docker sandbox
- [ ] Web UI

### v0.5 — Planned
- [ ] Agent Factory
- [ ] Profile-based agent generation
- [ ] Multi-agent coordination

## License

Apache-2.0
