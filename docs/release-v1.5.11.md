# ARUN v1.5.11 Release Notes

ARUN v1.5.11 is a patch release for implementation-heavy scrum generated
repository hygiene.

## Highlights

- Implementation-heavy sprint checkpoints now run a deterministic repository
  hygiene pass before committing.
- Final pull request branch publishing runs the same hygiene pass before
  `git add .`.
- Compiled binary artifacts such as ELF, Mach-O, and PE outputs are removed
  from generated target repositories.
- Markdown files with copied parent prompt blocks are cleaned when they contain
  the observed `Parent task`, `Operating mode`, `Quality bar`, and
  `Expected output` markers.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.11` and Helm chart version `1.5.11`.

```bash
helm upgrade arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.11 \
  --namespace arun \
  --reuse-values \
  --set image.tag=v1.5.11
```

## Validation

- `go test ./...`
- `helm lint charts/arun`
- `helm template arun charts/arun --namespace arun`
- Web UI header shows `v1.5.11 workspace`.
