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
  guidance:
    architecture:
      - Preserve existing repository layout before introducing new structure.
    outputExpectations:
      - Tests and lint pass.
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
| `spec.guidance.architecture` | No | Convention-aware architecture guidance for this agent |
| `spec.guidance.outputExpectations` | No | Concrete outputs or validation that indicate useful work was done |

## Convention Guidance

Built-in agent definitions include guidance that keeps generated work aligned
with common repository conventions while preserving local structure first.
For example, `go-backend` prefers idiomatic standard-library Go for small
services and adopts existing `cmd/`, `internal/`, `pkg/`, `api/`, router, or
middleware layouts when they already exist. `ci-fixer` prefers conventional
GitHub Actions patterns such as `actions/checkout`, `actions/setup-go`, Go
caching, `go test`, and `go vet`. `docs` follows existing README and `docs/`
style, and `reviewer` flags over-engineered or convention-breaking changes.

## Loading

```go
def, err := agent.LoadDefinition("path/to/agent.yaml")
```

## Examples

See `definitions/` directory:
- `go-backend.yaml`
- `reviewer.yaml`
- `ci-fixer.yaml`
- `docs.yaml`
