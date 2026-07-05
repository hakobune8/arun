# ARUN v1.5.24 Release Notes

ARUN v1.5.24 is a patch release for generated repository consistency after
validating `run-2afc43291a772eb9` on v1.5.23.

## Highlights

- Generated artifact hygiene now removes orphan Helm chart fragments such as
  `charts/*/templates` without a matching `Chart.yaml`.
- Helm quality gates now fail when any generated chart directory is incomplete,
  including template-only fragments that fresh-checkout reviewers would lint.
- Docs quality gates now fail when generated sprint/report docs claim missing
  endpoints such as `/health` or missing layout directories such as `cmd/` or
  `web/`.
- Deterministic docs recovery removes stale generated sprint/report docs with
  invalid repository claims and then re-validates, rather than treating fallback
  artifacts alone as a pass.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.24` and Helm chart version `1.5.24`.

```bash
helm upgrade arun https://github.com/hakobune8/arun/releases/download/arun-1.5.24/arun-1.5.24.tgz \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.24
```

## Verification

- `go test ./internal/orchestrator`
- `go test ./...`
- `npm --prefix web run build`
- `helm lint charts/arun`
- `helm template arun charts/arun`
- Web UI header shows `v1.5.24 workspace`.
