# ARUN v1.5.33 Release Notes

Released: 2026-07-07

## Highlights

- Fails generated artifact hygiene when Helm charts keep placeholder names such
  as `charts/name`, `charts/app`, or `name: name`.
- Repairs placeholder Helm chart directories, `Chart.yaml` names, `k8s/`
  directories, and generated Kubernetes deployment docs to repository-derived
  names before checkpoint commits.
- Updates deterministic Helm fallback to use repository-derived image
  repositories such as `ghcr.io/<owner>/<repo>` when a GitHub remote is
  available.

## Validation

- `go test ./internal/orchestrator -count=1`
- `go test ./...`
- `npm --prefix web run lint`
- `npm --prefix web run build`
- `helm lint ./charts/arun`
- `helm template arun ./charts/arun`

## Upgrade

Update the image tag or Helm chart version to `v1.5.33`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.33
```
