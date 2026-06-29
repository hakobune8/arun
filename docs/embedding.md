# Embedding

AgentOS uses embeddings for semantic search across memory, guidelines, and
past PRs. The embedder interface abstracts the underlying embedding model.

## Embedder Interface

```go
type Embedder interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    EmbedQuery(ctx context.Context, query string) ([]float32, error)
    Model() string
}
```

## LiteLLMEmbedder

Uses LiteLLM proxy for embedding generation. Configured via environment
variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `LITELLM_BASE_URL` | `http://localhost:4000` | LiteLLM proxy URL |
| `AGENTOS_MODEL_EMBEDDING` | `text-embedding-ada-002` | Embedding model |

```go
emb := embedding.NewLiteLLMEmbedder()
vectors, _ := emb.Embed(ctx, []string{"text to embed"})
```
