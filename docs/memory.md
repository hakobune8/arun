# Memory Layer

AgentOS provides a pluggable memory abstraction for agent state retention
across runs. The runtime does not know which implementation is in use.

## Store Interface

```go
type Store interface {
    Save(ctx context.Context, entry *Entry) error
    Search(ctx context.Context, query string, limit int) ([]Entry, error)
    Clear(ctx context.Context) error
    Type() string
}
```

## Backends

### VectorStore

Uses a vector database (local JSON or Qdrant) with LLM embeddings for
semantic search.

```go
vs := vector.NewLocalStore("./vectors")
emb := embedding.NewLiteLLMEmbedder()
store := memory.NewVectorStore(vs, emb)
```

### JSONStore

File-based JSONL storage with simple text search. Zero external dependencies.

```go
store, _ := memory.NewJSONStore("./memory.jsonl")
```

## Configuration

```go
cfg := memory.Config{
    Backend: "json",       // "vector", "json", or "sqlite"
    Path:    "./memory.jsonl",
}
store, _ := memory.New(ctx, cfg, vs, embedder)
```
