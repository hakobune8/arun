# ARUN v1.5.7 Release Notes

ARUN v1.5.7 is a patch release for the built-in implementation-heavy scrum
scenario template.

## Changed

- Increased the built-in `implementation-heavy-scrum` template default
  `maxDuration` from `60m` to `120m`.
- Kept the existing `maxSubtasks: 30` and `maxConcurrentRepoRuns: 1` defaults.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.7` and Helm chart version `1.5.7`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --namespace arun \
  --version 1.5.7 \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.7
```

## Verification

- `go test ./...`
- Web UI header shows `v1.5.7 workspace`.
- `/api/health` returns `ok`.
- The `implementation-heavy-scrum` template returns `maxDuration: 120m`.
