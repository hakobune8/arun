# ARUN Repository Transfer and Rename Record

Issue: [#271](https://github.com/hakobune8/arun/issues/271)

## Decision

v1.5.0 completes the repository transfer and project rename in one release.
The repository, module, CLI, Helm chart, Docker image, runtime environment,
state directory, repository convention directory, auth cookie, documentation,
and Web UI display surfaces now use the ARUN name.

This is intentionally a breaking cleanup. The project has no external
production users yet, so carrying broad compatibility aliases would add more
maintenance cost than value.

## New Canonical Surfaces

- Repository: `hakobune8/arun`
- Go module: `github.com/hakobune8/arun`
- CLI binary: `arun`
- Helm chart path and name: `charts/arun`, `arun`
- Container image: `ghcr.io/hakobune8/arun`
- Runtime environment prefix: `ARUN_`
- Runtime state directory: `~/.arun`
- Repository convention directory: `.arun/`
- Auth session cookie: `arun_session`
- Production URL: `https://arun.hakobune8.com`
- GitHub OAuth callback: `https://arun.hakobune8.com/auth/callback`

## Rollout Notes

- Ingress and TLS for `arun.hakobune8.com` are managed from
  `sslhq/staips-infra`.
- Runtime Kubernetes Secret keys should use the ARUN names where applicable,
  including `ARUN_SESSION_SECRET`.
- GitHub OAuth standard variables remain `GITHUB_OAUTH_*` because those names
  describe the upstream provider rather than the product.
- Existing pre-rename state should be migrated explicitly or cleared before the
  v1.5.0 production rollout.

## Verification Gate

- CI and CodeQL pass under `hakobune8/arun`.
- Docker workflow publishes `ghcr.io/hakobune8/arun:v1.5.0`.
- Helm chart workflow publishes chart `arun` version `1.5.0`.
- GitHub Release exists under `hakobune8/arun`.
- Production rollout uses the ARUN image, ARUN env values, and ARUN state path.
- `/api/health` returns OK from `https://arun.hakobune8.com`.
- Web UI header and document title show `ARUN`.
- `/api/agents` returns the built-in registry.
- Repository picker, GitHub issue creation, and PR creation are manually smoked
  from the production Web UI.
