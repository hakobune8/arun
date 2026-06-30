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
GET /api/search?q={query}&type={type}&limit={limit}
```

Search across memory, guidelines, and past PRs.

**Parameters:**
| Name | Type | Default | Description |
|---|---|---|---|
| `q` | string | required | Search query |
| `type` | string | `all` | Source type: `memory`, `guideline`, `pr`, `all` |
| `limit` | int | `10` | Maximum results |

**Response** (200):
```json
[
  {
    "source": "memory",
    "content": "...",
    "score": 0.95,
    "metadata": { "title": "..." }
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
server-side `GITHUB_TOKEN` in v1.0.

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
