# ARUN v1.5.10 Release Notes

ARUN v1.5.10 is a patch release for implementation-heavy scrum PR creation and
product planning quality.

## Highlights

- Generated orchestration pull request bodies are capped before GitHub API
  submission, preventing long scrum summaries from failing automatic PR creation.
- The `implementation-heavy-scrum` template now requires a product/design brief
  before implementation.
- Qualitative requirements such as novelty, polish, simplicity, and production
  readiness must be converted into observable acceptance criteria.
- Game and UX-heavy work must include a concrete differentiating mechanic,
  interaction, or content choice when the user request calls for it.
- Template guidance now discourages committing parent prompt text, ARUN run
  workspace artifacts, generated archives, or compiled binaries into target
  repositories.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.10` and Helm chart version `1.5.10`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.10 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.10
```

## Validation

- `go test ./internal/server -run 'TestImplementationHeavyScrumPlan_UsesSprintStageWorkflow|TestArtifactTemplates' -count=1 -v`
- `go test ./...`
- `helm lint charts/arun`
- `helm template arun charts/arun --namespace arun`
- Web UI header shows `v1.5.10 workspace`.
