# Web UI React + Tailwind Migration

Issue: #218

## Product Direction

The AgentOS Web UI should feel like a mobile-first operations OS: restrained,
fast to scan, and focused on repeated repository work. The interface is not a
landing page. It should open directly into the operational workspace.

## Design Principles

- Mobile first, then expand to desktop.
- Dark-first graphite surfaces with cool blue/cyan/green accents.
- Compact controls, small radii, and restrained borders.
- Lists and status rows over decorative cards.
- Long repository URLs, task descriptions, logs, diffs, and errors must wrap
  without causing full-page horizontal overflow.
- Existing API contracts under `/api/...` remain unchanged.

## Mobile Information Architecture

Primary navigation uses a bottom nav:

- Orchestrate
- Agents
- Audit

Orchestrate contains:

- New orchestration as stacked sections.
- Orchestration list as compact run rows.
- Detail view with status header and segmented tabs.

Detail tabs:

- Overview
- Runs
- Memory
- Guidelines
- Search
- GitHub

## Desktop Expansion

Desktop keeps the same routes and hierarchy while expanding layout:

- Top navigation remains available for wide screens.
- Detail content uses wider panels and denser tables.
- Long outputs remain in bounded, readable preformatted panels.

## Migration Commits

1. Capture UI/UX direction and migration plan.
2. Add Vite + React + TypeScript + Tailwind build setup.
3. Port Web UI flows to React components at feature parity.
4. Remove old static HTML path, wire production build into Docker/CI, and add
   responsive smoke checks.

## Acceptance Checks

- `npm run build`
- `npm run lint`
- `npm run smoke`
- `go test ./...`
- `golangci-lint run`
- Desktop and 390px mobile screenshots have no full-page horizontal overflow.
