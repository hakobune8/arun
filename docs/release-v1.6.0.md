# ARUN v1.6.0 Release Notes

Released: 2026-07-07

## Highlights

- Reconciles stale active orchestration records on server startup. Persisted
  `planning` or `running` records without an active worker are now marked
  `interrupted` with an explicit event instead of appearing to run forever.
- Allows operators to cancel persisted running records even when the original
  worker disappeared from the current server process.
- Treats `interrupted` as a terminal orchestration state for governance usage
  and notifications, so stale records no longer block repository concurrency.
- Adds device-flow audit evidence for authorization start, successful
  authentication, failures, and logout token removal.
- Returns the requested OAuth scope in successful device-flow poll responses so
  operators can confirm that `repo` and `workflow` were requested.

## Operational Notes

- `interrupted` means ARUN observed an active persisted record during startup
  but no in-process worker was registered for it. The run should be reviewed
  from its GitHub branch/PR or restarted intentionally.
- Canceling a persisted running record without an active worker records
  `cancel.requested` and `canceled` events and releases governance concurrency.
- Device flow still stores the GitHub token only inside the signed
  `arun_session` cookie. The token is never returned in API JSON responses.

## Validation

- `go test ./internal/server -count=1`
- `go test ./...`
- `npm --prefix web run lint`
- `npm --prefix web run build`
- `helm lint ./charts/arun`
- `helm template arun ./charts/arun`
- README screenshot refreshed from the local v1.6.0 Web UI.

## Upgrade

Update the image tag or Helm chart version to `v1.6.0`:

```bash
helm upgrade arun charts/arun \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.6.0
```
