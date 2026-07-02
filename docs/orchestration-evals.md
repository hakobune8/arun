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
- three-sprint agile scrum continuity across backlog refinement, sprint
  planning, execution, QA, review, reporting, retrospective, and next-sprint
  planning
- required artifacts such as `go.mod`, tests, workflow YAML, README, and review notes
- quality gate pass/fail reporting
- scenario duration and failure reasons
- functional coverage by area

Run a single scenario:

```sh
agentos evals --scenario empty-go-service-bootstrap --format json --output -
```

Run the three-sprint scrum simulation:

```sh
agentos evals \
  --scenario three-sprint-agile-scrum \
  --format markdown \
  --output .agentos/evals/three-sprint-scrum-report.md
```

The scrum scenario is deterministic by default. It models three iterations with
backlog refinement, sprint planning, execution, QA, review, reporting,
retrospective notes, and next-sprint planning. The report includes
sprint-by-sprint planned work, completed work, carried work, blockers, stage
checks, required reports, and functional coverage. Set
`AGENTOS_EVAL_SCRUM_LIVE=true` only when live GitHub and LiteLLM preset
configuration is ready; in that mode the scenario makes missing
`AGENTOS_EVAL_GITHUB_REPO` and `AGENTOS_EVAL_LLM_PRESET_MATRIX` /
`AGENTOS_LLM_PRESETS` settings fail the run.

## Functional Coverage

The report includes functional coverage, not only Go line coverage. Areas
include planning, agent routing, fallback execution, quality gates, required
artifacts, frontend, CI workflow, Docker, Helm, Kubernetes, reporting,
release-readiness, backlog refinement, sprint execution, retrospective,
memory continuity, and GitHub workflow routing.

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

Real GitHub writes and Kubernetes rollout checks are separate v1.4.x
operational scenarios. They are not part of the default CI eval suite.

## Live GitHub Workflow E2E

Live GitHub workflow checks are opt-in and create real GitHub artifacts. Run
them only against a dedicated test repository that already has the configured
base branch, with a token or GitHub App installation that can create issues,
branches, files, and pull requests:

```sh
AGENTOS_EVAL_GITHUB_REPO='owner/test-repo' \
AGENTOS_EVAL_GITHUB_BASE_BRANCH='main' \
GITHUB_TOKEN='<token-with-repo-access>' \
agentos evals \
  --github-workflow-e2e \
  --scenario github-workflow-e2e \
  --format markdown \
  --output .agentos/evals/github-workflow-report.md
```

The scenario creates a titled `[AgentOS Eval]` issue, comments on it, creates a
temporary branch, commits a small `.agentos-evals/` file, opens a draft pull
request, looks up check runs and workflow runs for the base branch, then closes
the PR and issue and deletes the branch. The report records the issue URL, PR
URL, branch, file path, cleanup state, and check/workflow lookup counts.

## Executable Scrum GitHub E2E

The executable scrum GitHub scenario extends the deterministic
`three-sprint-agile-scrum` flow with real GitHub issue artifacts. It is disabled
by default and refuses to run unless the target repository is explicitly
allowlisted.

Use a dedicated test repository such as `kazyamaz200/agentos-test`:

```sh
AGENTOS_EVAL_GITHUB_REPO='kazyamaz200/agentos-test' \
AGENTOS_EVAL_GITHUB_REPO_ALLOWLIST='kazyamaz200/agentos-test' \
GITHUB_TOKEN='<token-with-repo-access>' \
agentos evals \
  --scrum-github-e2e \
  --scenario three-sprint-scrum-github-e2e \
  --format markdown \
  --output .agentos/evals/scrum-github-report.md
```

The scenario creates `[AgentOS Eval]` backlog issues, comments sprint status on
each planned item, carries one blocked item from sprint 2 to sprint 3, closes
completed work, and records issue URLs plus sprint-by-sprint state in the
report. The first implementation keeps evidence as closed issues. More advanced
cleanup modes and interrupted-run recovery are tracked separately.

## Kubernetes Rollout E2E

Kubernetes rollout checks are opt-in and require an explicit kubeconfig,
context, and namespace. Run them only in a disposable namespace or another
controlled canary target:

```sh
AGENTOS_EVAL_KUBECONFIG=/secure/path/kubeconfig.yaml \
AGENTOS_EVAL_KUBE_CONTEXT=mgmt-k3s \
AGENTOS_EVAL_KUBE_NAMESPACE=agentos-evals \
agentos evals \
  --kubernetes-rollout-e2e \
  --scenario kubernetes-rollout-e2e \
  --format markdown \
  --output .agentos/evals/kubernetes-rollout-report.md
```

The scenario creates a small disposable Helm release
(`AGENTOS_EVAL_KUBE_RELEASE`, default `agentos-eval-rollout`) using a generated
chart, installs a baseline image, upgrades to a target image, waits for
Deployment rollout and readiness, observes the deployed image, runs
`helm rollback` to the baseline revision, observes the rollback image, and then
uninstalls the test release. Set `AGENTOS_EVAL_KUBE_KEEP_RELEASE=true` only
when you need to inspect the release after a failed run.

Optional image overrides:

```sh
AGENTOS_EVAL_KUBE_BASE_IMAGE=registry.k8s.io/pause:3.9
AGENTOS_EVAL_KUBE_TARGET_IMAGE=registry.k8s.io/pause:3.10
```

