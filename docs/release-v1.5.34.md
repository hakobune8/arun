# ARUN v1.5.34 Release Notes

Released: 2026-07-07

## Highlights

- Removes empty generated artifacts such as `server/server` during repository
  hygiene before sprint checkpoint and publish commits.
- Keeps compiled binary cleanup in the same final hygiene path so generated
  pull request branches do not retain local build outputs.
- Strengthens the implementation-heavy scrum template to require a repository
  `.gitignore` for local build outputs such as `server/server`, `tmp/`,
  `dist/`, and `node_modules/`.

## Validation

- `go test ./internal/server -count=1`
- `go test ./...`
- `npm --prefix web run lint`
- `npm --prefix web run build`
- `helm lint ./charts/arun`
- `helm template arun ./charts/arun`

## Upgrade

Update the image tag or Helm chart version to `v1.5.34`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.34
```
