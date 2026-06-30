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

Selecting a template renders variable inputs and a preview of the generated task
text. Applying the template fills the task description, recommended agents,
strategy, and pull request preference. When an orchestration starts, AgentOS
saves the selected template as `scenarioTemplate` on the orchestration record.

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
