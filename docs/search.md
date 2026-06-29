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

## CLI

```bash
agentos search "query"
```
