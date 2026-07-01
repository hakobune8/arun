# API Reference

AgentOS Web UI provides a minimal REST API for querying run history and search results.

## Base URL

By default, the server runs at `http://localhost:8080`.

## Endpoints

### Health Check

```
GET /api/health
```

Returns the server status.

**Response** (200):
```json
{
  "status": "ok",
  "time": "2026-06-28T10:00:00Z"
}
```

---

### List Runs

```
GET /api/runs
```

Returns all completed runs.

**Response** (200):
```json
[
  {
    "id": "task-001",
    "status": "completed",
    "started_at": "2026-06-28T09:00:00Z",
    "completed_at": "2026-06-28T09:05:00Z",
    "profile": "go-backend",
    "branch": "agent/task-001"
  }
]
```

---

### Get Run Detail

```
GET /api/runs/{id}
```

Returns detailed information about a specific run, including steps, errors, and LLM call logs.

**Response** (200):
```json
{
  "id": "task-001",
  "status": "completed",
  "plan": { ... },
  "result": { ... },
  "logs": [ ... ]
}
```

---

### Search

```
GET /api/search?q={query}&source={source}&limit={limit}
GET /api/search?q={query}&repo={repo}&baseBranch={branch}&source={source}&limit={limit}
```

Search across memory, guidelines, and past PRs.

When `repo` is provided, search is scoped to that repository and branch and
returns repository context results from memory, guidelines, orchestration runs,
run artifacts, GitHub artifacts, and code/files.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| `q` | string | required | Search query |
| `source` | string | `all` | Source filter: `memory`, `guideline`, `run`, `artifact`, `github`, `code`, `pr`, `all` |
| `repo` | string | optional | Repository scope, for example `owner/repo` |
| `baseBranch` | string | `main` | Branch scope when `repo` is set |
| `limit` | int | `10` or `50` | Maximum results |

**Response** (200):
```json
[
  {
    "source": "memory",
    "content": "...",
    "score": 0.95,
    "repo": "owner/repo",
    "branch": "main",
    "runId": "run-0123456789abcdef",
    "metadata": { "status": "approved" }
  }
]
```

---

### Repository Memory

```
GET /api/repository-memory?repo={repo}&baseBranch={branch}&q={query}&status={status}&type={type}
POST /api/repository-memory
PUT /api/repository-memory/{id}
DELETE /api/repository-memory/{id}
POST /api/repository-memory/{id}/approve
```

Repository memory is scoped by repository and branch. Approved entries are used
as Orchestrate planning context; pending entries are proposed by completed runs
and require approval before reuse.

Create or update body:

```json
{
  "repo": "owner/repo",
  "baseBranch": "main",
  "type": "validation",
  "content": "Run go test ./... before opening a PR.",
  "status": "pending",
  "pinned": true
}
```

Orchestration detail responses include:

- `memoryUsed`: approved entries included during planning.
- `memoryProposals`: pending entries proposed from the completed run.

---

### Repository Guidelines

```
GET /api/repository-guidelines?repo={repo}&baseBranch={branch}&q={query}&status={status}&type={type}&agent={agent}
POST /api/repository-guidelines
PUT /api/repository-guidelines/{id}
DELETE /api/repository-guidelines/{id}
```

Repository guidelines are scoped by repository and branch. AgentOS loads
`.agentos/guidelines/*.md`, `*.yaml`, and `*.yml` from the target repository at
orchestration planning time, ranks relevant active guidelines by task and agent,
and attaches them to planned subtasks.

Create or update body:

```json
{
  "repo": "owner/repo",
  "baseBranch": "main",
  "title": "Web UI API convention",
  "type": "architecture",
  "content": "Place Web UI API handlers in internal/server.",
  "tags": ["go-backend"],
  "required": true,
  "status": "active"
}
```

`DELETE` archives the guideline by setting `status` to `archived`.

Orchestration detail responses include:

- `guidelinesUsed`: guidelines attached to planned subtasks.
- `missedRequiredGuidelines`: required guidelines that were loaded but not
  attached to any subtask.

