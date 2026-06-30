# Profiles

Profiles define the behavior of an AgentOS agent. They are YAML files that specify the LLM configuration, available tools, commands, and limits.

## Structure

```yaml
name: "agent-name"           # Unique identifier
role: "description"          # Role description for the LLM

llm:
  provider: "litellm"        # LLM provider (currently only litellm)
  model: "coder"             # Model name mapped to LiteLLM
  temperature: 0.2           # Lower = more deterministic
  max_tokens: 8192           # Maximum response tokens

tools:
  allow:                     # Allowed tools
    - read_file
    - write_file
    - search
    - shell
    - git
    - test
  deny_commands:             # Denied shell command patterns
    - "rm -rf"
    - "sudo"

commands:                    # Custom commands for the target project
  test: "go test ./..."
  lint: "go vet ./..."
  build: "go build ./..."

limits:
  max_iterations: 8          # Max plan steps
  max_retries: 3             # Retries on test/lint failure
  max_changed_files: 20      # Safety limit on files modified
  max_runtime_minutes: 30    # Max wall-clock time

output:
  mode: "patch"              # Output mode: patch, summary, pr

guidance:
  architecture:              # Convention-aware behavior for planning/execution
    - "Preserve existing repository layout"
  output_expectations:       # Useful-work and validation expectations
    - "Tests pass"
```

## Built-in Profiles

- `profiles/go_backend.yaml` — Go backend coding agent
- `profiles/reviewer.yaml` — Code review agent
- `profiles/ci_fixer.yaml` — CI configuration fix agent
- `profiles/docs.yaml` — Documentation agent

Built-in profiles include convention-aware guidance. Agents should inspect the
target repository first, preserve clear local structure, and only introduce new
frameworks, dependencies, or top-level layouts when the task requires them.
