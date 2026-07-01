# Configuration

## LiteLLM

AgentOS uses LiteLLM as the LLM gateway. You can start LiteLLM with your preferred providers:

```bash
litellm --model gpt-4 --api_key $OPENAI_API_KEY
# or with a config file
litellm --config litellm_config.yaml
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `LITELLM_API_KEY` | `""` | API key for LiteLLM server |
| `LITELLM_BASE_URL` | `http://localhost:4000` | LiteLLM server URL |
| `AGENTOS_MODEL_CODER` | `coder` | Model alias for coding tasks |
| `AGENTOS_MODEL_EMBEDDING` | `text-embedding-ada-002` | Model alias for embeddings |
| `AGENTOS_LLM_DEFAULT_PRESET` | `default` | Default Web UI model preset ID |
| `AGENTOS_LLM_PRESETS` | generated default | JSON array of server-side LLM presets |
| `AGENTOS_ORCHESTRATE_SUBTASK_TIMEOUT` | `10m` | Maximum runtime for one orchestration subtask |
| `AGENTOS_ORCHESTRATE_PLAN_TIMEOUT` | `90s` | Maximum runtime for orchestration planning |

### Governance Limits

Web UI and API orchestrations accept optional `limits` values. These are stored
on each orchestration record and copied from schedules to scheduled runs:

```json
{
  "maxDuration": "30m",
  "maxSubtasks": 12,
  "maxRetries": 3,
  "maxLlmTokens": 200000,
  "maxGitHubRequests": 100,
  "maxConcurrentRepoRuns": 1,
  "maxConcurrentOrgRuns": 4
}
```

When omitted, AgentOS uses `30m` duration, `12` subtasks, and one active run per
repository. The server enforces maximum duration, planned subtasks, repository
concurrency, and organization concurrency. Token and GitHub request budgets are
reported in run detail as governance metadata until provider/request clients
expose reliable usage accounting.

### Web UI Model Presets

The Web UI never receives API key values. It reads public preset metadata from
`/api/settings/llm` and sends only the selected preset ID when starting an
orchestration or scoped agent run.

Example `AGENTOS_LLM_PRESETS`:

```json
[
  {
    "id": "staips",
    "name": "STAIPS coder",
    "provider": "litellm",
    "baseUrl": "http://staips-litellm.staips-edge.svc.cluster.local",
    "model": "staips-chat",
    "apiKeyEnv": "LITELLM_API_KEY"
  }
]
```

---

## Qdrant Vector Store

By default, AgentOS uses a local JSON-based vector store. To use Qdrant for production-scale vector search:

### Setup

```bash
# Start Qdrant with Docker
docker run -d --name qdrant -p 6333:6333 qdrant/qdrant
```

### Configuration

Set the `QDRANT_URL` environment variable:

```bash
export QDRANT_URL=http://localhost:6333
```

When `QDRANT_URL` is set, AgentOS will automatically use the Qdrant client instead of the local JSON store.

Qdrant collections are created automatically by AgentOS. No manual schema setup is required.

---

## Docker Sandbox

AgentOS has a sandbox interface, but the Docker backend is currently a stub and
is not available in v1.0. Use the local backend only for trusted repositories and
run AgentOS inside a separately isolated environment when executing untrusted
code.

The intended future configuration shape is:

```yaml
sandbox:
  backend: docker
  image: "custom-image:latest"
```

---

## MCP (Model Context Protocol)

AgentOS supports connecting to MCP servers to extend the available tools.

### Configuration

MCP servers are registered via the CLI:

```bash
# Register an MCP server
agentos mcp register my-server --command "python" --args "-m", "my_mcp_server"

# List registered servers
agentos mcp list

# Call a tool from an MCP server
agentos mcp call my-server tool_name '{"arg": "value"}'
```

### Environment Variables

MCP server commands inherit the AgentOS environment variables, including `LITELLM_API_KEY`, `OPENAI_API_KEY`, etc.

---

## GitHub Integration

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `GITHUB_TOKEN` | `""` | GitHub personal access token fallback for issue/PR/checks operations |
| `GITHUB_APP_ID` | `""` | GitHub App ID for installation-token authentication |
| `GITHUB_APP_INSTALLATION_ID` | `""` | GitHub App installation ID used to create installation access tokens |
| `GITHUB_APP_PRIVATE_KEY` | `""` | GitHub App private key PEM, usually provided from a Kubernetes Secret |
| `GITHUB_APP_PRIVATE_KEY_FILE` | `""` | Path to a GitHub App private key PEM file |
| `AGENTOS_PUBLIC_URL` | `""` | Public AgentOS base URL used in GitHub Issue comments |
| `AGENTOS_AUTH_REQUIRED` | `false` | Require signed GitHub sessions for work APIs |
| `AGENTOS_ADMIN_USERS` | `""` | Optional comma-separated GitHub logins allowed to run sensitive automation and view audit history |
| `GITHUB_OAUTH_CLIENT_ID` | `""` | GitHub OAuth App client ID |
| `GITHUB_OAUTH_CLIENT_SECRET` | `""` | GitHub OAuth App client secret |
| `GITHUB_OAUTH_CALLBACK_URL` | `""` | OAuth callback URL, for example `/auth/callback` on the public host |
| `AGENTOS_SESSION_SECRET` | `""` | HMAC secret for signed Web UI session cookies |

When `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, and either
`GITHUB_APP_PRIVATE_KEY` or `GITHUB_APP_PRIVATE_KEY_FILE` are set, AgentOS uses
a GitHub App installation token for API and HTTPS Git access. Otherwise it falls
back to `GITHUB_TOKEN`/`GH_TOKEN`.

The fallback personal access token requires the following scopes:
- `repo` — for private repositories
- `public_repo` — for public repositories only

GitHub login uses OAuth `read:user` to identify the UI user. Repository cloning
and GitHub API access use server-side GitHub App or token credentials; user
scoped repository credentials are a later auth-aware extension.

### Artifact Language and Templates

The Web UI can set an output language per orchestration. Repositories can also
define defaults in `.agentos/config.yaml`; explicit Web UI choices override
repository defaults.

Supported built-in languages are:

- `en` for English
- `ja` for Japanese

Repository templates use Go `text/template` syntax. Available fields include
`RunID`, `Repository`, `BaseBranch`, `TargetBranch`, `PRBase`, `Strategy`,
`Agents`, `Task`, `Summary`, and `IssueURL`.

Example `.agentos/config.yaml`:

```yaml
outputLanguage: ja
templates:
  issue:
    body: |
      AgentOS Orchestrate により作成されました。

      - Run: `{{.RunID}}`
      - Repository: `{{.Repository}}`
      - Target branch: `{{.TargetBranch}}`

      ## タスク

      {{.Task}}
  pullRequest:
    body: |
      AgentOS Orchestrate により作成されました。

      {{if .IssueURL}}Tracking issue: {{.IssueURL}}

      {{end}}## 概要

      {{.Summary}}
```

---

## Agent Templates

Multi-agent teams are defined in YAML template files:

```yaml
schema: "agentos/v1"
agents:
  - name: "coder"
    role: "Coding agent"
    model: "coder"
    tools:
      - read_file
      - write_file
      - shell
      - git
      - test
coordination:
  strategy: "sequential"
```

See [profiles/agents/template.yaml](../profiles/agents/template.yaml) for a complete example.
