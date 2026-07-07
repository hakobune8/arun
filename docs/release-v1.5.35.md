# ARUN v1.5.35 Release Notes

Released: 2026-07-07

## Highlights

- Adds deterministic `.gitignore` recovery during generated repository hygiene
  so local validation outputs such as `server/server`, `node_modules/`, `dist/`,
  and ARUN run logs do not create review noise after a fresh checkout.
- Broadens generated repository detection so early or partially recovered
  `server/` and `client/` layouts still receive build-output ignore patterns.
- Keeps fallback sprint-planning recovery from copying internal parent-task or
  prompt fragments into generated repository documentation.

## Validation

- `go test ./internal/server -count=1`
- `go test ./internal/orchestrator -count=1`
- `go test ./...`
- `npm --prefix web run lint`
- `npm --prefix web run build`
- `helm lint ./charts/arun`
- `helm template arun ./charts/arun`

## Upgrade

Update the image tag or Helm chart version to `v1.5.35`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.35
```
