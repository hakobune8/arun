# ARUN v1.5.13 Release Notes

ARUN v1.5.13 is a patch release for implementation-heavy scrum GitHub
publishing reliability.

## Fixed

- Implementation-heavy scrum now publishes each sprint checkpoint commit to the
  remote branch instead of waiting until the final PR creation step.
- Implementation-heavy scrum now forces GitHub Issue and PR artifacts even if a
  client omits or disables those flags, keeping the remote branch and PR as the
  source of truth for generated work.
- ARUN creates the pull request after the Sprint 1 checkpoint and updates the
  same PR with a concise final body when the orchestration completes.
- If GitHub rejects generated `.github/workflows/**` files because the OAuth
  token lacks `workflow` scope, ARUN moves the workflow definitions into
  `docs/arun-generated-workflows.md`, amends the unpublished checkpoint commit,
  and retries the push.
- Final GitHub publish failures now set `completed_with_publish_error` instead
  of leaving the run as a clean `completed` state.
- Generated PR bodies are shortened for readability and point to run artifacts,
  generated repository docs, and sprint checkpoint commits for full detail.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.13` and Helm chart version `1.5.13`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.13 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.13
```

## Verification

- `go test ./internal/server -run 'TestRecoverWorkflowScopePushFailure|TestCreatePullRequestForOrchestration|TestCommitScrumSprintCheckpoint|TestArtifactTemplates' -count=1 -v`
- `go test ./internal/github -count=1`
- `go test ./...`
- `git diff --check`
- Web UI header shows `v1.5.13 workspace`.
