# Pre-Merge Verification

Use the shared Kubernetes environment to verify pull requests before merging
when a change affects the Web UI, API behavior, deployment manifests, or
runtime behavior.

## Environment

- Cluster: `mgmt-k3s`
- Kubeconfig: `/Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml`
- Release namespace: `arun`
- Release name: `arun`
- Public URL: `https://arun.hakobune8.com`
- Verification namespace: `arun-build`
- Verification registry: `arun.hakobune8.com/<repo>/<image>:<tag>`

The verification registry is served from the same hostname as the Web UI and
routes only `/v2` to the registry service. The registry Ingress and TLS are
managed in the `sslhq/staips-infra` repository; keep this application chart
focused on the ARUN server deployment.

## Registry and BuildKit

Create or reuse these resources in `arun-build` from the infra repository:

- `registry:2` Deployment and Service.
- Ingress for host `arun.hakobune8.com`, path `/v2`, routing to the registry
  service.
- TLS secret issued for `arun.hakobune8.com`.
- BuildKit Job using `moby/buildkit` with an init container that clones the PR
  branch.

Build PR images with a unique tag:

```bash
export KUBECONFIG=/Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml
export TAG=pr-<number>-<short-sha>

kubectl -n arun-build delete job build-arun-pr --ignore-not-found
kubectl -n arun-build apply -f buildkit-job.yaml
kubectl -n arun-build wait --for=condition=complete job/build-arun-pr --timeout=10m
```

The BuildKit output should push to:

```text
arun.hakobune8.com/arun/arun:${TAG}
```

Before deploying, verify kubelet can pull the image:

```bash
kubectl -n arun-build run image-pull-test \
  --image=arun.hakobune8.com/arun/arun:${TAG} \
  --restart=Never --command -- /usr/local/bin/arun --version
kubectl -n arun-build wait --for=condition=Ready pod/image-pull-test --timeout=2m
kubectl -n arun-build delete pod image-pull-test
```

## Deploy the PR Image

Deploy the PR image with Helm and keep existing runtime values:

```bash
helm upgrade arun ./charts/arun -n arun --reuse-values \
  --set image.repository=arun.hakobune8.com/arun/arun \
  --set image.tag=${TAG} \
  --set image.pullPolicy=Always

kubectl -n arun rollout status deployment/arun --timeout=5m
```

If API checks need to bypass GitHub OAuth temporarily, clear the OAuth client ID
for the verification deployment only:

```bash
helm upgrade arun ./charts/arun -n arun --reuse-values \
  --set auth.required=false \
  --set auth.github.clientId=
```

Restore authentication before handing the environment back to users.

## Checks

Run checks appropriate for the PR before merging:

- `/api/health` returns `{"status":"ok"}`.
- Web UI loads the changed screen.
- Changed API paths return expected status and response body.
- For GitHub automation, use `hakobune8/arun-test` for side-effect tests
  and close any verification issues or PRs created during the test.
- Confirm GitHub Actions checks are green.

## Restore or Promote

If the PR is not merged immediately, restore the previous release image and
auth settings:

```bash
helm upgrade arun ./charts/arun -n arun --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=<previous-tag> \
  --set auth.required=true \
  --set auth.github.clientId=<github-oauth-client-id>
```

After merge, deploy the merged image from the normal release or Docker workflow.
