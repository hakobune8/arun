# Pre-Merge Verification

Use the shared Kubernetes environment to verify changes before opening or
updating a pull request when a change affects orchestration flow, Web UI, API
behavior, deployment manifests, container contents, or runtime behavior.

The standard flow is:

1. Run local checks such as `go test ./...` and `git diff --check`.
2. Build the current worktree in the cluster with BuildKit.
3. Deploy the short-lived validation image to the live ARUN environment.
4. Run targeted live checks, including a full orchestration when the task flow
   changed.
5. Only then open or update the GitHub PR and let GitHub CI validate the same
   commit.

## Environment

- Cluster: `mgmt-k3s`
- Kubeconfig: `/Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml`
- Release namespace: `arun`
- Release name: `arun`
- Public URL: `https://arun.hakobune8.com`
- Verification namespace: `arun-build`
- Verification image registry: `ttl.sh`

Validation images are pushed to `ttl.sh` with a 24-hour tag. This keeps the
pre-CI path independent from GitHub Container Registry permissions and avoids
publishing release-like tags before the change has passed live verification.

## Registry and BuildKit

Create or reuse the BuildKit Deployment and Service in `arun-build`, build the
current worktree as a short-lived image, deploy it with Helm, and run basic
runtime checks:

```bash
scripts/live-validate-buildkit.sh
```

By default the script:

- creates or updates `arun-build/buildkit`,
- builds `linux/amd64` from the current checkout,
- pushes `ttl.sh/arun-validate-<short-sha>:24h`,
- deploys it to `arun/arun` with `helm upgrade --reuse-values`,
- verifies `/api/health`,
- prints `arun version`,
- confirms `npm`, `helm`, and `go` are available in the runtime image.

For repeated or long-running validation, deploy the validation image into an
isolated namespace instead of replacing the live `arun` release:

```bash
RELEASE_NAMESPACE=arun-validate \
RELEASE_NAME=arun-validate \
MIRROR_RELEASE_VALUES=1 \
USE_GH_AUTH_TOKEN=1 \
scripts/live-validate-buildkit.sh
```

With `MIRROR_RELEASE_VALUES=1`, the script reads Helm values from
`arun/arun`, copies the `arun-runtime` Secret into the target namespace when
the namespace differs, and installs or upgrades the target release with the
short-lived validation image. This keeps production traffic on the released
image while the validation namespace uses the same LiteLLM, GitHub, OAuth, and
runtime settings.

`USE_GH_AUTH_TOKEN=1` copies the local `gh auth token` into the validation
namespace Secret as `GITHUB_TOKEN`. Use this for non-WebUI validation runs that
must create Issues, push branches, and create PRs without a browser OAuth
session. Alternatively set `VALIDATION_GITHUB_TOKEN` explicitly.

For user-scoped validation with GitHub OAuth, enable device flow on the OAuth
App and run:

```bash
ARUN_BASE_URL=https://arun.hakobune8.com scripts/device-login.sh
```

The script stores a signed ARUN session cookie in
`~/.arun/device-session.cookie`. Use that cookie jar with authenticated API
requests when validation must publish generated GitHub Actions workflow files
with the user's `workflow` scope.

Set `RUN_EVALS=1` to also run the built-in orchestration evals:

```bash
RUN_EVALS=1 scripts/live-validate-buildkit.sh
```

## Non-WebUI Orchestration Smoke

When the orchestration flow itself changes, repeat the full implementation-heavy
scrum scenario without using the Web UI:

```bash
scripts/live-validate-orchestrate.sh
```

This script does not change the public Helm auth settings. Instead, it starts a
temporary local-only ARUN server inside the current pod on port `18080` with
auth disabled, port-forwards that port to localhost, fetches the selected
scenario template, starts `/api/orchestrate`, and polls `/api/orchestrates/<id>`
until the run completes or fails. The temporary server is stopped when the
script exits.

To run the same non-WebUI orchestration against the isolated namespace:

```bash
RELEASE_NAMESPACE=arun-validate \
RELEASE_NAME=arun-validate \
scripts/live-validate-orchestrate.sh
```

Default inputs match the ARUN sandbox verification path:

- `TARGET_REPO=hakobune8/arun-test`
- `BASE_BRANCH=main`
- `TEMPLATE_ID=implementation-heavy-scrum`
- `PARENT_TASK=新規性のあるインベーダーゲームを作りたい。`
- `LLM_PRESET=staips`
- `OUTPUT_LANGUAGE=ja`

Useful overrides:

```bash
PARENT_TASK='別の検証タスク' \
TARGET_REPO=hakobune8/arun-test \
POLL_INTERVAL_SECONDS=60 \
MAX_WAIT_SECONDS=14400 \
scripts/live-validate-orchestrate.sh
```

The script relies on the deployment's existing runtime environment, including
`GITHUB_TOKEN`, LiteLLM presets, and persistent `ARUN_HOME`, so it exercises the
same server orchestration path as the Web UI without requiring a browser
session. Use device flow when the validation must run with a user-scoped OAuth
token while keeping the public authenticated API enabled.

Useful overrides:

```bash
KUBECONFIG_PATH=/path/to/kubeconfig \
VALIDATION_ID=validate-my-change \
IMAGE_REPOSITORY=ttl.sh/arun-validate-my-change \
IMAGE_TAG=24h \
scripts/live-validate-buildkit.sh
```

## Deploy the PR Image

The script deploys the validation image automatically. To deploy an already
built validation image manually, keep existing runtime values and change only
the image:

```bash
helm upgrade arun ./charts/arun -n arun --reuse-values \
  --set image.repository=ttl.sh/arun-validate-<short-sha> \
  --set image.tag=24h \
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
- `arun version` reports the validation ID.
- Runtime tools required by orchestration are present, including `npm`, `helm`,
  and `go`.
- Built-in evals pass when the change affects orchestration behavior.
- Web UI loads the changed screen.
- Changed API paths return expected status and response body.
- For GitHub automation, use `hakobune8/arun-test` for side-effect tests
  and close any verification issues or PRs created during the test.
- After live verification passes, open or update the GitHub PR and confirm
  GitHub Actions checks are green.

## Restore or Promote

If the PR is not merged immediately, restore the previous release image. The
script prints the previous image and a restore command after deployment:

```bash
helm upgrade arun ./charts/arun -n arun --reuse-values \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=<previous-tag>
```

After merge, deploy the merged image from the normal release or Docker workflow.
