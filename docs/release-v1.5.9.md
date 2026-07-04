# ARUN v1.5.9 Release Notes

ARUN v1.5.9 is a patch release for implementation-heavy scrum runtime defaults
and generated output quality.

## Changed

- Increased the built-in `implementation-heavy-scrum` template default
  `maxDuration` from `120m` to `180m`, allowing roughly one hour per sprint for
  three-sprint live runs.
- Strengthened heavy scrum task and subtask prompts with:
  - acceptance criteria,
  - product-centered outcome documentation,
  - frontend/backend/deployment/docs layout separation,
  - fresh-checkout validation,
  - residual-risk and known-limitations reporting,
  - release-blocking gap guidance.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.9` and Helm chart version `1.5.9`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --namespace arun \
  --version 1.5.9 \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.9
```

## Verification

- `go test ./internal/server -run 'TestServer_OrchestrateTemplates' -count=1 -v`
- `go test ./...`
- `npm run build`
- `helm lint charts/arun`
- `helm template arun charts/arun --namespace arun`
- Web UI header shows `v1.5.9 workspace`.
