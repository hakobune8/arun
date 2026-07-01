# Web UI Testing Guide

This guide provides step-by-step instructions for testing all features
of the AgentOS Web UI at `https://agentos.nakanoshima.hakobune8.com`.

For release regression checks, see the repeatable orchestration eval suite in
[Orchestration Evals](orchestration-evals.md). The default evals run without
external secrets and report functional coverage by scenario. Live deployment
smoke checks are opt-in with `agentos evals --live --live-url <url>`.

## Prerequisites

- Access to `https://agentos.nakanoshima.hakobune8.com`
- A running [LiteLLM](https://litellm.vercel.app) proxy configured for the cluster
- A local Git repository with Go code for test runs
- A GitHub repository for testing GitHub integration

---

## 1. Orchestrate

### Steps
1. Open `https://agentos.nakanoshima.hakobune8.com`
2. The Orchestrate view is shown by default

### Expected Results
- [ ] Header shows "AgentOS" and the authenticated GitHub user when auth is enabled
- [ ] `agentos evals --auth-e2e --live-url <url>` can run with an explicit
  `AGENTOS_EVAL_AUTH_COOKIE` or `AGENTOS_EVAL_AUTH_STORAGE_STATE`
- [ ] Main navigation exposes Orchestrate, Agents, and Audit
- [ ] Orchestrate has New, List, and Detail segments
- [ ] New orchestration starts with Repository, then Task, then Agents
- [ ] Repository is selected from the GitHub repository picker
- [ ] The mobile bottom navigation remains visible and usable

### Input Values
- No input required
- Orchestration records display from `${AGENTOS_HOME}/orchestrations/`

---

## 2. Agents Page

### Steps
1. Click the **Agents** tab
2. View the list of registered agents

### Expected Results
- [ ] Shows 15 built-in agents: go-backend, frontend, reviewer, ci-fixer, docs, security, release-manager, dependency-updater, qa, docker, helm, kubernetes, devops, analyst, reporter
- [ ] Each agent card shows: name, version, description, author, required tools
- [ ] Tool tags are displayed as small badges

### Input Values
- No input required
- Data comes from `agent.DefaultRegistry()`

---

## 3. New Orchestration

### Steps
1. Open **Orchestrate** and select the **New** segment
2. Fill in the form:
   - **Repository**: select a GitHub repository
   - **Base Branch**: `main`
   - **Task**: `"Add a function Greet(name string) string that returns a greeting message. Use the existing codebase style."`
   - **Agents**: `go-backend`
3. Click **Start Orchestration**
4. Wait for the orchestration detail view to appear

### Expected Results
- [ ] Repository selector shows GitHub repositories visible to the deployment credentials
- [ ] Agent checklist shows all registered agents
- [ ] Form submission triggers an async orchestration
- [ ] Detail view shows run status, task text, tabs, timeline, and artifacts

### Input Values
| Field | Value |
|-------|-------|
| Repository | any test GitHub repository |
| Base Branch | `main` |
| Agents | `go-backend` |
| Task | `Add a function Greet(name string) string that returns a greeting message. Use the existing codebase style.` |

---

## 4. Orchestration List And Detail

### Steps
1. Open **Orchestrate** and select the **List** segment
2. View orchestration history
3. Open any orchestration
4. Use the **Detail** tabs to inspect Overview, Runs, Memory, Guidelines, Search, and GitHub

### Expected Results
- [ ] List shows orchestration ID, status, task, repository, branch, and recency
- [ ] Detail header shows selected repository, branch, strategy, preset, and task text
- [ ] Overview shows subtasks, pass/fail counts, summary, and timeline
- [ ] Runs tab shows subtask assignments and outputs
- [ ] Memory, Guidelines, Search, and GitHub tabs load without layout overflow

### Input Values
- Select any existing orchestration.

---

## 5. Search Page

### Steps
1. Click the **Search** tab
2. Enter a search query
3. Select a source filter (optional)
4. Click **Search**

### Expected Results
- [ ] Search results show matches with source label, including GitHub or Kubernetes evidence when those sources are selected and configured
- [ ] Source filter limits results to memory, guidelines, or PRs
- [ ] Empty results show "No results" message

### Input Values
| Field | Value |
|-------|-------|
| Query | `error handling` |
| Source | `All Sources` |

---

## 6. GitHub Integration

### Steps
1. Click the **GitHub** tab
2. Enter a repository (e.g., `kazyamaz200/agentos`)
3. Click **Load**
4. Switch tabs: Issues, Pull Requests, CI Checks

### Expected Results
- [ ] Issues tab shows open issues with number, title, state, labels
- [ ] Pull Requests tab shows open PRs with number, title, state, branch
- [ ] CI Checks tab shows check suites with name, status, conclusion
- [ ] Entering an invalid repo shows error message
- [ ] Empty repos show "No issues/PRs found" message

### Input Values
| Field | Value |
|-------|-------|
| Repository | `kazyamaz200/agentos` |
| Tab | Issues / Pull Requests / CI Checks |

---

## 7. Orchestrate (Multi-Agent)

### Steps
1. Click the **Orchestrate** tab
2. Verify agent checkboxes are displayed
3. Select agents: `go-backend`, `reviewer`
4. Select strategy: `Sequential`
5. Enter task: `"Implement a simple HTTP health check endpoint"`
6. Click **Start Orchestration**

### Expected Results
- [ ] Agent checkboxes show all registered agents
- [ ] Multiple agents can be selected
- [ ] Strategy dropdown has Sequential and Parallel options
- [ ] After submission, shows plan with subtasks
- [ ] Each subtask shows agent assignment
- [ ] Summary is displayed at the end

### Input Values
| Field | Value |
|-------|-------|
| Agents | `go-backend`, `reviewer` |
| Strategy | `Sequential` |
| Task | `Implement a simple HTTP health check endpoint that returns {"status":"ok"}` |

---

## Quick Reference: All Test Cases

| # | Page | Action | Key Verifications |
|---|------|--------|-------------------|
| 1 | Orchestrate | Load page | Repository, Task, Agents order and mobile navigation |
| 2 | Agents | View list | Built-in agents, metadata, tool tags, guidance |
| 3 | New Orchestration | Submit task | Async orchestration and detail view |
| 4 | Orchestration List | Browse history | List and detail tabs |
| 5 | Search | Query search | Results with source labels |
| 6 | GitHub | Load repo | Issues, PRs, Checks tabs |
| 7 | Orchestrate | Multi-agent | Plan preview, execution, summary |
