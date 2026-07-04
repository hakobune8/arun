# ARUN v1.5.14 Release Notes

ARUN v1.5.14 is a patch release for implementation-heavy scrum checkpoint
publishing and Web UI responsiveness.

## Fixed

- Checkpoint publishing now runs only after a checkpoint report subtask
  completes successfully and the checkpoint commit is created. This prevents an
  early generic branch publish from racing the intended sprint checkpoint push.
- `/api/orchestrates` now returns lightweight list summaries instead of full run
  records. This keeps the Web UI List tab responsive when stored runs contain
  large subtasks, results, events, summaries, memory, and guideline payloads.
- Full orchestration details remain available from `/api/orchestrates/{id}`.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.14` and Helm chart version `1.5.14`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.14 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.14
```

## Verification

- `go test ./internal/server`
- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.14 workspace`.
