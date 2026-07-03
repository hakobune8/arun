# ARUN v1.5.3 Release Notes

ARUN v1.5.3 is a patch release for implementation-heavy scrum planning
resilience.

## Fixed

- Added deterministic recovery for built-in analyst planning subtasks when
  runtime planner output is empty or unparsable.
- Prevented an empty initial planning response from cascading all downstream
  implementation-heavy scrum subtasks.
- Added regression coverage for the production `run-bf1c11db4510fe3a` failure
  mode.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.3` and Helm chart version `1.5.3`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --namespace arun \
  --version 1.5.3 \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.3
```

## Verification

- `go test ./...`
- Web UI header shows `v1.5.3 workspace`.
- `/api/health` returns `ok`.
