# Quick Start

This guide walks through setting up AgentOS and running your first
coding task end-to-end.

## Prerequisites

- Go 1.22+ (for building from source)
- A running [LiteLLM](https://litellm.vercel.app) proxy (or any OpenAI-compatible API)

## 1. Install

### From Source

```bash
git clone https://github.com/kazyamaz200/agentos.git
cd agentos
go build -o agentos ./cmd/agentos/
./agentos version
```

### From Release

Download the latest binary from the
[Releases page](https://github.com/kazyamaz200/agentos/releases) for your
platform.

## 2. Start LiteLLM

AgentOS requires an OpenAI-compatible LLM API. The simplest way is to
start a LiteLLM proxy:

```bash
pip install litellm
litellm --model gpt-4o --port 4000
```

Or use any OpenAI-compatible endpoint:

```bash
export LITELLM_BASE_URL=http://localhost:4000
export LITELLM_API_KEY=sk-local
export AGENTOS_MODEL_CODER=coder
```

## 3. Create a Task

Create a file called `task.yaml`:

```yaml
id: "hello-agentos"
type: "issue_to_patch"
repo: "./my-project"
base_branch: "main"
branch: "agent/hello"
title: "Add greeting function"
description: |
  Add a function `Greet(name string) string` that returns
  a greeting message. Do not break existing tests.
```

Replace `repo` with the path to a local Git repository.

## 4. Choose a Profile

AgentOS ships with built-in profiles. For a Go project, use:

```bash
agentos run \
  --task task.yaml \
  --profile profiles/go_backend.yaml
```

Or use a definition file (v1.0 format):

```bash
agentos run \
  --task task.yaml \
  --definition definitions/go-backend.yaml
```

## 5. View Results

After the run completes, artifacts are saved to
`~/.agentos/runs/<task-id>/`:

| File | Description |
|------|-------------|
| `plan.json` | The execution plan |
| `diff.patch` | Code changes as a unified diff |
| `test.log` | Test output (if tests were run) |
| `summary.md` | Human-readable run summary |
| `pr_body.md` | PR description (ready to paste) |

## 6. Next Steps

- Browse the [CLI reference](README.md#quick-start) for more commands
- Read the [Agent Definitions](agent-definitions.md) guide
- Learn about [multi-agent orchestration](orchestrator.md)
- Set up the [Web UI](api.md) with `agentos serve --port 8080`
