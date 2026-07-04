# ARUN v1.5.15 Release Notes

ARUN v1.5.15 is a patch release for implementation-heavy scrum generated
product coherence.

## Fixed

- Go fallback now serves an existing static `index.html` from `/` instead of
  returning unrelated placeholder text when a browser UI exists.
- Invader-style static frontend fallback now uses one product concept across
  package metadata, HTML title, and visible UI labels.
- The implementation-heavy scrum template now directs agents to keep
  `docs/product-brief.md` as the single product brief and treat duplicate
  product brief files as release-blocking gaps.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.15` and Helm chart version `1.5.15`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.15 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.15
```

## Verification

- `go test ./internal/orchestrator`
- `go test ./internal/server`
- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.15 workspace`.
