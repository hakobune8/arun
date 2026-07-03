# v1.4.1 Release Notes

ARUN v1.4.1 is a patch release for the v1.4 operational-flow hardening
track. It collects fixes from production verification of the
`implementation-heavy-scrum` Web UI flow.

## Changes Since v1.4.0

- Implementation-heavy scrum can complete against an empty or sandbox
  repository with concrete Go, frontend, Docker, Helm, Kubernetes, CI, and
  documentation artifacts.
- Docker, Helm, and Kubernetes fallback recovery now handles missing or invalid
  generated artifacts and reruns the relevant validation gates.
- Scrum templates no longer force a Japanese stakeholder report. They now ask
  for the selected output language or the repository's usual language.
- Web UI orchestration counts treat completed subtasks as passed even when
  older records do not include a result `success` field.
- Stage Presets are displayed as explicit stage, agent, and preset values.
- Web UI and Helm defaults now identify the patch release as v1.4.1.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.4.1` and Helm chart version
`1.4.1`.

```bash
helm --kubeconfig /Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml \
  upgrade --install arun charts/arun \
  -n arun \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.4.1 \
  --set image.pullPolicy=Always \
  --server-side=true \
  --force-conflicts \
  --wait \
  --timeout 5m
```

## Verification Checklist

- Deployment rolls out successfully and reports one ready pod.
- `/api/health` returns `{"status":"ok"}`.
- Web UI JavaScript and CSS assets return HTTP 200.
- Web UI header shows `v1.4.1 workspace`.
- `implementation-heavy-scrum` can run from the Web UI with GitHub Issue and
  Pull Request creation enabled.
- The orchestration overview shows completed subtasks in the passed count.
- Generated repository artifacts pass `go test ./...`, `go vet ./...`,
  `helm lint`, and `helm template`.

## Rollback

Roll back to the previous known-good image tag with Helm while preserving
runtime values:

```bash
helm --kubeconfig /Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml \
  upgrade --install arun charts/arun \
  -n arun \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=<previous-tag> \
  --set image.pullPolicy=Always \
  --server-side=true \
  --force-conflicts \
  --wait \
  --timeout 5m
```
