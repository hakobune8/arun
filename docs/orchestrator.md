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
4. Enter a repository as `owner/repo`, `https://github.com/owner/repo.git`,
   or a local path such as `/workspace/scenario-repo`.
5. Enter the base branch, usually `main`.
6. Describe the task and start orchestration.

For remote repositories, AgentOS clones each request into an isolated workspace
under `AGENTOS_HOME/workspaces/orchestrate`. This keeps concurrent runs against
different repositories from sharing a mutable checkout. Private GitHub
repositories require `GITHUB_TOKEN` in the AgentOS deployment environment.
