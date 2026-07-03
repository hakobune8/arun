# ARUN v1.5.2 Release Notes

ARUN v1.5.2 is a patch release for implementation-heavy scrum runs that
generate Go services.

## Fixed

- Added deterministic recovery for Go QA validation failures in built-in
  implementation-heavy scrum scenarios.
- Prevented recoverable Go QA failures from cascading into all downstream scrum
  subtasks.
- Repaired generated Go Dockerfiles that copy `go.sum` when no `go.sum` exists.
- Added generated QA documentation for local tests and smoke testing.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.2` and Helm chart version `1.5.2`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --namespace arun \
  --version 1.5.2 \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.2
```

## Verification

- `go test ./...`
- Web UI header shows `v1.5.2 workspace`.
- `/api/health` returns `ok`.
