# Architecture

## Overview

AgentOS is a coding agent execution platform designed for safe, reproducible code generation using LLMs via LiteLLM.

```
User / CLI
     |
AgentOS CLI
     |
Agent Runtime
     |
Agent Profile
     |
Task Planner
     |
Tool Executor
     |
Review & Retry Loop
     |
Patch / Summary / PR Body Draft
```

## Components

### CLI Layer
- `cmd/agentos/main.go` — Entry point
- `internal/cli/` — Command definitions (root, agent, checks, ci-fix, guideline, issue, mcp, memory, orchestrate, pr, review, run, search, serve, version)

### Runtime Layer
- `internal/runtime/runtime.go` — Orchestrates the full run lifecycle
- `internal/runtime/context.go` — Run context carrying task, profile, tools
- `internal/runtime/result.go` — Data types for plans, results, reviews

### Agent Layer
- `internal/agent/agent.go` — Core interfaces
- `internal/agent/planner.go` — LLM-based task planning
- `internal/agent/reviewer.go` — LLM-based code review
- `internal/agent/retry.go` — Retry logic for failed steps

### LLM Layer
- `internal/llm/client.go` — LLM client interface and LiteLLM implementation
- `internal/llm/types.go` — Request/response types
- `internal/llm/litellm.go` — Environment-based configuration
- `internal/llm/prompts.go` — System prompts for planner, coder, reviewer

### Tool Layer
- `internal/tools/tool.go` — Tool interface and registry
- `internal/tools/filesystem.go` — Read/write files
- `internal/tools/shell.go` — Shell command execution with safety policy
- `internal/tools/git.go` — Git operations
- `internal/tools/test.go` — Test execution
- `internal/tools/search.go` — Code search

### Safety Layer
- `internal/safety/command_policy.go` — Command denylist enforcement
- `internal/safety/secrets.go` — Secret file detection

### State Layer
- `internal/state/run_store.go` — Run state persistence
- `internal/state/logger.go` — JSONL logging for tools and LLM calls

## Run Lifecycle

1. Load task YAML
2. Load profile YAML
3. Verify repository exists
4. Create feature branch
5. LLM generates a plan (structured JSON)
6. Execute plan steps using registered tools
7. Run tests and lint
8. Retry on failure (up to max_retries)
9. LLM reviews the diff
10. Generate summary.md and pr_body.md
11. Save all artifacts to `.agentos/runs/{task_id}/`
