# ARUN v1.5.23 Release Notes

ARUN v1.5.23 is a patch release for generated artifact hygiene after
`run-9c4793fa3d28df6a` produced a working root app but still retained stale
alternate UI concepts and an incomplete Helm chart.

## Fixed

- Frontend validation no longer treats root `staticAssetPath` handling as proof
  that alternate `web/` or `frontend/` UI trees are served.
- Deterministic artifact hygiene now removes unserved alternate UI trees and
  incomplete generated Helm charts before final checkpoint validation.
- Regression coverage now protects the pattern where a generated branch has a
  working root app beside stale alternate frontend concepts and `Chart.yaml`-only
  deployment artifacts.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.23` and Helm chart version `1.5.23`.

```bash
helm upgrade arun https://github.com/hakobune8/arun/releases/download/arun-1.5.23/arun-1.5.23.tgz \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.23
```

## Verification

- `go test ./internal/orchestrator`
- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.23 workspace`.
