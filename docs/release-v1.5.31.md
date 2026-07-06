# ARUN v1.5.31 Release Notes

Released: 2026-07-07

## Highlights

- Adds GitHub OAuth device-flow endpoints and a local helper script so
  authenticated orchestration can be repeated outside the Web UI without
  exposing the access token.
- Requests `repo workflow` OAuth scope by default, allowing generated
  `.github/workflows/*` files to be pushed by orchestration runs that use
  GitHub OAuth sessions.
- Strengthens generated artifact hygiene gates for empty generated files,
  duplicate CI workflow names, stray root Helm metadata, plan-only remediation
  documents, and frontend assets that are referenced but not served.
- Improves deterministic recovery for generated frontend assets, CI workflows,
  and stale generated artifact layouts.
- Preserves the live validation deployment after validation errors, making the
  verification path easier to inspect and retry before a release.

## Validation

- `go test ./internal/orchestrator -count=1`
- `go test ./...`
- `npm --prefix web run lint`
- `npm --prefix web run build`
- `helm lint ./charts/arun`
- `helm template arun ./charts/arun`
- `bash -n scripts/live-validate-orchestrate.sh scripts/live-validate-buildkit.sh scripts/device-login.sh`
- Live validation image `ttl.sh/arun-validate-chart-root-hygiene-1783354001:24h`
  completed `run-6a591def142e0beb` with 25/25 subtasks completed and 0 failed.
- Fresh checkout validation of the generated `hakobune8/arun-test` branch
  passed for Go tests/vet/build, client tests/build, Helm lint/template,
  GitHub Actions CI, and HTTP asset serving.

## Upgrade

Update the image tag or Helm chart version to `v1.5.31`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.31
```
