# ARUN v1.5.30 Release Notes

Released: 2026-07-06

## Highlights

- Strengthens generated artifact contracts for implementation-heavy scrum runs,
  with clearer expectations for `server/`, `client/`, Docker, Helm, and
  validation commands.
- Moves generated frontend package metadata into `client/package.json` for
  separated client/server repositories and rejects stale root `package.json`
  layouts.
- Adds runtime asset validation so the Go server must serve the same local
  CSS/JS referenced by `client/index.html`.
- Adds validation helper scripts for cluster-side image builds and repeatable
  full orchestrate checks against a validation release.

## Validation

- `go test ./internal/orchestrator -count=1`
- `go test ./...`
- `npm --prefix web run lint`
- `npm --prefix web run build`
- GitHub Actions CI for PR #448:
  - `frontend`
  - `lint`
  - `build`
  - `build-macos`
  - `build-windows`
- Live validation image `ttl.sh/arun-validate-c767d14:24h` completed
  `run-e834f5caac063225` with 25/25 subtasks completed and 0 failed.
- Fresh checkout validation of the generated `hakobune8/arun-test` branch
  passed for server/client layout, Go tests/vet/build, client scripts,
  Helm lint/template, and HTTP asset serving.

## Upgrade

Update the image tag or Helm chart version to `v1.5.30`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.30
```