---

### GitHub Repository Picker

```
GET /api/github/repositories
```

Returns repositories visible to the configured GitHub credentials. The Web UI
uses this endpoint to populate the New Orchestrate repository selector. GitHub
App installation repositories are preferred when installation credentials are
configured; otherwise the server falls back to repositories visible to the
configured user token.

**Response** (200):
```json
[
  {
    "id": 1,
    "name": "repo",
    "full_name": "owner/repo",
    "private": true,
    "html_url": "https://github.com/owner/repo",
    "default_branch": "main"
  }
]
```

---

### Schedules

```
GET /api/schedules
GET /api/schedules/templates
POST /api/schedules
GET /api/schedules/{id}
PUT /api/schedules/{id}
POST /api/schedules/{id}/pause
POST /api/schedules/{id}/resume
POST /api/schedules/{id}/run
```

Schedules define recurring Orchestrate jobs. They persist under
`AGENTOS_HOME/schedules`, store execution history, and link each started
execution to an orchestration record through `scheduleId`.

`GET /api/schedules/templates` returns built-in maintenance and reporting
templates with recommended agents, schedule examples, expected Markdown
outputs, and required permissions.

Create or update body:

```json
{
  "templateId": "weekly-repository-health-report",
  "name": "Weekly repository health report",
  "repo": "owner/repo",
  "baseBranch": "main",
  "task": "Create a repository health report.",
  "agents": ["analyst", "reporter"],
  "strategy": "sequential",
  "llmPreset": "staips",
  "outputLanguage": "ja",
  "schedule": {
    "type": "cron",
    "cron": "0 9 * * 1",
    "timezone": "Asia/Tokyo"
  },
  "concurrencyPolicy": "forbid",
  "github": {
    "createIssue": false,
    "createPullRequest": false
  }
}
```

Built-in template IDs include:

| ID | Recommended cadence | Agents | Output |
|---|---|---|---|
| `daily-failed-run-report` | `0 9 * * *` | `analyst`, `reporter` | Failed-run Markdown report and optional Issue |
| `weekly-repository-health-report` | `0 9 * * 1` | `analyst`, `reporter` | Repository health Markdown report and optional Issue |
| `weekly-security-triage` | `0 10 * * 1` | `security`, `analyst`, `reporter` | Security triage Markdown report and optional Issue |
| `weekly-dependency-update` | `0 11 * * 1` | `dependency-updater`, `ci-fixer`, `reviewer` | Dependency update PR or no-change report |
| `monthly-release-readiness` | `0 9 1 * *` | `release-manager`, `analyst`, `reporter` | Release readiness Markdown report and optional Issue |
| `memory-guideline-stale-check` | `0 9 * * 5` | `analyst`, `reporter` | Stale-context Markdown report and cleanup recommendations |

For interval schedules, use:

```json
{
  "schedule": {
    "type": "interval",
    "interval": "24h",
    "timezone": "UTC"
  }
}
```

`concurrencyPolicy` is `forbid` by default and skips a due execution when the
previous schedule run is still planning or running. `POST /run` manually starts
the same saved configuration and records the execution in schedule history.

---

## Authentication

Authentication is optional. Local development can run without login. Production
deployments can require GitHub OAuth sessions by setting
`AGENTOS_AUTH_REQUIRED=true` or Helm `auth.required=true`.

```
GET /api/auth/session
GET /auth/login
GET /auth/callback
GET /auth/logout
```

When authentication is enabled, work-triggering APIs require a valid signed
session cookie. Repository cloning and GitHub API operations still use the
server-side GitHub token or GitHub App installation credentials.

Issue-sourced orchestrations can require human approval before closing their
source Issue:

```http
POST /api/orchestrates/{id}/approval
Content-Type: application/json

{"action":"approve","reason":"optional note"}
```

Use `{"action":"reject","reason":"..."}` to reject a pending approval. Approval
actions require the same automation authorization controls as other sensitive
orchestration actions.

## Rate Limiting

No rate limiting is currently implemented. Consider using a reverse proxy for production deployments.
