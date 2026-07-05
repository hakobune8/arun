# ARUN v1.5.21 Release Notes

ARUN v1.5.21 is a patch release for deterministic frontend recovery after the
v1.5.20 live scrum run failed on an unserved alternate UI tree.

## Fixed

- Frontend fallback recovery now runs when generated repositories contain
  unserved alternate `web/` or `frontend/` trees, even if the primary root
  static frontend already exists.
- Deterministic fallback hygiene now removes unserved alternate frontend trees
  and generated binary artifacts before validating the recovered result.
- Deterministic frontend fallback docs no longer copy full task or prompt text
  into smoke-test, testing, or changelog content.

## Changed

- Documentation now records Qwen3.6-35B-A3B as the current recommended
  open-weight validation model while keeping ARUN model-agnostic.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.21` and Helm chart version `1.5.21`.

```bash
helm upgrade arun https://github.com/hakobune8/arun/releases/download/arun-1.5.21/arun-1.5.21.tgz \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.21
```

## Verification

- `go test ./internal/orchestrator -run 'TestRecoverBuiltInSubtask_FrontendRemovesUnservedAlternateUIAndArtifacts|TestFrontendQualityGateFailsForUnservedWebIndex|TestRecoverBuiltInSubtask_StaticFrontendQAAndRelease'`
- `go test ./internal/orchestrator`
- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.21 workspace`.
