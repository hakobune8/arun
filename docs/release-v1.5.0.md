# ARUN v1.5.0 Release Notes

ARUN v1.5.0 is the repository-transfer and rename release. The repository now
lives at `hakobune8/arun`, and the public release, image, chart, module, CLI,
environment, state, cookie, and documentation surfaces use the ARUN name.

## Changes Since v1.4.1

- Repository transferred to `hakobune8/arun`.
- Production Web UI URL changed to `https://arun.hakobune8.com`.
- Go module path changed to `github.com/hakobune8/arun`.
- Primary CLI binary and command examples changed to `arun`.
- Helm chart name, chart path, release examples, Kubernetes labels, and default
  image repository changed to ARUN naming.
- Runtime environment variables, state directories, repository convention
  directories, and auth cookies changed to `ARUN_*`, `~/.arun`, `.arun/`, and
  `arun_session`.
- Public documentation, README badges, release links, chart metadata, chart
  releaser config, GoReleaser release target, and default container image
  repository now point at `hakobune8/arun`.
- Helm chart `version`, chart `appVersion`, default image tag, and Web UI
  workspace label now identify v1.5.0.
- Public Web UI branding now uses `ARUN`.
- Web UI language can be switched between English and Japanese.
- Japanese UI mode persists in the browser and defaults orchestration output
  language to Japanese when no output language was selected.
- Built-in scenario templates return Japanese names and descriptions in
  Japanese UI mode and include an instruction that reports, issue/PR bodies,
  summaries, and stakeholder notes should be written in Japanese unless
  repository conventions require otherwise.
- A product rename and repository transfer migration plan is documented in
  [Product Rename and Repository Transfer Migration Plan](product-rename-migration-plan.md).

## Compatibility

v1.5.0 intentionally makes the ARUN rename as a breaking cleanup because there
are no external production users yet. Existing v1.4.x runtime state and Helm
values should be treated as pre-rename data and migrated or discarded
deliberately during rollout.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.0` and Helm chart version `1.5.0`.

```bash
helm --kubeconfig /Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml \
  upgrade --install arun charts/arun \
  -n arun \
  --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.0 \
  --set env.ARUN_PUBLIC_URL=https://arun.hakobune8.com \
  --set auth.github.callbackUrl=https://arun.hakobune8.com/auth/callback \
  --set image.pullPolicy=Always \
  --server-side=true \
  --force-conflicts \
  --wait \
  --timeout 5m
```

## Verification Checklist

- CI and CodeQL pass on the transferred repository.
- Docker workflow publishes `ghcr.io/hakobune8/arun:v1.5.0` for
  `linux/amd64` and `linux/arm64`.
- Helm chart workflow publishes chart version `1.5.0` to
  `https://hakobune8.github.io/arun`.
- GitHub Release exists under `hakobune8/arun`.
- Production rollout uses the v1.5.0 image.
- Production Ingress and TLS for `arun.hakobune8.com` are applied from
  `sslhq/staips-infra`.
- Production OAuth callback URL is configured as
  `https://arun.hakobune8.com/auth/callback`.
- `/api/health` returns `{"status":"ok"}`.
- Web UI JavaScript and CSS assets return HTTP 200.
- Web UI header shows `v1.5.0 workspace`.
- Web UI language can switch to Japanese and survives reload.
- Built-in scenario templates show Japanese labels and set output language to
  Japanese in Japanese UI mode.
- `/api/agents` returns the built-in registry.

## Rollback

Because v1.5.0 renames module, chart, CLI, environment, cookie, and state
surfaces, rollback should be handled as a deliberate redeploy of the previous
v1.4.1 release rather than a simple in-place Helm value rollback.
