# ARUN v1.5.6 Release Notes

ARUN v1.5.6 is a patch release for implementation-heavy scrum frontend and QA
validation in the production runtime image.

## Fixed

- Added Node.js and npm to the runtime image so live orchestration can run
  generated `package.json` validation scripts such as `npm test` and
  `npm run build`.
- Kept frontend and QA quality gates from cascading when a JavaScript runtime or
  package manager is unavailable but static smoke-test evidence is present.
- Added regression coverage for frontend and QA validation without Node/npm on
  `PATH`.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.6` and Helm chart version `1.5.6`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --namespace arun \
  --version 1.5.6 \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.6
```

## Verification

- `go test ./...`
- Web UI header shows `v1.5.6 workspace`.
- `/api/health` returns `ok`.
- Runtime image includes `node` and `npm`.
