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
agentos orchestrate --agents "go-backend,reviewer" --task "..." --strategy parallel
```
