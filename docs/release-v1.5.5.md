# ARUN v1.5.5 Release Notes

ARUN v1.5.5 is a patch release for implementation-heavy scrum frontend-stage
recovery after Go service generation.

## Fixed

- Added deterministic recovery for frontend validation failures when a canonical
  Go service has already been generated.
- Generated lightweight static frontend artifacts for Go-service
  frontend/static subtasks instead of cascading downstream scrum subtasks.
- Added regression coverage based on the production `run-5f1e8e8966b5e825`
  failure mode.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.5` and Helm chart version `1.5.5`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --namespace arun \
  --version 1.5.5 \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.5
```

## Verification

- `go test ./...`
- Web UI header shows `v1.5.5 workspace`.
- `/api/health` returns `ok`.
