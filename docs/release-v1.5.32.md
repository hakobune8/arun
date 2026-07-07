# ARUN v1.5.32 Release Notes

Released: 2026-07-07

## Highlights

- Fails generated artifact hygiene when a complete Helm chart under
  `charts/<name>/` is accompanied by orphan root chart fragments such as
  `charts/values.yaml`, `charts/Chart.yaml`, `charts/Chart.lock`, or
  `charts/.helmignore`.
- Removes those orphan chart fragments during deterministic recovery so
  generated PRs are closer to a human-mergeable state for continued feature
  development.
- Strengthens the implementation-heavy scrum template to require
  self-contained chart directories and reviewer-accurate validation docs.
- Updates deterministic fallback README/testing docs to show Go, frontend, and
  Helm validation from a fresh-checkout perspective.

## Validation

- `go test ./internal/orchestrator -count=1`
- `go test ./internal/server -count=1`
- `go test ./...`
- `npm --prefix web run lint`
- `npm --prefix web run build`
- `helm lint ./charts/arun`
- `helm template arun ./charts/arun`

## Upgrade

Update the image tag or Helm chart version to `v1.5.32`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.32
```
