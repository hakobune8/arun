# Upgrade To v1.1

This guide summarizes the operational changes introduced in AgentOS v1.1 for
GitHub automation, orchestration visibility, and approval-controlled issue
closure.

## GitHub Automation

v1.1 makes GitHub issue and pull request workflows first-class orchestration
outputs. Orchestration records can now include the source issue, generated
branch, pull request URL, close policy, approval status, and whether the source
issue was closed by AgentOS.

For repository write operations, configure a GitHub token with the required
repository permissions. GitHub App installation tokens are supported for
deployments that should avoid long-lived personal access tokens.

## Issue-Sourced Orchestration

Issue-triggered runs can be started from imported GitHub issues and controlled
with labels or slash-style command parameters. Common controls include:

```text
agentos:run
agentos:create-pr
agentos:close-never
agentos:close-on-quality-gate-pass
agentos:close-on-pr-merge
agentos:approval-required
```

Slash-style parameters are also supported in issue comments or imported issue
text:

```text
/agentos run agents=go-backend,reviewer strategy=parallel create_pr=true close_policy=after_human_approval approval=true
```

GitHub-to-AgentOS webhook delivery is not required for v1.1 because many
deployments are not reachable from GitHub. Webhook-based automatic PR merge
detection is deferred; the `on_pr_merge` policy is recorded for conservative
manual follow-up.

## Quality Gates

Orchestration can evaluate expected outputs before marking a run complete.
Quality gates cover generated files, tests, lint commands, diffs, and artifact
requirements. Runs that create pull requests should use quality gates before
publishing or closing source issues.

## Human Approval Gates

For higher-risk tasks, AgentOS can leave a completed run in `pending_approval`
instead of closing the source issue immediately. Authorized users can approve
or reject the run from the Web UI or through:

```http
POST /api/orchestrates/{id}/approval
```

Use `{"action":"approve","reason":"..."}` to approve a pending run, or
`{"action":"reject","reason":"..."}` to reject it. Approval decisions are
captured in the orchestration record and audit log.

## Web UI Changes

The Web UI now includes live orchestration progress, timeline events, logs,
cancellation, recommendations, GitHub output metadata, responsive layout
improvements, and approval controls for pending runs.

## Security And Audit

v1.1 adds RBAC checks around automation actions and records audit events for
write operations. Secret redaction is centralized across logs, reports, comments,
and artifacts. Review redaction settings before exposing orchestration logs to
users outside the operations team.

## Deployment Notes

Upgrade the Helm release to the desired image tag and keep persistent storage
enabled so existing run and orchestration records survive pod restarts:

```bash
helm repo update
helm upgrade --install agentos agentos/agentos \
  --namespace agentos \
  --set image.tag=v1.1 \
  --set env.LITELLM_BASE_URL=http://litellm:4000
```

If GitHub OAuth is required for work-triggering APIs, keep
`auth.required=true` and configure `AGENTOS_ADMIN_USERS` for approval and other
administrative actions.
