# Upgrade To v1.3

This guide summarizes the v1.3 Scheduled Operations & Reporting release. It
covers investigation/reporting agents, scoped evidence sources, recurring
orchestration schedules, built-in maintenance workflow templates, outcome
notifications, and Docker/Helm/Kubernetes operations agents.

## Release Status

v1.3.0 completes the Scheduled Operations & Reporting milestone. The final
release includes all planned v1.3 implementation issues, including the
Docker, Helm, Kubernetes, and DevOps built-in operations agents.

## Built-In Agents

v1.3 adds investigation, reporting, and operations specialists to the built-in
registry:

- `analyst` investigates logs, run history, GitHub evidence, repository files,
  memory, guidelines, and artifacts.
- `reporter` turns findings into structured Markdown reports and
  GitHub-ready summaries.
- `docker` handles Dockerfiles, image builds, `.dockerignore`, compose files,
  health checks, and container safety expectations.
- `helm` handles chart templates, values, schema, `helm lint`,
  `helm template`, and chart versioning.
- `kubernetes` handles manifests, services, ingress, probes, resources,
  securityContext, rollout checks, and kubectl dry-run validation.
- `devops` coordinates broad deployment and release-hardening work that spans
  image, chart, manifest, security, QA, and rollback concerns.

The planner and recommendation API route operations tasks to the new
specialists. For example, a Helm/Kubernetes rollout task recommends the `ops`
preset with `devops`, `docker`, `helm`, `kubernetes`, `release-manager`,
`security`, `qa`, and `reviewer`.

## Evidence Sources

Repository context search can use:

- repository memory and guidelines
- orchestration records and run artifacts
- GitHub issues, PRs, checks, workflow runs, and workflow logs
- Kubernetes pod logs when `ARUN_KUBERNETES_NAMESPACE` and
  `ARUN_KUBERNETES_SELECTOR` are configured
- repository code/files

Evidence results include provenance metadata and secret redaction.

## Scheduled Runs

The Web UI Schedules page and `/api/schedules` API can persist recurring
orchestration jobs. A schedule stores:

- repository and base branch
- task, agents, strategy, LLM preset, and output language
- interval or five-field cron timing with timezone
- overlap policy
- GitHub Issue/PR artifact settings
- notification settings
- execution history and linked orchestration run IDs

Due schedules run in-process with the Web server. Missed schedules are checked
on startup. Keep persistent storage enabled so schedules and run history
survive pod restarts.

## Workflow Templates

`GET /api/schedules/templates` and the Schedules page expose built-in
maintenance and reporting templates:

- `daily-failed-run-report`
- `weekly-repository-health-report`
- `weekly-security-triage`
- `weekly-dependency-update`
- `monthly-release-readiness`
- `memory-guideline-stale-check`

Templates include recommended agents, schedule examples, expected Markdown
outputs, GitHub artifact defaults, and permission notes. Users can still edit
repository, output language, Issue/PR creation settings, and schedule cadence
before saving.

## Outcome Notifications

Schedules can enable notifications with these triggers:

- `started`
- `completed`
- `failed`
- `skipped`
- `pr_created`
- `quality_gate_failed`
- `manual_intervention`

Destinations are:

- `inbox` for Web UI notification history
- `webhook` for outbound HTTP delivery with retry history
- `github_issue` for comments on a created Issue
- `github_pr` for comments on a created PR

Notification history is stored under `ARUN_HOME/notifications` and exposed
through `GET /api/notifications`.

## Deployment Notes

Upgrade the Helm release to the final v1.3.0 image tag and keep persistent
storage enabled:

```bash
helm repo update
helm upgrade --install arun arun/arun \
  --namespace arun \
  --set image.tag=v1.3.0 \
  --set env.LITELLM_BASE_URL=http://litellm:4000
```

The v1.3.0 chart defaults `image.tag` to `v1.3.0`. Before publishing, confirm
that both `version` and `appVersion` in `charts/arun/Chart.yaml` match the
intended release so the chart release workflow can publish a new immutable
chart version.

## Operational Caveats

- Scheduled jobs run inside the Web server process. Do not scale the server
  horizontally without adding external scheduler coordination.
- Webhook delivery is outbound notification delivery only. GitHub-to-ARUN
  webhook delivery remains optional for issue-triggered workflows.
- The Docker sandbox backend remains an extension point for task execution;
  the new Docker agent is an operations specialist for repository/container
  changes, not an execution sandbox.
- Kubernetes evidence search requires configured namespace, selector, and a
  kubectl-accessible cluster context in the deployment environment.

## Final Release Checklist

- Confirm `CHANGELOG.md` v1.3.0 notes include every closed v1.3 milestone
  issue.
- Verify README feature summaries match the final 15-agent built-in registry.
- Verify the Web UI header and release docs refer to v1.3 where current
  release identity is displayed.
- Verify `charts/arun/Chart.yaml` and `charts/arun/values.yaml` use the
  final release version and image tag.
- Build and deploy the final v1.3.0 image, then run health, Web UI asset,
  agent registry, schedule template, notification, and ops recommendation
  checks.
- Confirm the deployed image tag, chart version, GitHub release tag, and
  changelog heading all refer to the same final v1.3.0 version.
