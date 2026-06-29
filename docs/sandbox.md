# Sandbox

The Sandbox interface abstracts execution isolation for agent tasks.
Runtime does not know which backend is in use.

## Sandbox Interface

```go
type Sandbox interface {
    PrepareRun(taskID string) error
    RunPath() string
    SaveFile(name string, data []byte) error
    AbsPath(relative string) string
    RepoAbsPath(repoRelative string) string
    RootDir() string
    Type() string
}
```

## Backends

### LocalSandbox

Uses the local filesystem for workspace and run directories.

```go
sb := sandbox.NewLocalSandbox("/path/to/repo")
sb.PrepareRun("task-123")
sb.SaveFile("diff.patch", data)
```

### DockerSandbox (future)

Containerized execution with resource limits (stub, not yet implemented).

## Configuration

```go
cfg := sandbox.Config{
    Backend: "local",        // "local" or "docker"
    RootDir: "/path/to/repo",
}
sb, _ := sandbox.New(cfg)
```
