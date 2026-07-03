# Upgrade To v1.0

This guide summarizes the changes users should account for when moving from
ARUN v0.x to v1.0.

## Stable Runtime Surface

v1.0 treats the `runtime.Agent` interface as the stable extension point for
agent implementations. Agents should implement planning, execution, and review
through the runtime interfaces rather than depending on older profile-only
helpers.

## Agent Definitions

New projects should prefer versioned agent definitions:

```yaml
apiVersion: arun.io/v1
kind: AgentDefinition
metadata:
  name: go-backend
spec:
  role: Go backend coding agent
  model: coder
  tools:
    - read_file
    - write_file
    - search
    - shell
    - git
    - test
```

Run definitions with:

```bash
arun run --task task.yaml --definition definitions/go-backend.yaml
```

Profile YAML remains supported for existing users:

```bash
arun run --task task.yaml --profile profiles/go_backend.yaml
```

## State Directory

Run artifacts continue to use `ARUN_HOME`, defaulting to `~/.arun`.
v1.0 also stores Web UI orchestration records under:

```text
${ARUN_HOME}/orchestrates
${ARUN_HOME}/workspaces/orchestrate
```

For Kubernetes deployments, keep persistence enabled before upgrading so run
history, orchestration records, and local vector indexes survive pod restarts.

## LLM Configuration

The default LiteLLM environment variables remain:

```bash
export LITELLM_BASE_URL=http://localhost:4000
export LITELLM_API_KEY=sk-local
export ARUN_MODEL_CODER=coder
```

For the Web UI, administrators can expose selectable presets through Helm
`llm.presets`. Browser clients only receive public preset metadata; API keys
stay in server-side environment variables or Kubernetes Secrets.

## Web UI And Authentication

The v1.0 Web UI supports GitHub OAuth sessions. Authentication is disabled by
default for local development and can be required in Kubernetes with:

```bash
helm upgrade --install arun arun/arun \
  --namespace arun \
  --set auth.required=true \
  --set auth.github.clientId=<oauth-client-id> \
  --set auth.github.callbackUrl=https://arun.example.com/auth/callback \
  --set secrets.existingSecret=arun-secrets
```

Session cookies are always marked `Secure`, so authenticated deployments should
be served over HTTPS.

## Multi-Agent Orchestration

v1.0 includes sequential and parallel orchestration over built-in or
definition-backed agents:

```bash
arun orchestrate \
  --agents "go-backend,reviewer,docs" \
  --strategy parallel \
  --repo . \
  --task "Add a health endpoint, tests, and documentation"
```

The deployed Web UI is designed around remote GitHub repositories. Use
`https://github.com/owner/repo.git` or `owner/repo` and configure
`GITHUB_TOKEN` for private repositories.

## Helm Chart

The chart repository is published at:

```bash
helm repo add arun https://hakobune8.github.io/arun
helm repo update
helm upgrade --install arun arun/arun \
  --namespace arun --create-namespace \
  --set env.LITELLM_BASE_URL=http://litellm:4000 \
  --set secrets.litellmApiKey=<litellm-api-key>
```

When changing chart files for a new release, update both `version` and
`appVersion` in `charts/arun/Chart.yaml`.

## CLI Completion

v1.0 adds shell completion generation:

```bash
arun completion bash
arun completion zsh
arun completion fish
arun completion powershell
```
