#!/usr/bin/env bash
set -euo pipefail

BROKER_URL="${BROKER_URL:-${PROMPTLOCK_BROKER_URL:-http://127.0.0.1:8765}}"
BROKER_UNIX_SOCKET="${BROKER_UNIX_SOCKET:-${PROMPTLOCK_AGENT_UNIX_SOCKET:-}}"
SESSION_TOKEN="${SESSION_TOKEN:-${PROMPTLOCK_SESSION_TOKEN:-}}"

usage() {
  cat <<'USAGE'
Usage:
  secretctl.sh request --agent ID --task ID --ttl MIN --reason TEXT --secret NAME [--secret NAME...]
  secretctl.sh access --lease TOKEN --secret NAME

Env:
  BROKER_URL (or PROMPTLOCK_BROKER_URL; default: http://127.0.0.1:8765 when no socket is set)
  BROKER_UNIX_SOCKET (or PROMPTLOCK_AGENT_UNIX_SOCKET; preferred for local hardened dual-socket deployments)
  SESSION_TOKEN (or PROMPTLOCK_SESSION_TOKEN; required when broker auth is enabled)
USAGE
}

broker_request() {
  local method="$1"
  local path="$2"
  shift 2
  if [[ -n "$BROKER_UNIX_SOCKET" ]]; then
    curl --unix-socket "$BROKER_UNIX_SOCKET" -sS -X "$method" "http://promptlock.local${path}" "$@"
    return
  fi
  curl -sS -X "$method" "${BROKER_URL}${path}" "$@"
}

cmd="${1:-}"
shift || true

if [[ -z "$cmd" ]]; then
  usage; exit 1
fi

if [[ "$cmd" == "request" ]]; then
  agent=""; task=""; ttl=""; reason=""; secrets=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --agent) agent="$2"; shift 2;;
      --task) task="$2"; shift 2;;
      --ttl) ttl="$2"; shift 2;;
      --reason) reason="$2"; shift 2;;
      --secret) secrets+=("$2"); shift 2;;
      *) echo "unknown arg: $1"; exit 1;;
    esac
  done

  if [[ -z "$agent" || -z "$task" || -z "$ttl" || -z "$reason" || ${#secrets[@]} -eq 0 ]]; then
    echo "missing required args" >&2; usage; exit 1
  fi

  json_secrets="$(printf '%s\n' "${secrets[@]}" | jq -R . | jq -s .)"
  jq -n \
    --arg a "$agent" \
    --arg t "$task" \
    --arg r "$reason" \
    --argjson ttl "$ttl" \
    --argjson s "$json_secrets" \
    '{agent_id:$a, task_id:$t, reason:$r, ttl_minutes:$ttl, secrets:$s}' \
  | broker_request POST "/v1/leases/request" \
      -H "Authorization: Bearer $SESSION_TOKEN" \
      -H 'content-type: application/json' -d @-
  echo
  exit 0
fi

if [[ "$cmd" == "access" ]]; then
  lease=""; secret=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --lease) lease="$2"; shift 2;;
      --secret) secret="$2"; shift 2;;
      *) echo "unknown arg: $1"; exit 1;;
    esac
  done

  jq -n --arg l "$lease" --arg s "$secret" '{lease_token:$l, secret:$s}' \
    | broker_request POST "/v1/leases/access" \
        -H "Authorization: Bearer $SESSION_TOKEN" \
        -H 'content-type: application/json' -d @-
  echo
  exit 0
fi

echo "unknown command: $cmd" >&2
usage
exit 1
