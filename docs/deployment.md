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
  --set env.LITELLM_BASE_URL=http://litellm:4000
```

### From Local Chart

```bash
helm install agentos ./charts/agentos \
  --namespace agentos --create-namespace \
  --set env.LITELLM_BASE_URL=http://litellm:4000
```

## Configuration

### Required

| Parameter | Description | Example |
|-----------|-------------|---------|
| `env.LITELLM_BASE_URL` | LiteLLM proxy URL | `http://litellm:4000` |

### Optional

See [values.yaml](../charts/agentos/values.yaml) for all available options.

| Parameter | Default | Description |
|-----------|---------|-------------|
| `image.tag` | `1.0.0` | Container image tag |
| `env.AGENTOS_MODEL_CODER` | `coder` | LLM model for coding tasks |
| `env.AGENTOS_HOME` | `/home/agentos/.agentos` | State directory for run artifacts and local vector indexes |
| `env.QDRANT_URL` | `""` | Qdrant vector DB URL (optional) |
| `env.GITHUB_TOKEN` | `""` | GitHub API token (optional) |
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

The container runs as a non-root `agentos` user by default. The built-in Web UI
does not provide authentication; expose it through an authenticated ingress,
VPN, or reverse proxy for shared environments. Optional NetworkPolicy rendering
can be enabled with `networkPolicy.enabled=true`.

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
