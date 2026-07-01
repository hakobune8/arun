# v1.4.0 Release Notes

AgentOS v1.4.0 completes the Governance Scale & Evals milestone.

## Highlights

- Governance limits for orchestration duration, subtask count, retries,
  repository concurrency, organization concurrency, LLM token budgets, and
  GitHub request budgets.
- Storage retention policies with usage reporting, dry-run cleanup,
  archive-before-delete, cleanup execution reports, and skips for active or
  GitHub-linked records.
- Orchestration evaluation suite with deterministic scenario coverage,
  functional coverage reporting, JSON/Markdown reports, and opt-in live smoke
  checks.
- Mobile storage cleanup UX fixes for explicit preview-before-cleanup behavior
  and non-overlapping bottom navigation labels.

## Upgrade

Use image tag `ghcr.io/kazyamaz200/agentos:v1.4.0` and Helm chart version
`1.4.0`.

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

## Verification Checklist

- Deployment rolls out successfully and reports one ready pod.
- `/api/health` returns `{"status":"ok"}`.
- Web UI JavaScript and CSS assets return HTTP 200.
- `/api/agents` returns the 15 built-in agents.
- Authenticated Web UI storage usage loads.
- Storage cleanup requires previewed selection before execution.
- `agentos evals` completes the deterministic suite and writes JSON/Markdown
  reports.
- Unauthenticated protected APIs return `401` when production authentication is
  required.

## Notes

- Live evals remain opt-in because they require credentials and disposable
  external targets.
- v1.4.x will deepen real operational scenario coverage, including
  authenticated Web UI E2E, GitHub issue/PR flow, Kubernetes rollout/rollback,
  schedule notifications, real LLM smoke, LiteLLM preset tuning, and
  three-sprint agile scrum simulation.

## Rollback

Roll back to the previous known-good image tag with Helm while preserving
runtime values:

```bash
helm --kubeconfig /Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml \
  upgrade --install agentos charts/agentos \
  -n agentos \
  --reuse-values \
  --set image.repository=ghcr.io/kazyamaz200/agentos \
  --set image.tag=<previous-tag> \
  --set image.pullPolicy=Always \
  --server-side=true \
  --force-conflicts \
  --wait \
  --timeout 5m
```
