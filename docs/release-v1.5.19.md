# ARUN v1.5.19 Release Notes

ARUN v1.5.19 is a patch release for checkpoint publish reliability and
generated artifact quality gates after the v1.5.18 live scrum rerun.

## Fixed

- Checkpoint branch publishing now uses an explicit
  `--force-with-lease=refs/heads/<branch>:<remote-sha>` value based on
  `git ls-remote`, avoiding stale implicit tracking ref state during later
  sprint publishes.
- Generated frontend validation now fails when `frontend/index.html` exists
  but is not served by the Go entrypoint.
- Generated documentation validation now fails remediation notes that defer
  release-blocking fixes to a future human-led sprint.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.19` and Helm chart version `1.5.19`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.19 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.19
```

## Verification

- `go test ./internal/server -run 'TestGitPushHead|TestPrepareOrchestrationGitHub'`
- `go test ./internal/orchestrator -run 'TestFrontendQualityGate|TestDocsQualityGate|TestRecoverBuiltInSubtask_StaticFrontendFallbackGatePasses|TestRecoverFrontendDocsKeepsExistingStaticAppTitle|TestDockerQualityGate|TestHelmQualityGate'`
- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.19 workspace`.
