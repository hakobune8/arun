#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

KUBECONFIG_PATH="${KUBECONFIG_PATH:-/Users/ssl222/Downloads/kubeconfig/mgmt-k3s.yaml}"
if [[ -f "$KUBECONFIG_PATH" ]]; then
  export KUBECONFIG="$KUBECONFIG_PATH"
fi

RELEASE_NAMESPACE="${RELEASE_NAMESPACE:-arun}"
RELEASE_NAME="${RELEASE_NAME:-arun}"
BUILD_NAMESPACE="${BUILD_NAMESPACE:-arun-build}"
BUILDKIT_IMAGE="${BUILDKIT_IMAGE:-moby/buildkit:v0.25.1}"
BUILDKIT_LOCAL_PORT="${BUILDKIT_LOCAL_PORT:-1234}"
PUBLIC_URL="${PUBLIC_URL:-https://arun.hakobune8.com}"
SHORT_SHA="$(git rev-parse --short HEAD)"
VALIDATION_ID="${VALIDATION_ID:-validate-${SHORT_SHA}}"
IMAGE_REPOSITORY="${IMAGE_REPOSITORY:-ttl.sh/arun-${VALIDATION_ID}}"
IMAGE_TAG="${IMAGE_TAG:-24h}"
PLATFORM="${PLATFORM:-linux/amd64}"
RUN_EVALS="${RUN_EVALS:-0}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_command kubectl
require_command helm
require_command buildctl
require_command curl
require_command jq

PREVIOUS_IMAGE="$(kubectl -n "$RELEASE_NAMESPACE" get deploy "$RELEASE_NAME" -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || true)"

kubectl create namespace "$BUILD_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
kubectl -n "$BUILD_NAMESPACE" apply -f - <<YAML
apiVersion: apps/v1
kind: Deployment
metadata:
  name: buildkit
  labels:
    app.kubernetes.io/name: buildkit
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: buildkit
  template:
    metadata:
      labels:
        app.kubernetes.io/name: buildkit
    spec:
      nodeSelector:
        kubernetes.io/arch: amd64
      containers:
        - name: buildkitd
          image: ${BUILDKIT_IMAGE}
          args:
            - --addr
            - tcp://0.0.0.0:1234
          ports:
            - name: buildkit
              containerPort: 1234
          securityContext:
            privileged: true
          volumeMounts:
            - name: buildkit-state
              mountPath: /var/lib/buildkit
      volumes:
        - name: buildkit-state
          emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: buildkit
  labels:
    app.kubernetes.io/name: buildkit
spec:
  selector:
    app.kubernetes.io/name: buildkit
  ports:
    - name: buildkit
      port: 1234
      targetPort: buildkit
YAML

kubectl -n "$BUILD_NAMESPACE" rollout status deploy/buildkit --timeout=180s

kubectl -n "$BUILD_NAMESPACE" port-forward svc/buildkit "${BUILDKIT_LOCAL_PORT}:1234" >/tmp/arun-buildkit-port-forward.log 2>&1 &
PORT_FORWARD_PID=$!
cleanup() {
  kill "$PORT_FORWARD_PID" >/dev/null 2>&1 || true
}
trap cleanup EXIT

for _ in $(seq 1 30); do
  if buildctl --addr "tcp://127.0.0.1:${BUILDKIT_LOCAL_PORT}" debug workers >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! buildctl --addr "tcp://127.0.0.1:${BUILDKIT_LOCAL_PORT}" debug workers >/dev/null 2>&1; then
  echo "buildkit port-forward did not become ready" >&2
  cat /tmp/arun-buildkit-port-forward.log >&2 || true
  exit 1
fi

echo "building ${IMAGE_REPOSITORY}:${IMAGE_TAG} from ${SHORT_SHA}"
buildctl --addr "tcp://127.0.0.1:${BUILDKIT_LOCAL_PORT}" build \
  --frontend dockerfile.v0 \
  --local context=. \
  --local dockerfile=. \
  --opt "platform=${PLATFORM}" \
  --opt "build-arg:VERSION=${VALIDATION_ID}" \
  --output "type=image,name=${IMAGE_REPOSITORY}:${IMAGE_TAG},push=true"

echo "deploying ${IMAGE_REPOSITORY}:${IMAGE_TAG} to ${RELEASE_NAMESPACE}/${RELEASE_NAME}"
helm -n "$RELEASE_NAMESPACE" upgrade "$RELEASE_NAME" ./charts/arun \
  --reuse-values \
  --set "image.repository=${IMAGE_REPOSITORY}" \
  --set "image.tag=${IMAGE_TAG}" \
  --set "image.pullPolicy=Always"

kubectl -n "$RELEASE_NAMESPACE" rollout status "deploy/${RELEASE_NAME}" --timeout=180s
curl -fsS "${PUBLIC_URL}/api/health" | jq .

POD="$(kubectl -n "$RELEASE_NAMESPACE" get pods -l "app.kubernetes.io/name=${RELEASE_NAME}" -o jsonpath='{.items[0].metadata.name}')"
kubectl -n "$RELEASE_NAMESPACE" exec "$POD" -- arun version
kubectl -n "$RELEASE_NAMESPACE" exec "$POD" -- sh -lc 'command -v npm && npm --version && command -v helm && helm version --short && command -v go && go version'

if [[ "$RUN_EVALS" == "1" ]]; then
  kubectl -n "$RELEASE_NAMESPACE" exec "$POD" -- arun evals --format json | jq -r '.summary'
fi

cat <<EOF

Live validation image deployed:
  ${IMAGE_REPOSITORY}:${IMAGE_TAG}

Previous image:
  ${PREVIOUS_IMAGE:-unknown}

Restore command:
  helm -n ${RELEASE_NAMESPACE} upgrade ${RELEASE_NAME} ./charts/arun --reuse-values --set image.repository=$(printf '%s' "${PREVIOUS_IMAGE}" | sed 's/:.*$//') --set image.tag=$(printf '%s' "${PREVIOUS_IMAGE}" | sed 's/^.*://')
EOF
