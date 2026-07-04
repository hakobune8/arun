# ARUN v1.5.16 Release Notes

ARUN v1.5.16 is a patch release for implementation-heavy scrum checkpoint
branch publishing.

## Fixed

- Run branch publishing now refreshes the remote tracking ref before
  `git push --force-with-lease`.
- Successful run branch pushes now update the local remote tracking ref to
  `HEAD`.
- Later Sprint 2 and Sprint 3 checkpoint publishes no longer remain blocked by
  stale lease state after the Sprint 1 PR branch is created.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.16` and Helm chart version `1.5.16`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.16 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.16
```

## Verification

- `go test ./internal/server`
- `go test ./...`
- Web UI header shows `v1.5.16 workspace`.
