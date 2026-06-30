# Upgrade To v1.2

This draft guide summarizes the v1.2 Agent Expansion & Repository Context
changes that are ready before the final Docker, Helm, and Kubernetes operations
agent work in #193 lands.

## Release Status

v1.2.0 documentation is intentionally not final until #193 is closed. Before
tagging v1.2.0, update this guide and the changelog with the final operations
agent behavior, validation expectations, and any deployment examples introduced
by that issue.

## Agent Expansion

v1.2 broadens the built-in agent registry beyond the original backend, review,
CI, and docs workflows. Built-in agents now carry richer metadata for domains,
trigger keywords, trigger files, architecture guidance, and output
expectations. The orchestrator uses that metadata during planning and fallback
routing so work can be assigned to more appropriate specialists.

Specialized built-ins now cover frontend application work, security review,
release preparation, dependency updates, QA, documentation, CI fixes, backend
implementation, and review. Docker, Helm, and Kubernetes operations agents are
tracked separately in #193 and should be considered pending until that issue is
closed.

## Repository-Defined Agents

Target repositories can provide custom AgentOS profiles under:

```text
.agentos/agents/*.yaml
```

The Web UI can load these profiles from the selected repository. AgentOS
validates custom agents before use, rejects attempts to override built-in agent
names, and persists selected definitions on the orchestration record for
reproducibility.

See [Repository Agents](repository-agents.md) for the schema, validation rules,
and examples.

## Scenario Templates

v1.2 adds reusable scenario templates for common orchestration flows. Templates
can define task prompts, default agents, execution strategy, pull request
preference, and variables.

Repository-specific templates live under:

```text
.agentos/scenarios/*.yaml
```

The New Orchestrate form can preview and apply templates before starting a run.
See [Scenario Templates](scenario-templates.md) for the file format and
validation rules.

## Repository Context

Repository memory, guidelines, and scoped search are first-class v1.2 context
sources:

- Repository memory stores approved lessons by repository and branch. Completed
  runs can propose pending memory for human approval before future reuse.
- Repository guidelines store branch-scoped rules from the Web UI and load
  `.agentos/guidelines/*.md`, `*.yaml`, and `*.yml` from target repositories
  during planning.
- Repository context search spans memory, guidelines, orchestration runs, run
  artifacts, GitHub artifacts, and code/files for the selected repository and
  branch.

These features are available through the Web UI and REST APIs documented in
[API Reference](api.md), [Memory](memory.md), [Guidelines](guidelines.md), and
[Search](search.md).

## React Web UI

The Web UI has moved from the legacy embedded static HTML page to a
React/TypeScript application built with Vite and Tailwind CSS. Docker image
builds now run the frontend build before compiling the Go binary, and CI runs
frontend lint, production build, and responsive smoke checks.

Operationally, the server still serves built assets from
`internal/server/static`. Local Web UI development happens under `web/`:

```bash
cd web
npm ci
npm run dev
npm run build
npm run lint
npm run smoke
```

The Web UI is mobile-first and includes Orchestrate, Agents, Audit, GitHub,
Memory, Guidelines, Search, and run detail views. The New Orchestrate flow now
uses a GitHub repository picker backed by authenticated repository listing.

See [React Web UI Migration](webui-react-tailwind-migration.md) for additional
implementation notes.

## GitHub And Webhook Notes

GitHub OAuth sessions can protect work-triggering APIs, while repository clone
and write operations use server-side GitHub credentials or GitHub App
installation tokens.

GitHub-to-AgentOS webhook delivery is still not required for v1.2.0 unless a
later release task changes this before tagging. The `on_pr_merge` close policy
remains recorded for conservative manual follow-up; automatic PR merge
detection remains deferred.

## Deployment Notes

Upgrade the Helm release to the desired v1.2.0 image tag and keep persistent
storage enabled so repository memory, guidelines, run artifacts, and
orchestration records survive pod restarts:

```bash
helm repo update
helm upgrade --install agentos agentos/agentos \
  --namespace agentos \
  --set image.tag=v1.2.0 \
  --set env.LITELLM_BASE_URL=http://litellm:4000
```

Before publishing a chart release that changes `charts/agentos/**`, update both
`version` and `appVersion` in `charts/agentos/Chart.yaml` intentionally so the
chart release workflow can publish a new immutable chart version.

## Final Release Checklist

- Close #193 and document the final Docker, Helm, and Kubernetes operations
  agents.
- Confirm `CHANGELOG.md` v1.2.0 notes include every closed v1.2 milestone issue.
- Verify README feature summaries match the final agent registry and Web UI.
- Confirm this upgrade guide no longer contains draft-only wording before the
  v1.2.0 tag is created.
- Build and deploy the final v1.2.0 image, then run the documented health and
  Web UI asset checks.
