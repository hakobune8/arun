# Coding Guidelines

Coding guidelines provide context to agents for maintaining codebase
consistency. Guidelines are stored as YAML files and indexed for
semantic retrieval.

## Guideline Format

```yaml
guidelines:
  - name: "error-handling"
    description: "Always return errors instead of panicking"
    tags:
      - go
      - errors

  - name: "naming"
    description: "Use camelCase for variable names, PascalCase for exports"
    tags:
      - go
      - style
```

## Loading Guidelines

```go
store := guideline.NewStore()
store.LoadDirectory("./guidelines")
```

## Searching Guidelines

```go
results, _ := store.Search(ctx, "error handling patterns", 5)
```

## Repository-Scoped Guidelines

Repository-specific guidelines can be stored in `.agentos/guidelines/` inside a
target repository. AgentOS loads `*.md`, `*.yaml`, and `*.yml` files from this
directory when an orchestration starts, scopes them by repository and base
branch, and reuses them during planning.

Markdown files become a single guideline. The first heading is used as the
title:

```markdown
# Web UI API convention

Place Web UI API handlers in internal/server and add handler tests.
```

YAML files can define one guideline, a list, or a `guidelines` wrapper:

```yaml
guidelines:
  - title: Web UI API convention
    content: Place Web UI API handlers in internal/server and add handler tests.
    type: architecture
    tags: [go-backend]
    required: true
```

`required: true` marks a guideline as mandatory. Required guidelines are ranked
ahead of advisory guidelines and AgentOS records any required guideline that was
not attached to a planned subtask.

## Repository Guideline API

Repository guidelines are also manageable from the Web UI Guidelines tab on an
orchestration detail page.

```http
GET /api/repository-guidelines?repo={repo}&baseBranch={branch}&q={query}&status=active
POST /api/repository-guidelines
PUT /api/repository-guidelines/{id}
DELETE /api/repository-guidelines/{id}
```

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

Orchestration detail responses include:

- `guidelinesUsed`: guidelines attached to planned subtasks.
- `missedRequiredGuidelines`: required guidelines that were loaded but not
  attached to any subtask.
