# Search Service

The unified search service queries multiple data sources using vector
similarity search.

## Sources

| Source | Description |
|--------|-------------|
| `memory` | Agent memory entries |
| `guidelines` | Coding guidelines |
| `prs` | Past pull requests |

## Service

```go
vs := vector.NewLocalStore("./vectors")
emb := embedding.NewLiteLLMEmbedder()
svc := search.NewService(vs, emb)
```

## Searching

```go
results, _ := svc.Search(ctx, "how to handle errors", search.TypeAll, 20)
```

The Web UI search endpoint also supports repository-scoped context discovery:

```http
GET /api/search?q=context&repo=owner/repo&baseBranch=main&source=memory
```

With `repo` set, AgentOS searches only records scoped to that repository and
branch. Supported repository context sources are:

| Source | Description |
|--------|-------------|
| `memory` | Repository memory entries |
| `guideline` | Repository guidelines |
| `run` | Orchestration records |
| `artifact` | Planned subtasks and run outputs |
| `github` | Issue and PR artifacts recorded on orchestrations; when selected explicitly, live GitHub issues, PRs, checks, and workflow run logs |
| `kubernetes` | Pod logs from the configured kubeconfig, namespace, and label selector |
| `code` | Matching repository files |

Repository-scoped search results include `repo`, `branch`, `runId`, score,
timestamps, source metadata, and action links in the Web UI. Search result
cards can be promoted into repository memory or guidelines, and stale memory can
be archived directly from the result list.

Live GitHub evidence is loaded only when `source=github` is selected. AgentOS
redacts obvious tokens and secrets from issue bodies, check output, and workflow
logs before returning them. If one GitHub source cannot be read, the search
response includes a non-fatal `source_error` result while still returning any
other evidence that was available.

Kubernetes logs are loaded only when `source=kubernetes` is selected and
`AGENTOS_KUBERNETES_NAMESPACE` plus `AGENTOS_KUBERNETES_SELECTOR` are set.
`AGENTOS_KUBECONFIG`, `AGENTOS_KUBERNETES_CONTEXT`,
`AGENTOS_KUBERNETES_CONTAINER`, and `AGENTOS_KUBECTL` can further constrain the
source. Log output is redacted before it is returned.

## CLI

```bash
agentos search "query"
```