The report includes namespace, context, release, Deployment name, deployed
image, rollback image, Helm revision and status, rollout duration, rollback
note, and recent Kubernetes event snippets. Failure output includes recent
events without printing configured auth cookies or GitHub tokens.

## Real LLM Orchestration Smoke

Real LLM smoke checks are opt-in and make live LiteLLM calls. They run a single
bounded docs-agent orchestration against a disposable local repository and
verify that the agent creates `AGENTOS_REAL_LLM_SMOKE.md` with the required
smoke phrase and residual-risk section:

```sh
AGENTOS_EVAL_LIVE_LLM=true \
AGENTOS_EVAL_LLM_REPO_ALLOWLIST=temp \
AGENTOS_EVAL_LLM_MODEL=coder \
AGENTOS_EVAL_LLM_TIMEOUT=2m \
AGENTOS_EVAL_LLM_MAX_TOKENS=1024 \
LITELLM_BASE_URL=http://litellm:4000 \
LITELLM_API_KEY='<litellm-api-key>' \
agentos evals \
  --real-llm-smoke-e2e \
  --scenario real-llm-orchestration-smoke \
  --format markdown \
  --output .agentos/evals/real-llm-smoke-report.md
```

The scenario is disabled by default and requires
`AGENTOS_EVAL_LIVE_LLM=true`, an explicit `LITELLM_BASE_URL`, a model via
`AGENTOS_EVAL_LLM_MODEL` or `AGENTOS_MODEL_CODER`, and an allowlist containing
`temp`. The LLM wrapper caps each request at `AGENTOS_EVAL_LLM_MAX_TOKENS` and
the orchestration context is bounded by `AGENTOS_EVAL_LLM_TIMEOUT`.

The report records plan/subtask count, execution success/failure, required
artifact status, quality-gate results, LLM request/response counts, token usage
when the LiteLLM response includes it, the configured timeout/token budget, and
residual risk. Cost is reported only when supplied externally through
`AGENTOS_EVAL_LLM_COST_BUDGET`.

## LiteLLM Preset Matrix

LiteLLM preset matrix checks are opt-in and make live LiteLLM calls. They send a
short marker prompt to each configured preset and compare success, duration,
failure reason, retry policy, token usage, and budget metadata:

```sh
AGENTOS_EVAL_LLM_PRESETS=true \
AGENTOS_EVAL_LLM_PRESET_MATRIX='[
  {
    "id": "coding",
    "name": "Coding",
    "useCase": "repository implementation and CI fixes",
    "provider": "litellm",
    "baseUrl": "http://litellm:4000",
    "model": "coder",
    "apiKeyEnv": "LITELLM_API_KEY",
    "timeout": "45s",
    "temperature": 0.1,
    "maxTokens": 4096,
    "retryAttempts": 1,
    "tokenBudget": 120000,
    "costBudget": "normal"
  },
  {
    "id": "review",
    "name": "Review",
    "useCase": "PR review, risk analysis, security notes",
    "provider": "litellm",
    "baseUrl": "http://litellm:4000",
    "model": "coder",
    "apiKeyEnv": "LITELLM_API_KEY",
    "timeout": "45s",
    "temperature": 0,
    "maxTokens": 3072,
    "retryAttempts": 1,
    "tokenBudget": 80000,
    "costBudget": "normal"
  },
  {
    "id": "smoke",
    "name": "Low-cost smoke",
    "useCase": "staging and release health checks",
    "provider": "litellm",
    "baseUrl": "http://litellm:4000",
    "model": "coder",
    "apiKeyEnv": "LITELLM_API_KEY",
    "timeout": "30s",
    "temperature": 0,
    "maxTokens": 512,
    "retryAttempts": 0,
    "tokenBudget": 5000,
    "costBudget": "low"
  }
]' \
LITELLM_API_KEY='<litellm-api-key>' \
agentos evals \
  --litellm-preset-evals \
  --scenario litellm-preset-matrix \
  --format markdown \
  --output .agentos/evals/litellm-preset-report.md
```

If `AGENTOS_EVAL_LLM_PRESET_MATRIX` is omitted, the scenario falls back to
`AGENTOS_LLM_PRESETS` and default eval values. The scenario never prints API key
values; it reports only preset IDs, public endpoint metadata, model names,
duration, token usage when LiteLLM returns it, and the budget metadata supplied
in the matrix. By default a non-empty model response is considered a successful
preset call; set `expectedPhrase` on a matrix item when a provider-specific
marker must be present in the response.

## Schedule Notification E2E

Schedule notification checks are opt-in and require an authenticated API
session. The scenario creates a short-lived schedule, manually executes it,
verifies the schedule execution history, verifies the `started` inbox
notification, then deletes the test schedule and notification:

```sh
AGENTOS_EVAL_AUTH_COOKIE='agentos_session=<signed-session-cookie>' \
agentos evals \
  --schedule-notification-e2e \
  --scenario schedule-notification-e2e \
  --live-url https://agentos.nakanoshima.hakobune8.com \
  --format markdown \
  --output .agentos/evals/schedule-notification-report.md
```

Optional `AGENTOS_EVAL_SCHEDULE_REPO` and
`AGENTOS_EVAL_SCHEDULE_BASE_BRANCH` values override the default safe local
repository scope (`.` / `main`). The report includes the schedule ID, trigger
time, run ID, execution status, notification ID, notification trigger, and
notification status.

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
