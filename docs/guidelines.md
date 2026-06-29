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
