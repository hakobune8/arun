#!/usr/bin/env bash
set -euo pipefail

ARUN_BASE_URL="${ARUN_BASE_URL:-https://arun.hakobune8.com}"
COOKIE_JAR="${COOKIE_JAR:-${HOME}/.arun/device-session.cookie}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

require_command curl
require_command jq

mkdir -p "$(dirname "$COOKIE_JAR")"

start="$(curl -fsS -X POST "${ARUN_BASE_URL%/}/api/auth/device/start" \
  -H "Content-Type: application/json" \
  -c "$COOKIE_JAR" \
  -b "$COOKIE_JAR")"

device_code="$(printf '%s' "$start" | jq -r '.deviceCode')"
user_code="$(printf '%s' "$start" | jq -r '.userCode')"
verification_uri="$(printf '%s' "$start" | jq -r '.verificationUriComplete // .verificationUri')"
interval="$(printf '%s' "$start" | jq -r '.interval // 5')"
expires_in="$(printf '%s' "$start" | jq -r '.expiresIn // 900')"

if [[ -z "$device_code" || "$device_code" == "null" || -z "$user_code" || "$user_code" == "null" ]]; then
  echo "device start response did not include a device code" >&2
  printf '%s\n' "$start" >&2
  exit 1
fi

cat <<EOF
Open this URL and enter the code:
  ${verification_uri}

Code:
  ${user_code}

Polling ARUN until GitHub authorization completes...
EOF

deadline=$(( $(date +%s) + expires_in ))
while true; do
  response_file="$(mktemp)"
  status="$(curl -sS -o "$response_file" -w '%{http_code}' \
    -X POST "${ARUN_BASE_URL%/}/api/auth/device/poll" \
    -H "Content-Type: application/json" \
    -c "$COOKIE_JAR" \
    -b "$COOKIE_JAR" \
    --data-binary "$(jq -n --arg code "$device_code" '{deviceCode:$code}')")"
  response="$(cat "$response_file")"
  rm -f "$response_file"

  case "$status" in
    200)
      login="$(printf '%s' "$response" | jq -r '.user.login // "authenticated"')"
      echo "Authenticated as ${login}."
      echo "Cookie jar: ${COOKIE_JAR}"
      exit 0
      ;;
    202)
      next_interval="$(printf '%s' "$response" | jq -r '.interval // empty')"
      if [[ -n "$next_interval" && "$next_interval" != "null" && "$next_interval" != "0" ]]; then
        interval="$next_interval"
      fi
      ;;
    *)
      echo "device poll failed with HTTP ${status}" >&2
      printf '%s\n' "$response" >&2
      exit 1
      ;;
  esac

  if (( $(date +%s) >= deadline )); then
    echo "device authorization timed out" >&2
    exit 1
  fi
  sleep "$interval"
done
