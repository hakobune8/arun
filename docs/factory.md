# Agent Factory

The Factory creates fully wired `runtime.Agent` instances from versioned
Agent Definitions. No manual LLM client, tool registry, or sandbox setup
is required.

## CreateFromDefinition

```go
f := factory.NewFactory(workDir)
agt, llmClient, err := f.CreateFromDefinition(def)
```

Returns:
- `runtime.Agent` — ready to run
- `llm.LLMClient` — the configured LLM client
- `error` — any creation error

## BuildAgentFromDefinition

Convenience function that loads a Definition YAML and creates an agent:

```go
agt, llmClient, err := factory.BuildAgentFromDefinition("def.yaml", "/path/to/repo")
```

## What the Factory Does

1. Loads and validates the Definition YAML
2. Creates an LLM client with the configured model
3. Creates a safety policy from deny commands
4. Creates a tool registry with allowed tools
5. Creates a BaseAgent wired to the LLM client
6. Returns the ready-to-use agent

## Integration with Runtime

```go
agt, llmClient, _ := factory.BuildAgentFromDefinition("definitions/go-backend.yaml", ".")
ws := sandbox.NewWorkspace(".")
cfg := &runtime.Config{DryRun: false}
rt := runtime.NewRuntime(llmClient, prof, ws, cfg, agt)
rt.Run(ctx, task)
```
