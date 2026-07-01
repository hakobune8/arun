# Deployment

AgentOS can be deployed in Kubernetes using the official Helm chart.
This deploys the AgentOS Web UI server with persistent storage for
run artifacts.

## Prerequisites

- Kubernetes 1.22+
- Helm 3.8+
- A running [LiteLLM](https://litellm.vercel.app) proxy (or any
  OpenAI-compatible API) accessible from the cluster

## Quick Install

```bash
# Add the AgentOS Helm repository
helm repo add agentos https://kazyamaz200.github.io/agentos
helm repo update

# Install AgentOS
helm install agentos agentos/agentos \
  --namespace agentos --create-namespace \
  --set env.LITELLM_BASE_URL=http://litellm:4000 \
  --set secrets.litellmApiKey=<litellm-api-key>
```

The chart repository is published from the `gh-pages` branch by the Helm Chart
workflow. Already published chart versions are skipped, so bump
`charts/agentos/Chart.yaml` before publishing a new chart release.

### From Local Chart

```bash
helm install agentos ./charts/agentos \
  --namespace agentos --create-namespace \
  --set env.LITELLM_BASE_URL=http://litellm:4000 \
  --set secrets.litellmApiKey=<litellm-api-key>
```

## Configuration

### Required

| Parameter | Description | Example |
|-----------|-------------|---------|
| `env.LITELLM_BASE_URL` | LiteLLM proxy URL | `http://litellm:4000` |
| `secrets.litellmApiKey` or `secrets.existingSecret` | LiteLLM API key source | `agentos-secrets` |

### Optional

See [values.yaml](../charts/agentos/values.yaml) for all available options.

| Parameter | Default | Description |
|-----------|---------|-------------|
| `image.tag` | `v1.2.0` | Container image tag |
| `env.AGENTOS_MODEL_CODER` | `coder` | LLM model for coding tasks |
| `env.AGENTOS_HOME` | `/home/agentos/.agentos` | State directory for run artifacts and local vector indexes |
| `env.AGENTOS_PUBLIC_URL` | `""` | Public AgentOS base URL used in GitHub Issue comments |
| `env.AGENTOS_ADMIN_USERS` | `""` | Optional comma-separated GitHub logins allowed to run sensitive automation and view audit history |
| `env.AGENTOS_ORCHESTRATE_SUBTASK_TIMEOUT` | `10m` | Maximum runtime for one orchestration subtask |
| `env.QDRANT_URL` | `""` | Qdrant vector DB URL (optional) |
| `env.GITHUB_TOKEN` | `""` | GitHub API token fallback (optional) |
| `env.GITHUB_APP_ID` | `""` | GitHub App ID for installation-token authentication |
| `env.GITHUB_APP_INSTALLATION_ID` | `""` | GitHub App installation ID |
| `env.GITHUB_APP_PRIVATE_KEY_FILE` | `""` | Path to a mounted GitHub App private key PEM |
| `auth.required` | `false` | Require GitHub login for work-triggering APIs |
| `auth.github.clientId` | `""` | GitHub OAuth App client ID |
| `auth.github.callbackUrl` | `""` | GitHub OAuth callback URL |
| `llm.presets` | default LiteLLM preset | Admin-defined LLM presets shown in the Web UI |
| `secrets.existingSecret` | `""` | Existing Kubernetes Secret containing sensitive values |
| `secrets.githubAppPrivateKey` | `""` | GitHub App private key PEM rendered into the chart Secret |
| `persistence.size` | `10Gi` | Storage for run artifacts |
| `ingress.enabled` | `false` | Enable Ingress |
| `resources.limits.cpu` | `500m` | CPU limit |
| `networkPolicy.enabled` | `false` | Create an ingress NetworkPolicy |

## State and Persistence

AgentOS stores run artifacts and the local vector index under `AGENTOS_HOME`.
The chart sets `AGENTOS_HOME=/home/agentos/.agentos` and mounts persistence at
the same path. If persistence is disabled, the chart uses `emptyDir` and state
is lost when the pod is recreated.

## Security

The container runs as a non-root `agentos` user by default. For shared
environments, enable GitHub login and keep API keys in Kubernetes Secrets.
Optional NetworkPolicy rendering can be enabled with
`networkPolicy.enabled=true`.

### GitHub Login

Create a GitHub OAuth App with a callback URL that matches the public URL for
your deployment:

```text
https://agentos.example.com/auth/callback
```

Then install or upgrade with authentication enabled:

```bash
kubectl -n agentos create secret generic agentos-secrets \
  --from-literal=LITELLM_API_KEY=<litellm-api-key> \
  --from-literal=GITHUB_TOKEN=<github-token> \
  --from-literal=GITHUB_OAUTH_CLIENT_SECRET=<oauth-client-secret> \
  --from-literal=AGENTOS_SESSION_SECRET=<random-32-byte-secret>

helm upgrade --install agentos ./charts/agentos \
  --namespace agentos --create-namespace \
  --set secrets.existingSecret=agentos-secrets \
  --set auth.required=true \
  --set auth.github.clientId=<oauth-client-id> \
  --set auth.github.callbackUrl=https://agentos.example.com/auth/callback
```

When authentication is enabled, the Web UI shows the signed-in GitHub identity
and work-triggering APIs require a valid signed session. Session cookies are
always marked `Secure`, so GitHub login should be served over HTTPS.

### LLM Presets

Use `llm.presets` to expose administrator-approved model choices in the Web UI.
Only public metadata is returned to browsers; API key values are read from
environment variables backed by Kubernetes Secrets.

```yaml
llm:
  defaultPreset: staips
  presets:
    - id: staips
      name: STAIPS coder
      provider: litellm
      baseUrl: http://staips-litellm.staips-edge.svc.cluster.local
      model: staips-chat
      apiKeyEnv: LITELLM_API_KEY
```

### GitHub Container Registry

The chart uses images from `ghcr.io/kazyamaz200/agentos`. If your
cluster requires pull credentials, create a secret:

```bash
kubectl -n agentos create secret docker-registry ghcr \
  --docker-server=ghcr.io \
  --docker-username=<your-username> \
  --docker-password=<your-token>
```

## Architecture

```
                          ┌─────────────┐
                          │  LiteLLM    │
                          │  (external)  │
                          └──────┬──────┘
                                 │
                    ┌────────────▼────────────┐
                    │  agentos (Deployment)    │
                    │  ┌────────────────────┐ │
                    │  │  agentos serve     │ │
                    │  │  (port 8080)       │ │
                    │  └────────────────────┘ │
                    └────────────┬────────────┘
                                 │
                    ┌────────────▼────────────┐
                    │  PersistentVolume       │
                    │  (run artifacts)        │
                    └─────────────────────────┘
```

## Verifying

```bash
# Check pod status
kubectl -n agentos get pods

# Port-forward to test
kubectl -n agentos port-forward svc/agentos 8080:8080

# Test health endpoint
curl http://localhost:8080/api/health
```

Expected response:
```json
{"status":"ok","time":"2026-06-29T02:39:15Z"}
```
