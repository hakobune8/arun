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
VALIDATION_API_PORT="${VALIDATION_API_PORT:-18080}"
LOCAL_PORT="${LOCAL_PORT:-18080}"
TARGET_REPO="${TARGET_REPO:-hakobune8/arun-test}"
BASE_BRANCH="${BASE_BRANCH:-main}"
TEMPLATE_ID="${TEMPLATE_ID:-implementation-heavy-scrum}"
UI_LANGUAGE="${UI_LANGUAGE:-ja}"
OUTPUT_LANGUAGE="${OUTPUT_LANGUAGE:-ja}"
LLM_PRESET="${LLM_PRESET:-staips}"
PARENT_TASK="${PARENT_TASK:-新規性のあるインベーダーゲームを作りたい。}"
ISSUE_TITLE="${ISSUE_TITLE:-${PARENT_TASK%%$'\n'*}}"
PR_TITLE="${PR_TITLE:-${ISSUE_TITLE}}"
POLL_INTERVAL_SECONDS="${POLL_INTERVAL_SECONDS:-30}"
MAX_WAIT_SECONDS="${MAX_WAIT_SECONDS:-14400}"
KEEP_VALIDATION_API="${KEEP_VALIDATION_API:-0}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_command kubectl
require_command curl
require_command jq

POD="$(kubectl -n "$RELEASE_NAMESPACE" get pods -l "app.kubernetes.io/name=${RELEASE_NAME}" -o jsonpath='{.items[0].metadata.name}')"
if [[ -z "$POD" ]]; then
  echo "no ${RELEASE_NAMESPACE}/${RELEASE_NAME} pod found" >&2
  exit 1
fi

cleanup_local() {
  if [[ -n "${PORT_FORWARD_PID:-}" ]]; then
    kill "$PORT_FORWARD_PID" >/dev/null 2>&1 || true
  fi
}

cleanup_remote() {
  if [[ "$KEEP_VALIDATION_API" == "1" ]]; then
    return
  fi
  kubectl -n "$RELEASE_NAMESPACE" exec "$POD" -- sh -lc '
    if [ -f /tmp/arun-live-validate-api.pid ]; then
      kill "$(cat /tmp/arun-live-validate-api.pid)" >/dev/null 2>&1 || true
      rm -f /tmp/arun-live-validate-api.pid
    fi
  ' >/dev/null 2>&1 || true
}

trap 'cleanup_local; cleanup_remote' EXIT

kubectl -n "$RELEASE_NAMESPACE" exec "$POD" -- sh -lc '
  if [ -f /tmp/arun-live-validate-api.pid ]; then
    kill "$(cat /tmp/arun-live-validate-api.pid)" >/dev/null 2>&1 || true
    rm -f /tmp/arun-live-validate-api.pid
  fi
  ARUN_AUTH_REQUIRED=false \
  GITHUB_OAUTH_CLIENT_ID= \
  GITHUB_OAUTH_CLIENT_SECRET= \
  nohup arun serve --port '"$VALIDATION_API_PORT"' >/tmp/arun-live-validate-api.log 2>&1 &
  echo $! >/tmp/arun-live-validate-api.pid
'

kubectl -n "$RELEASE_NAMESPACE" port-forward "pod/${POD}" "${LOCAL_PORT}:${VALIDATION_API_PORT}" >/tmp/arun-live-validate-port-forward.log 2>&1 &
PORT_FORWARD_PID=$!
API_BASE="http://127.0.0.1:${LOCAL_PORT}"

for _ in $(seq 1 60); do
  if curl -fsS "${API_BASE}/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done
if ! curl -fsS "${API_BASE}/api/health" >/dev/null 2>&1; then
  echo "validation API did not become ready" >&2
  kubectl -n "$RELEASE_NAMESPACE" exec "$POD" -- sh -lc 'cat /tmp/arun-live-validate-api.log 2>/dev/null || true' >&2 || true
  cat /tmp/arun-live-validate-port-forward.log >&2 || true
  exit 1
fi

templates_file="$(mktemp)"
payload_file="$(mktemp)"
record_file="$(mktemp)"
trap 'rm -f "$templates_file" "$payload_file" "$record_file"; cleanup_local; cleanup_remote' EXIT

curl -fsS -X POST "${API_BASE}/api/orchestrate/templates" \
  -H 'Content-Type: application/json' \
  -d "$(jq -n --arg repo "$TARGET_REPO" --arg base "$BASE_BRANCH" --arg lang "$UI_LANGUAGE" '{repo:$repo,baseBranch:$base,uiLanguage:$lang}')" \
  >"$templates_file"

