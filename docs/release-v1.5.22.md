# ARUN v1.5.22 Release Notes

ARUN v1.5.22 is a patch release for generated frontend reviewer-readiness after
`run-0fcc6c77b6703bd2` completed with a served HTML page whose local CSS and
JavaScript assets returned 404.

## Fixed

- Frontend validation now fails when root `index.html` references local CSS or
  JavaScript files that the Go entrypoint does not serve.
- Deterministic frontend recovery now repairs the generated Go entrypoint when
  fallback frontend files already exist, so recovered apps serve `/`,
  `/healthz`, `/styles.css`, and `/src/*.js`.
- Generated pull request bodies no longer paste raw multi-agent execution
  summaries; ARUN replaces those blocks with concise structured summaries.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.22` and Helm chart version `1.5.22`.

```bash
helm upgrade arun https://github.com/hakobune8/arun/releases/download/arun-1.5.22/arun-1.5.22.tgz \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.22
```

## Verification

- `go test ./...`
- `npm --prefix web run build`
- Web UI header shows `v1.5.22 workspace`.
