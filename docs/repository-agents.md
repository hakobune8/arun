# Repository Agents

Repository agents let a target repository provide custom AgentOS profiles
without rebuilding AgentOS. Put versioned agent definitions under:

```text
.agentos/agents/*.yaml
```

The Web UI can import these definitions from the New Orchestration form with
**Load Repository Agents**. Imported custom agents appear in the agent picker
with a `custom` tag. When an orchestration starts, AgentOS stores the selected
custom agent definitions in the orchestration record as `customAgents` so later
reviewers can reproduce which profile, tools, commands, limits, and guidance
were used.

## Validation

Repository-defined agents use the same `agentos.io/v1` Agent schema as normal
agent definitions, with additional safety checks for orchestration use:

- `metadata.name` must match `^[a-z][a-z0-9-]{1,62}$`.
- Custom agents cannot override built-in agent names.
- `spec.tools.allow` is required and may only include `read_file`,
  `write_file`, `search`, `shell`, `git`, and `test`.
- If `shell` is allowed, `spec.safety.denyCommands` is required.
- `spec.commands.test`, `spec.commands.lint`, and `spec.commands.build` reject
  unsafe command fragments such as `rm -rf`, `sudo`, `curl`, `wget`, `ssh`,
  `scp`, and privileged Docker runs.

Invalid definitions are rejected by the API with a clear `400` response before
execution starts.

## Frontend Example

```yaml
apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: frontend-app
  labels:
    role: Frontend application agent for React, Tailwind CSS, accessibility, and browser smoke checks
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
    test: npm test
    lint: npm run lint
    build: npm run build
  limits:
    maxRetries: 2
    maxIterations: 8
  guidance:
    architecture:
      - Inspect the existing frontend framework, routing, state, and component conventions before adding new UI.
      - Keep controls dense and predictable for operational tools; avoid marketing-style layout unless the repository already uses it.
    outputExpectations:
      - Frontend tests, lint, and build pass when configured.
      - UI changes include accessibility and responsive behavior notes.
```

## Security Example

```yaml
apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: repo-security
  labels:
    role: Repository-specific security review and remediation agent
spec:
  llm:
    model: coder
    temperature: 0.1
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
      - curl
      - wget
      - ssh
      - scp
  commands:
    test: go test ./...
    lint: go vet ./...
  limits:
    maxRetries: 1
    maxIterations: 6
  guidance:
    architecture:
      - Inspect auth, sessions, permissions, dependency use, and secret handling before changing code.
      - Prefer narrow defensive fixes that match existing repository patterns.
    outputExpectations:
      - Security-sensitive changes include tests or concrete manual verification steps.
      - Residual risk is documented when a finding cannot be fully remediated.
```

## Release Manager Example

```yaml
apiVersion: agentos.io/v1
kind: Agent
metadata:
  name: repo-release-manager
  labels:
    role: Repository-specific release preparation agent
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
      - git
  safety:
    denyCommands:
      - rm -rf
      - sudo
  commands:
    test: go test ./...
  limits:
    maxRetries: 1
    maxIterations: 5
  guidance:
    architecture:
      - Inspect changelog, version, Helm chart, deployment, and rollback conventions before editing release files.
      - Keep release changes explicit and avoid publishing, tagging, or deployment actions unless requested.
    outputExpectations:
      - Release notes and checklist entries are traceable to merged changes and validation commands.
      - Known gaps and rollback considerations are summarized.
```
