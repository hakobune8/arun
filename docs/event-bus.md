# Event Bus

AgentOS provides a structured event bus for runtime observability. Every runtime
operation produces typed events that can be persisted, streamed, or replayed.

## Architecture

```
Runtime → EventBus → Subscribers (Logger, FileStore, WebSocket, etc.)
                 ↘ FileStore (JSONL persistence for replay)
```

## Event Types

| Type | Description |
|------|-------------|
| `task.created` | A new task run has been created |
| `planning.started` | Agent begins planning phase |
| `planning.finished` | Agent completes planning |
| `tool.started` | A tool begins execution |
| `tool.finished` | A tool completes successfully |
| `tool.failed` | A tool execution fails |
| `review.started` | Agent begins review phase |
| `review.finished` | Agent completes review |
| `run.completed` | Run finishes successfully |
| `run.failed` | Run fails with an error |

## Bus Interface

```go
type Bus interface {
    Publish(ctx context.Context, e *Event) error
    Subscribe(handler Handler, types ...Type) (Unsubscribe, error)
    Close() error
}
```

## InMemoryBus

The default implementation dispatches events to subscribers synchronously.

```go
bus := event.NewInMemoryBus()
```

## FileStore

Persists events to a JSONL file for later replay.

```go
store, _ := event.NewFileStore("/path/to/events.jsonl")
unsub, _ := bus.Subscribe(store.Handler())
defer unsub()
```

## Replay

```go
store.Replay(ctx, func(ctx context.Context, e event.Event) error {
    fmt.Printf("Replayed: %s\n", e.Type)
    return nil
})
```

## Usage in Runtime

The Runtime's `Run()` method publishes events at every lifecycle phase
through the `r.Events` field:

```go
rt := runtime.NewRuntime(llm, prof, ws, cfg, agt)
rt.Events = bus
```
