# ARUN v1.5.18 Release Notes

ARUN v1.5.18 is a patch release for implementation-heavy scrum publish
recovery and generated artifact quality gates.

## Fixed

- Scrum checkpoint publishing now force-refreshes remote tracking refs before
  pushing with `--force-with-lease`, reducing stale lease failures after earlier
  sprint branch updates.
- Frontend documentation fallback now keeps the existing app title as the
  product name and avoids copying task prompt text into product briefs.
- Docker quality gates now fail when a repository has static UI assets but the
  runtime image does not copy them.
- Helm quality gates now fail empty charts and require rendered Deployment and
  Service resources.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.18` and Helm chart version `1.5.18`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.18 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.18
```

## Verification

- `go test ./internal/server -run 'TestGitPushHead|TestPrepareOrchestrationGitHub'`
- `go test ./internal/orchestrator -run 'TestRecoverFrontendStaticAppUsesInvaderProductConceptTitle|TestRecoverFrontendDocsKeepsExistingStaticAppTitle|TestRecoverDockerfile|TestDockerQualityGate|TestRecoverBuiltInSubtask_HelmRuntimeErrorRecovers|TestHelmQualityGate'`
- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.18 workspace`.
