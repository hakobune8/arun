# Product Rename and Repository Transfer Migration Plan

Issue: [#271](https://github.com/kazyamaz200/agentos/issues/271)

## Decision for v1.5.0

v1.5.0 should prioritize releasing from the transferred GitHub repository while
deferring a broad product/package rename.

The public name `AgentOS` has meaningful collision risk. Recent search results
show multiple active or visible uses, including:

- `rivet-dev/agentos` on GitHub.
- `agentos-project/agentos` and `agentos.org`.
- Infobip `AgentOS`.
- PwC `agent OS`.
- Builder Methods `Agent OS`.
- Several AI-agent or agent operating system references using the same phrase.

The candidate name `arun` is shorter and more distinctive in this project
context, but it is not clear enough to adopt broadly yet. `arun` is already used
as a PyPI package name and appears heavily as a personal/user name across GitHub
and public search results. Treat it as a candidate codename until a final
availability, trademark, domain, registry, and package-name check is complete.

## Recommended Scope

### v1.5.0: Repository Transfer Release

Release v1.5.0 only after the repository has moved to the target GitHub owner.
Keep the product, CLI, Go module, chart, image, state directories, environment
variables, cookie names, and Kubernetes release defaults compatible with v1.4.x.

Required v1.5.0 changes:

- Update README badges, clone URLs, release URLs, chart repository URLs, and
  operational runbooks to the transferred repository location.
- Update GitHub Actions references that embed the old owner/repository.
- Confirm release, Docker image, chart, CodeQL, and CI workflows still run from
  the transferred repository.
- Confirm GitHub OAuth callback, repository picker, issue creation, PR creation,
  and workflow polling work after transfer.
- Keep `github.com/kazyamaz200/agentos` as the Go module path unless the
  transfer owner explicitly requires a module-path breaking change.
- Keep `ghcr.io/kazyamaz200/agentos` image compatibility for at least one
  release window if GHCR access and package visibility permit it.
- Keep the Helm chart name `agentos` and Kubernetes release name examples
  unchanged for v1.5.0.

### Post-v1.5.0: Product Rename Track

Create separate implementation issues after a final name is selected. Do not
mix a repository transfer, product rename, Go module rename, image migration,
and Helm chart rename in a single PR.

Suggested phases:

1. Product-facing brand label and Web UI copy.
2. Documentation and screenshots.
3. Docker image namespace/tag publishing, with old image aliases retained.
4. Helm chart display metadata and optional new chart alias.
5. CLI binary and command alias strategy.
6. Go module path migration only if the repository name or owner change makes it
   unavoidable.
7. Environment variables, state directories, cookie names, and API identifiers
   only with explicit compatibility shims.

## Inventory

The repository contains many durable `AgentOS`, `agentos`, and `AGENTOS`
references. A current source scan, excluding bundled static assets, found
references across documentation, Go packages, Helm charts, Docker, tests,
scripts, and Web UI files.

High-impact areas:

- README badges, clone commands, Go Report Card, Go Reference, release links,
  chart repository command, screenshots, and roadmap.
- `go.mod` module path and every Go import path under
  `github.com/kazyamaz200/agentos`.
- CLI command examples and binary name `agentos`.
- `cmd/agentos`.
- Docker image defaults in `charts/agentos/values.yaml`.
- Helm chart path, chart name, helper template names, selectors, labels,
  release examples, and chart-releaser config.
- Kubernetes examples using namespace, release, service, ingress host, labels,
  and environment variables.
- Runtime state and config names such as `AGENTOS_HOME`.
- Auth cookies such as `agentos_session` and `agentos_oauth_state`.
- Web UI title, header, static HTML metadata, favicon/icon assets, and version
  label.
- GitHub issue/PR comments, eval artifacts, generated fallback repository
  content, and test fixtures.
- `.agentos` repository convention directories.

Low-risk brand-only surfaces:

- Web UI display title and version/about surfaces.
- README headline and marketing copy.
- Screenshot alt text and documentation prose.
- Release notes.

High-risk compatibility surfaces:

- Go module path.
- CLI command name and shell completion.
- Docker image repository.
- Helm chart name and release name.
- Kubernetes labels/selectors.
- Environment variables.
- Cookie names and auth session behavior.
- State directory layout under `${AGENTOS_HOME}` and `.agentos` repository
  conventions.

## Repository Transfer Checklist

Before transfer:

- Decide the target owner and final repository slug.
- Confirm branch protection rules can be recreated.
- Confirm GitHub Actions secrets and environment variables are available in the
  target owner.
- Confirm GHCR package ownership, visibility, and write permissions.
- Confirm GitHub Pages availability for chart publishing.
- Confirm OAuth App callback URL and allowed repository access after transfer.
- Freeze release branches and avoid merging broad rename work during transfer.

Immediately after transfer:

- Update local remotes.
- Recreate or verify branch protection.
- Verify Actions permissions, CodeQL, release, Docker, and chart workflows.
- Verify GitHub Pages chart index.
- Verify GHCR image publishing from the transferred repository.
- Verify existing install commands or document replacement commands.
- Verify the production deployment can pull the intended v1.5.0 image.

v1.5.0 release gate:

- CI passes.
- CodeQL passes.
- Docker workflow publishes multi-arch `linux/amd64` and `linux/arm64` image.
- Helm chart workflow publishes the immutable v1.5.0 chart.
- GitHub Release exists under the transferred repository.
- Production rollout uses the v1.5.0 image.
- `/api/health` returns OK.
- Web UI assets return HTTP 200.
- `/api/agents` returns the built-in registry.
- Repository picker, GitHub issue creation, and PR creation are manually smoked
  from the production Web UI.

Rollback:

- Keep v1.4.1 release notes and image coordinates documented.
- Keep production values capable of rolling back to
  `ghcr.io/kazyamaz200/agentos:v1.4.1` until the new registry path is proven.
- If chart publishing fails after transfer, deploy with an explicit image tag
  and local chart checkout while chart publication is repaired.

## Product Rename Checklist

Before choosing a new public name:

- Search GitHub, npm, PyPI, Go package index, Docker Hub, GHCR, Helm Artifact
  Hub, common domains, and common social handles.
- Check obvious trademark and product conflicts.
- Decide whether the project needs a product name, repository slug, CLI name,
  Go module path, Docker image path, and Helm chart name to match exactly.
- Decide whether old names remain as aliases or become deprecated.

If `arun` remains a candidate:

- Do not use it as a package/module/registry name without deeper availability
  review.
- Prefer testing it first as a Web UI/product display name or codename.
- Avoid changing `agentos` operational identifiers until a compatibility plan
  is accepted.

## Implementation Issue Split

Recommended follow-up issues after #271:

- Transfer repository and update repository-location documentation for v1.5.0.
- Verify release workflows after repository transfer.
- Add Web UI product display-name configuration.
- Update README/screenshots/release notes for selected product branding.
- Add Docker image migration and alias plan.
- Add Helm chart migration and alias plan.
- Add CLI alias/deprecation support if the command name changes.
- Add Go module migration plan if the module path changes.

## Documentation Impact

README roadmap should track repository transfer and rename planning as v1.5+
productization work. Release notes for v1.5.0 should call out whether v1.5.0 is
repository-transfer-only or also includes any user-visible branding change.
