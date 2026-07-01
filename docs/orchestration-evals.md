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

Authenticated workflows, real GitHub writes, and Kubernetes rollout checks
remain environment verification steps for PRs and releases. They are not part
of the default CI eval suite.
