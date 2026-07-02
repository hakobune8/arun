# Orchestration Evals

AgentOS includes a repeatable orchestration eval suite for v1.4 release
regression checks.

## Deterministic Suite

The default suite runs without external LLM, GitHub, Kubernetes, or staging
secrets:

```sh
agentos evals --format markdown --output .agentos/evals/orchestration-report.md
```

It validates:

- fallback planning and agent routing
- deterministic empty-repository Go service recovery
- required artifacts such as `go.mod`, tests, workflow YAML, README, and review notes
- quality gate pass/fail reporting
- scenario duration and failure reasons
- functional coverage by area

Run a single scenario:

```sh
agentos evals --scenario empty-go-service-bootstrap --format json --output -
```

## Functional Coverage

The report includes functional coverage, not only Go line coverage. Areas
include planning, agent routing, fallback execution, quality gates, required
artifacts, frontend, CI workflow, Docker, Helm, Kubernetes, reporting,
release-readiness, and GitHub workflow routing.

Coverage is counted by scenario. A failed scenario does not count as covered
for its areas, which makes release regressions visible by feature instead of
only by package.

## Live Smoke

Live smoke checks are opt-in because they require a reachable deployment and
environment-specific auth/network behavior:

```sh
agentos evals \
  --live \
  --live-url https://agentos.nakanoshima.hakobune8.com \
  --format markdown \
  --output .agentos/evals/live-orchestration-report.md
```

The live smoke checks cover:

- `/api/health`
- Web UI JS/CSS assets
- `/api/agents`
- storage auth boundary, where unauthenticated `/api/storage` should return
  `401` when production auth is required

## Authenticated Web UI E2E

Authenticated browser checks are a separate opt-in layer on top of live smoke.
They require Playwright dependencies from `web/` and explicit session material:

```sh
AGENTOS_EVAL_AUTH_COOKIE='agentos_session=<signed-session-cookie>' \
agentos evals \
  --auth-e2e \
  --live-url https://agentos.nakanoshima.hakobune8.com \
  --format markdown \
  --output .agentos/evals/auth-webui-report.md
```

Alternatively, provide a Playwright storage state file:

```sh
AGENTOS_EVAL_AUTH_STORAGE_STATE=/secure/path/storage-state.json \
agentos evals --auth-e2e --live-url https://agentos.example.com
```

The authenticated Web UI E2E covers:

- `/api/auth/session` returns an authenticated session
- desktop navigation across Orchestrate, Schedules, Storage, Agents, and Audit
- mobile bottom navigation across Run, Sched, Storage, Agents, and Audit
- horizontal overflow and bottom navigation label overlap checks

Session cookies and storage state paths are only read from environment
configuration. They are not printed in eval reports. Set
`AGENTOS_EVAL_AUTH_E2E_OUT` to capture optional desktop and mobile screenshots
outside the repository.

Real GitHub writes and Kubernetes rollout checks remain separate v1.4.x
operational scenarios. They are not part of the default CI eval suite.

## Storage Cleanup E2E

Storage cleanup checks are opt-in and require disposable fixtures that are
scoped by repository and base branch. Do not run this scenario against broad
production data. Seed test orchestration records first, then run:

```sh
AGENTOS_EVAL_AUTH_COOKIE='agentos_session=<signed-session-cookie>' \
AGENTOS_EVAL_STORAGE_REPO='agentos-evals/storage-cleanup' \
AGENTOS_EVAL_STORAGE_BASE_BRANCH='agentos-eval-storage-cleanup' \
agentos evals \
  --storage-cleanup-e2e \
  --scenario storage-cleanup-e2e \
  --live-url https://agentos.nakanoshima.hakobune8.com \
  --format markdown \
  --output .agentos/evals/storage-cleanup-report.md
```

The scenario calls authenticated `/api/storage/cleanup` with a policy that
matches only the configured repo and branch. Use one recent retained record,
one old unlinked record, and one old GitHub-linked record for the same repo and
branch. It verifies:

- `/api/storage` usage can be read before cleanup
- dry-run preview selects disposable cleanup targets
- dry-run preview skips a protected target, such as a GitHub-linked record
- cleanup execution archives or deletes selected targets
- `/api/storage` usage can be read after cleanup
- a successful `storage.cleanup` audit event matches the execution summary
- post-cleanup preview has no remaining selected targets

The report includes selected, archived, deleted, skipped, and byte counts for
dry-run, execution, and post-cleanup preview. It also includes before/after
usage snapshots, matching audit events, and the fixture IDs returned by the
cleanup API.
