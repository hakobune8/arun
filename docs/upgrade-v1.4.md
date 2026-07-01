# Upgrade To v1.4

This guide summarizes the v1.4 Governance Scale & Evals release. It covers
orchestration governance limits, storage retention and cleanup, and the
orchestration evaluation suite.

## Release Status

v1.4.0 completes the planned Governance Scale & Evals milestone:

- governance execution limits, quotas, and cost controls
- storage retention, archival, and cleanup policies
- orchestration regression and scenario evaluation coverage

Operational flow hardening continues in v1.4.x with authenticated Web UI E2E,
live GitHub and Kubernetes scenarios, schedule-to-notification checks, real LLM
smoke coverage, LiteLLM preset tuning, and agile scrum simulation.

## Governance Limits

Orchestrations and schedules can carry governance metadata for:

- maximum duration
- maximum subtasks
- maximum retries
- repository concurrency
- organization concurrency
- LLM token budget
- GitHub request budget

The server enforces maximum duration, subtask count, repository concurrency,
and organization concurrency. Retry, token, and request budgets are recorded in
metadata for policy visibility and follow-up enforcement work.

## Storage Retention And Cleanup

v1.4 adds storage usage reporting and retention cleanup for persistent
AgentOS state under `AGENTOS_HOME`.

Cleanup supports dry-run preview and execution. It archives before deletion and
skips records that are active or linked to GitHub artifacts so operators can
review the planned cleanup before data is removed.

Production deployments that require authentication return `401` for storage
APIs without a valid session. This is expected.

## Orchestration Evals

Run the deterministic suite locally or in CI:

```bash
agentos evals
```

Generate machine-readable and reviewable reports:

```bash
agentos evals --format json --output report.json
agentos evals --format markdown --output report.md
```

Live smoke checks are opt-in and should only be enabled in environments with
the required credentials and disposable test targets. See
[Orchestration Evals](orchestration-evals.md) for environment variables,
scenario coverage, and report formats.

## Helm Upgrade

The v1.4.0 chart defaults `image.tag` to `v1.4.0`. Upgrade the release while
keeping existing runtime values:

```bash
helm --kubeconfig /Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml \
  upgrade --install agentos charts/agentos \
  -n agentos \
  --reuse-values \
  --set image.repository=ghcr.io/kazyamaz200/agentos \
  --set image.tag=v1.4.0 \
  --set image.pullPolicy=Always \
  --server-side=true \
  --force-conflicts \
  --wait \
  --timeout 5m
```

`--server-side=true --force-conflicts` is useful when a previous emergency
rollout changed the Deployment image with `kubectl set image`.

## Post-Upgrade Verification

Check the running deployment:

```bash
kubectl --kubeconfig /Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml \
  -n agentos get deploy agentos -o wide

kubectl --kubeconfig /Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml \
  -n agentos rollout status deploy/agentos --timeout=180s
```

Check the public service and Web UI assets:

```bash
curl -fsS https://agentos.nakanoshima.hakobune8.com/api/health

INDEX=$(curl -fsS https://agentos.nakanoshima.hakobune8.com/)
JS=$(printf '%s\n' "$INDEX" | sed -n 's/.*src="\([^"]*index-[^"]*\.js\)".*/\1/p')
CSS=$(printf '%s\n' "$INDEX" | sed -n 's/.*href="\([^"]*index-[^"]*\.css\)".*/\1/p')

curl -fsSI "https://agentos.nakanoshima.hakobune8.com$JS"
curl -fsSI "https://agentos.nakanoshima.hakobune8.com$CSS"
```

Confirm the built-in agent registry:

```bash
curl -fsS https://agentos.nakanoshima.hakobune8.com/api/agents \
  | jq -r '.[].Name' | sort
```

Expected agents:

```text
analyst
ci-fixer
dependency-updater
devops
docker
docs
frontend
go-backend
helm
kubernetes
qa
release-manager
reporter
reviewer
security
```

For the v1.4 feature scope, also confirm:

- storage usage is visible in the Web UI for an authenticated session
- storage cleanup preview responds before cleanup execution is enabled
- `agentos evals` produces deterministic JSON and Markdown reports
- unauthenticated storage and schedule APIs return `401` when
  `auth.required` is enabled

## Final Release Checklist

- Confirm `CHANGELOG.md` v1.4.0 notes include every closed v1.4 milestone
  issue.
- Verify README roadmap separates v1.4.0 completed work from v1.4.x hardening.
- Verify `charts/agentos/Chart.yaml` and `charts/agentos/values.yaml` use the
  final release version and image tag.
- Build and deploy the final v1.4.0 image, then run health, Web UI asset,
  agent registry, storage, and eval checks.
- Confirm the deployed image tag, chart version, GitHub release tag, and
  changelog heading all refer to the same final v1.4.0 version.
