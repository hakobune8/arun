# Pre-Merge Verification

Use the shared Kubernetes environment to verify pull requests before merging
when a change affects the Web UI, API behavior, deployment manifests, or
runtime behavior.

## Environment

- Cluster: `mgmt-k3s`
- Kubeconfig: `/Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml`
- Release namespace: `agentos`
- Release name: `agentos`
- Public URL: `https://agentos.nakanoshima.hakobune8.com`
- Verification namespace: `agentos-build`
- Verification registry: `agentos.nakanoshima.hakobune8.com/<repo>/<image>:<tag>`

The verification registry is served from the same hostname as the Web UI and
routes only `/v2` to the registry service. This keeps TLS and kubelet image
pulls working without changing node-level containerd registry configuration.

## Registry and BuildKit

Create or reuse these resources in `agentos-build`:

- `registry:2` Deployment and Service.
- Ingress for host `agentos.nakanoshima.hakobune8.com`, path `/v2`, routing to
  the registry service.
- TLS secret copied from the `agentos` namespace or issued for the same host.
- BuildKit Job using `moby/buildkit` with an init container that clones the PR
  branch.

Build PR images with a unique tag:

```bash
export KUBECONFIG=/Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml
export TAG=pr-<number>-<short-sha>

kubectl -n agentos-build delete job build-agentos-pr --ignore-not-found
kubectl -n agentos-build apply -f buildkit-job.yaml
kubectl -n agentos-build wait --for=condition=complete job/build-agentos-pr --timeout=10m
```

The BuildKit output should push to:

```text
agentos.nakanoshima.hakobune8.com/agentos/agentos:${TAG}
```

Before deploying, verify kubelet can pull the image:

```bash
kubectl -n agentos-build run image-pull-test \
  --image=agentos.nakanoshima.hakobune8.com/agentos/agentos:${TAG} \
  --restart=Never --command -- /usr/local/bin/agentos --version
kubectl -n agentos-build wait --for=condition=Ready pod/image-pull-test --timeout=2m
kubectl -n agentos-build delete pod image-pull-test
```

## Deploy the PR Image

Deploy the PR image with Helm and keep existing runtime values:

```bash
helm upgrade agentos ./charts/agentos -n agentos --reuse-values \
  --set image.repository=agentos.nakanoshima.hakobune8.com/agentos/agentos \
  --set image.tag=${TAG} \
  --set image.pullPolicy=Always

kubectl -n agentos rollout status deployment/agentos --timeout=5m
```

If API checks need to bypass GitHub OAuth temporarily, clear the OAuth client ID
for the verification deployment only:

```bash
helm upgrade agentos ./charts/agentos -n agentos --reuse-values \
  --set auth.required=false \
  --set auth.github.clientId=
```

Restore authentication before handing the environment back to users.

## Checks

Run checks appropriate for the PR before merging:

- `/api/health` returns `{"status":"ok"}`.
- Web UI loads the changed screen.
- Changed API paths return expected status and response body.
- For GitHub automation, use `kazyamaz200/agentos-test` for side-effect tests
  and close any verification issues or PRs created during the test.
- Confirm GitHub Actions checks are green.

## Restore or Promote

If the PR is not merged immediately, restore the previous release image and
auth settings:

```bash
helm upgrade agentos ./charts/agentos -n agentos --reuse-values \
  --set image.repository=ghcr.io/kazyamaz200/agentos \
  --set image.tag=<previous-tag> \
  --set auth.required=true \
  --set auth.github.clientId=<github-oauth-client-id>
```

After merge, deploy the merged image from the normal release or Docker workflow.
