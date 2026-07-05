# ARUN v1.5.25 Release Notes

ARUN v1.5.25 is a patch release for generated product brief hygiene after
validating `run-6450380fa0b16b52` on v1.5.24.

## Highlights

- Generated artifact hygiene now treats `docs/product-brief.md` as the
  canonical product brief when present.
- Root-level `product-brief.md` is removed when the canonical docs brief
  exists, preventing stale alternate concepts from surviving final checkpoints.
- Regression coverage now covers branches where the implementation and
  `docs/product-brief.md` agree but root `product-brief.md` describes a
  different product.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.25` and Helm chart version `1.5.25`.

```bash
helm upgrade arun https://github.com/hakobune8/arun/releases/download/arun-1.5.25/arun-1.5.25.tgz \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.25
```

## Verification

- `go test ./internal/orchestrator`
- `go test ./...`
- `npm --prefix web run build`
- `helm lint charts/arun`
- `helm template arun charts/arun`
- Web UI header shows `v1.5.25 workspace`.
