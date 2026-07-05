# ARUN v1.5.29 Release Notes

Released: 2026-07-06

## Highlights

- Generated frontend recovery now handles partial HTML output where
  `index.html` references local CSS or JavaScript assets that were not created.
- This covers the failure observed in `run-1595721ff6f52640`, where
  `client/index.html` referenced `style.css` and `app.js` but those files were
  missing, causing the first frontend validation gate to fail before branch or
  PR publication.

## Validation

- `go test ./internal/orchestrator -run 'TestRecoverBuiltInSubtask_FrontendValidationRecovers(MissingReferencedAssets|GoService|UnservedRootAssets)|TestFrontendQualityGateFailsForMissingLocalAssets'`
- `go test ./...`
- `git diff --check`

## Upgrade

Update the image tag or Helm chart version to `v1.5.29`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.29
```
