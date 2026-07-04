# ARUN v1.5.17 Release Notes

ARUN v1.5.17 is a patch release for generated application quality in
implementation-heavy scrum runs.

## Fixed

- Fallback Dockerfiles now copy static frontend assets into runtime images when
  a browser UI exists, so container `/` serves the same primary UI as local
  `go run .`.
- Static frontend fallback now writes `docs/product-brief.md` and implements a
  gravity-lane mechanic in the generated UI/source code.
- The implementation-heavy scrum template now treats duplicate product briefs,
  README H1 drift, missing implemented differentiating mechanics, and missing
  container `/` smoke checks as release-blocking quality gaps.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.17` and Helm chart version `1.5.17`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.17 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.17
```

## Verification

- `go test ./internal/orchestrator`
- `go test ./internal/server`
- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.17 workspace`.
