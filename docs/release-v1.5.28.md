# ARUN v1.5.28 Release Notes

Released: 2026-07-06

## Highlights

- Generated Go module paths now prefer the target GitHub repository path, so
  empty-repository fallback output no longer inherits timestamp-based workspace
  directory names.
- Default GitHub issues created from Web UI orchestrations now contain run
  metadata plus a concise task summary instead of embedding the full parent task
  template.
- Generated frontend quality gates now catch missing local CSS/JavaScript
  assets, stale Go static asset handlers, and product-name drift across README,
  product brief, and browser title artifacts.

## Validation

- `go test ./internal/orchestrator ./internal/server`
- `go test ./...`
- Deterministic production pod eval: `7/7 passed` on the pre-release image
  containing these fixes.

## Upgrade

Update the image tag or Helm chart version to `v1.5.28`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.28
```
