# ARUN v1.5.20 Release Notes

ARUN v1.5.20 is a patch release for generated PR quality gates and pull
request body hygiene after the v1.5.19 live scrum rerun.

## Fixed

- Generated frontend validation now fails unserved alternate UI entrypoints
  such as `web/index.html` when the Go entrypoint does not serve that tree.
- Generated Helm validation now checks every chart in the repository, not only
  the first `Chart.yaml`, and requires each chart to include values, templates,
  and rendered Deployment and Service resources.
- Generated pull request summaries now scrub prompt/task contamination such as
  `Parent task:`, `Operating mode:`, `Quality bar:`, and `Expected output:`
  before readability truncation.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.20` and Helm chart version `1.5.20`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.20 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.20
```

## Verification

- `go test ./internal/orchestrator -run 'TestFrontendQualityGateFailsForUnserved(Web|Frontend)Index|TestHelmQualityGateFails(ForEmptyChart|WhenAnyChartIsEmpty)'`
- `go test ./internal/server -run 'TestArtifactTemplates_PRBody'`
- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.20 workspace`.
