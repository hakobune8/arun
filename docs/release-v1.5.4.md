# ARUN v1.5.4 Release Notes

ARUN v1.5.4 is a patch release for Japanese implementation-heavy scrum Go QA
recovery.

## Fixed

- Recognized Japanese UI scrum template wording such as `minimal Go HTTP
  server` when detecting canonical Go service tasks.
- Kept deterministic Go QA recovery active for Japanese runs that hit runtime
  QA validation failures even when the generated Go service is locally valid.
- Added regression coverage based on the production `run-55047b30c2b1de5b`
  failure mode.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.4` and Helm chart version `1.5.4`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --namespace arun \
  --version 1.5.4 \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.4
```

## Verification

- `go test ./...`
- Web UI header shows `v1.5.4 workspace`.
- `/api/health` returns `ok`.
