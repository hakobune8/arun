# Agent Definitions

AgentOS supports a versioned, declarative YAML format for defining agents.
The runtime can instantiate agents directly from definition files — no Go
code required.

## Schema

```yaml
apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: my-agent
  labels:
    role: backend
spec:
  llm:
    model: coder
    temperature: 0.2
    maxTokens: 8192
  tools:
    allow:
      - read_file
      - write_file
      - search
      - shell
      - git
      - test
  safety:
    denyCommands:
      - rm -rf
      - sudo
  commands:
    test: go test ./...
    lint: go vet ./...
    build: go build ./...
  limits:
    maxRetries: 3
    maxIterations: 8
```

## Fields

| Field | Required | Description |
|-------|----------|-------------|
| `apiVersion` | Yes | Must be `"agentos.io/v1"` |
| `kind` | Yes | Must be `"Agent"` |
| `metadata.name` | Yes | Agent name |
| `spec.llm.model` | Yes | LLM model name |
| `spec.tools.allow` | No | Allowed tool names (empty = all) |
| `spec.safety.denyCommands` | No | Denied shell commands |
| `spec.commands` | No | Custom test/lint/build commands |
| `spec.limits` | No | Max retries and iterations |

## Loading

```go
def, err := agent.LoadDefinition("path/to/agent.yaml")
```

## Examples

See `definitions/` directory:
- `go-backend.yaml`
- `reviewer.yaml`
- `ci-fixer.yaml`
