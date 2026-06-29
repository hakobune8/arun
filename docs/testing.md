# Web UI Testing Guide

This guide provides step-by-step instructions for testing all features
of the AgentOS Web UI at `https://agentos.nakanoshima.hakobune8.com`.

## Prerequisites

- Access to `https://agentos.nakanoshima.hakobune8.com`
- A running [LiteLLM](https://litellm.vercel.app) proxy configured for the cluster
- A local Git repository with Go code for test runs
- A GitHub repository for testing GitHub integration

---

## 1. Dashboard

### Steps
1. Open `https://agentos.nakanoshima.hakobune8.com`
2. The Dashboard tab is shown by default

### Expected Results
- [ ] Header shows "AgentOS v1.0" with navigation tabs
- [ ] Stats cards show: Total Runs, Completed, Failed, In Progress
- [ ] "Recent Runs" table shows the 10 most recent runs (or empty)
- [ ] Tabs: Dashboard, Runs, Agents, Search, GitHub, Orchestrate, New Run

### Input Values
- No input required
- Runs display automatically from `~/.agentos/runs/`

---

## 2. Agents Page

### Steps
1. Click the **Agents** tab
2. View the list of registered agents

### Expected Results
- [ ] Shows 4 built-in agents: go-backend, reviewer, ci-fixer, docs
- [ ] Each agent card shows: name, version, description, author, required tools
- [ ] Tool tags are displayed as small badges

### Input Values
- No input required
- Data comes from `agent.DefaultRegistry()`

---

## 3. New Run

### Steps
1. Click the **New Run** tab
2. Fill in the form:
   - **Agent**: `go-backend`
   - **Task Title**: `"Add greeting function"`
   - **Description**: `"Add a function Greet(name string) string that returns a greeting message. Use the existing codebase style."`
   - **Repository Path**: `<path to a local Go project>`
3. Click **Start Run**
4. Wait for the run ID link to appear
5. Click the run ID link to view details

### Expected Results
- [ ] Agent dropdown shows all registered agents
- [ ] Form submission triggers async run
- [ ] Status message shows run ID with clickable link
- [ ] Run detail page shows artifacts (plan.json, diff.patch, summary.md, etc.)

### Input Values
| Field | Value |
|-------|-------|
| Agent | `go-backend` |
| Task Title | `Add greeting function` |
| Description | `Add a function Greet(name string) string that returns a greeting message. Use the existing codebase style.` |
| Repository Path | `/path/to/your/go-project` |

---

## 4. Runs Page

### Steps
1. Click the **Runs** tab
2. View the run history table
3. Type a run ID in the filter input
4. Select a status filter
5. Click **View** on any run

### Expected Results
- [ ] Run history table shows all runs with ID, Status, Agent, Created
- [ ] Status filtering works (All, Completed, Failed, Pending)
- [ ] ID text filter works
- [ ] Run detail view shows all artifacts
- [ ] Artifacts are displayed in a readable order (run_state.json, plan.json, diff.patch, etc.)

### Input Values
- Filter: any partial run ID
- Status: select "Completed" to see only successful runs
- Click "View" on any run to see details

---

## 5. Search Page

### Steps
1. Click the **Search** tab
2. Enter a search query
3. Select a source filter (optional)
4. Click **Search**

### Expected Results
- [ ] Search results show matches with source label
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
| 1 | Dashboard | Load page | Stats cards, recent runs, navigation |
| 2 | Agents | View list | 4 agents, metadata, tool tags |
| 3 | New Run | Submit task | Async execution, artifact viewing |
| 4 | Runs | Browse history | Filter by ID/status, detail view |
| 5 | Search | Query search | Results with source labels |
| 6 | GitHub | Load repo | Issues, PRs, Checks tabs |
| 7 | Orchestrate | Multi-agent | Plan preview, execution, summary |
