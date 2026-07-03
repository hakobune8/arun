# Scenario Templates

Scenario templates turn common orchestration requests into reusable task
prompts, agent selections, and launch defaults. The Web UI exposes them in the
New Orchestration form.

Built-in templates cover:

- Go HTTP service bootstrap
- Bug fix with tests and review
- Documentation-only update
- CI failure fixer
- Security/code-scanning remediation
- Release preparation
- Frontend UI change
- Three-sprint agile scrum simulation
- Implementation-heavy scrum for new or sandbox repositories

Selecting a template renders variable inputs and a preview of the generated task
text. Applying the template replaces declared `{{variableName}}` placeholders
from those inputs and fills the task description, recommended agents, strategy,
limits, issue preference, and pull request preference. When an orchestration
starts, AgentOS saves the selected template as `scenarioTemplate` on the
orchestration record.

The Web UI sends the selected UI language when it loads templates. For built-in
templates, Japanese UI mode localizes the template name and description, sets
the template default `outputLanguage` to `ja`, and appends an instruction that
generated reports, issue/PR bodies, user-facing summaries, and stakeholder
notes should be written in Japanese unless repository conventions require
otherwise. Repository-provided templates are not machine-translated; their text
is returned as authored, with the UI language used only as the default output
language when the template does not specify one.

## Stage Presets

Web/API orchestrations route planning and built-in agents to recommended
LiteLLM preset IDs by default:

| Scope | Preset ID |
| --- | --- |
| Planning | `planning` |
| Implementation agents | `coding` |
| `reviewer` | `review` |
| `qa` | `smoke` |
| `analyst` | `planning` |
| `reporter` | `reporting` |

If a recommended preset is not configured, AgentOS falls back to the selected
or default orchestration preset and records the fallback reason as
`stagePresets` on the orchestration record. The Web UI detail view displays the
resolved stage and agent preset routing.

## Repository Templates

Repositories can add custom templates under:

```text
.agentos/scenarios/*.yaml
```

Example:

```yaml
id: repo-docs
name: Repository Docs Update
description: Update project documentation with repository-specific defaults.
agents:
  - docs
  - reviewer
strategy: sequential
createIssue: true
createPullRequest: true
requireApproval: false
taskTemplate: |
  Update {{docTarget}} in {{repo}} on {{baseBranch}}.

  Audience: {{audience}}
  Required details: {{details}}

  Match existing documentation style and keep examples copy-pasteable.
variables:
  - name: repo
    label: Repository
    placeholder: owner/repo
    required: true
  - name: baseBranch
    label: Base branch
    default: main
    required: true
  - name: docTarget
    label: Doc target
    placeholder: README.md
    required: true
  - name: audience
    label: Audience
    placeholder: operators
  - name: details
    label: Required details
    placeholder: configuration and troubleshooting
```

## Validation

Repository templates are validated before they are returned to the Web UI:

- `id`, `name`, `agents`, and `taskTemplate` are required.
- `id` must use kebab-case style names matching `^[a-z][a-z0-9-]{1,62}$`.
- `strategy` must be `sequential` or `parallel`; empty defaults to
  `sequential`.
- Every `agents` entry must exist in the AgentOS registry.
- Variable names must match `^[A-Za-z][A-Za-z0-9_]{0,62}$`.
- Duplicate variable names are rejected.

Template substitution uses `{{variableName}}` placeholders. Missing values are
rendered as empty strings in the preview.
