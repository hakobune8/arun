# ARUN v1.5.8 Release Notes

ARUN v1.5.8 is a patch release for Web UI task and run readability.

## Changed

- Collapsed long orchestration task text by default in the detail header.
- Split run descriptions from embedded parent task context and made both
  expandable.
- Added a status-colored Runs tab timeline with step numbers and dependency
  tags so task flow is easier to inspect during long orchestration runs.
- Collapsed verbose run output, errors, and diffs behind short previews.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.8` and Helm chart version `1.5.8`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --namespace arun \
  --version 1.5.8 \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.8
```

## Verification

- `npm run build`
- `go test ./...`
- `helm lint charts/arun`
- `helm template arun charts/arun --namespace arun`
- Web UI header shows `v1.5.8 workspace`.