if ! jq -e --arg id "$TEMPLATE_ID" '.[] | select(.id == $id)' "$templates_file" >/dev/null; then
  echo "scenario template not found: ${TEMPLATE_ID}" >&2
  jq -r '.[].id' "$templates_file" >&2
  exit 1
fi

rendered_task="$(jq -r --arg id "$TEMPLATE_ID" --arg repo "$TARGET_REPO" --arg base "$BASE_BRANCH" '
  .[] | select(.id == $id) | .taskTemplate
  | gsub("\\{\\{repo\\}\\}"; $repo)
  | gsub("\\{\\{ repo \\}\\}"; $repo)
  | gsub("\\{\\{baseBranch\\}\\}"; $base)
  | gsub("\\{\\{ baseBranch \\}\\}"; $base)
' "$templates_file")"
full_task="$rendered_task"
if [[ -n "${PARENT_TASK}" ]]; then
  full_task="${PARENT_TASK}"$'\n\n'"${rendered_task}"
fi

jq -n \
  --slurpfile templates "$templates_file" \
  --arg id "$TEMPLATE_ID" \
  --arg repo "$TARGET_REPO" \
  --arg base "$BASE_BRANCH" \
  --arg task "$full_task" \
  --arg preset "$LLM_PRESET" \
  --arg outputLanguage "$OUTPUT_LANGUAGE" \
  --arg issueTitle "$ISSUE_TITLE" \
  --arg prTitle "$PR_TITLE" '
  ($templates[0][] | select(.id == $id)) as $template |
  {
    agents: $template.agents,
    scenarioTemplate: {id: $template.id, name: $template.name, source: $template.source},
    repo: $repo,
    baseBranch: $base,
    task: $task,
    strategy: ($template.strategy // "sequential"),
    llmPreset: $preset,
    outputLanguage: ($template.outputLanguage // $outputLanguage),
    github: {
      createIssue: ($template.createIssue // true),
      createPullRequest: ($template.createPullRequest // true),
      branchName: "",
      prBase: $base,
      issueTitle: $issueTitle,
      prTitle: $prTitle,
      issueTemplate: "default",
      prTemplate: "default"
    },
    limits: ($template.limits // {})
  }' >"$payload_file"

echo "starting live validation orchestration via local validation API"
curl -fsS -X POST "${API_BASE}/api/orchestrate" \
  -H 'Content-Type: application/json' \
  --data-binary "@${payload_file}" >"$record_file"

RUN_ID="$(jq -r '.id' "$record_file")"
if [[ -z "$RUN_ID" || "$RUN_ID" == "null" ]]; then
  echo "orchestration start response did not include id" >&2
  cat "$record_file" >&2
  exit 1
fi
echo "started ${RUN_ID}"

deadline=$(( $(date +%s) + MAX_WAIT_SECONDS ))
while true; do
  record="$(curl -fsS "${API_BASE}/api/orchestrates/${RUN_ID}")"
  status="$(printf '%s' "$record" | jq -r '.status')"
  completed="$(printf '%s' "$record" | jq '[.subtasks[]? | select(.status == "completed")] | length')"
  failed="$(printf '%s' "$record" | jq '[.subtasks[]? | select(.status == "failed")] | length')"
  running="$(printf '%s' "$record" | jq -r '[.subtasks[]? | select(.status == "running") | .id] | join(",")')"
  pr_url="$(printf '%s' "$record" | jq -r '.github.pullRequestUrl // .github.pullRequestURL // ""')"
  issue_url="$(printf '%s' "$record" | jq -r '.github.issueUrl // .github.issueURL // ""')"
  echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) status=${status} completed=${completed} failed=${failed} running=${running:-none}"
  if [[ -n "$issue_url" ]]; then
    echo "issue=${issue_url}"
  fi
  if [[ -n "$pr_url" ]]; then
    echo "pr=${pr_url}"
  fi
  case "$status" in
    completed|pending_approval)
      printf '%s\n' "$record" | jq '{id,status,error,github,subtasks: [.subtasks[]? | {id,status,error:(.result.error // null)}]}'
      exit 0
      ;;
    failed|completed_with_publish_error|approval_rejected|canceled)
      printf '%s\n' "$record" | jq '{id,status,error,github,subtasks: [.subtasks[]? | {id,status,error:(.result.error // null)}],events:(.events[-10:] // [])}'
      exit 1
      ;;
  esac
  if (( $(date +%s) >= deadline )); then
    echo "timed out waiting for ${RUN_ID}" >&2
    exit 1
  fi
  sleep "$POLL_INTERVAL_SECONDS"
done
