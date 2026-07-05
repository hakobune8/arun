# ARUN v1.5.27 Release Notes

Date: 2026-07-05

## Highlights

- Generated apps now use a clearer `client/` and `server/` layout for new
  repository runs.
- Root-mixed generated artifacts are migrated during deterministic cleanup, so
  stricter layout gates still have a recovery path.

## Fixes

- Deterministic fallback now writes the Go HTTP entrypoint and tests under
  `server/`.
- Browser assets are written under `client/`, with package scripts checking
  `client/src/main.js`.
- Root-level generated `main.go`, `index.html`, `styles.css`, and `src/main.js`
  can be migrated into the separated layout.
- Docker recovery builds `./server` and copies `client/` into the runtime
  image.
- Eval required artifacts now match the separated generated layout.

## Template Guidance

- Implementation-heavy scrum templates now prefer `client/` for frontend
  assets, `server/` for backend code, `docs/` for product/validation notes, and
  `charts/` or `k8s/` for deployment artifacts.
- Backend guidance now allows clean-architecture-friendly growth by separating
  HTTP handling from domain/application logic when the implementation grows
  beyond a tiny vertical slice.

## Validation

- `go test ./internal/orchestrator`
- `go test ./internal/server`
- `go test ./...`
- `git diff --check`

## Upgrade Notes

- No data migration is required.
- Existing generated branches are not rewritten. Re-run the workflow on
  v1.5.27 to apply the stricter generated layout hygiene.
