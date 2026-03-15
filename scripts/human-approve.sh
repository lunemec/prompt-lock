#!/usr/bin/env bash
set -euo pipefail

BROKER_URL="${BROKER_URL:-http://127.0.0.1:8765}"
BROKER_UNIX_SOCKET="${BROKER_UNIX_SOCKET:-}"
APPROVE_ENDPOINT_STYLE="${APPROVE_ENDPOINT_STYLE:-auto}"
OPERATOR_TOKEN="${OPERATOR_TOKEN:-}"
REQ_ID="${1:-}"
TTL="${2:-20}"

if [[ -z "$REQ_ID" ]]; then
  echo "Usage: human-approve.sh <request_id> [ttl_minutes]" >&2
  exit 1
fi

if [[ -z "$OPERATOR_TOKEN" ]]; then
  echo "OPERATOR_TOKEN is required" >&2
  exit 1
fi

broker_request() {
  local path="$1"
  local response_file status
  response_file="$(mktemp)"
  trap 'rm -f "$response_file"' RETURN
  if [[ -n "$BROKER_UNIX_SOCKET" ]]; then
    status="$(
      jq -n --argjson ttl "$TTL" '{ttl_minutes:$ttl}' \
        | curl --unix-socket "$BROKER_UNIX_SOCKET" -sS -o "$response_file" -w '%{http_code}' -X POST "http://promptlock.local${path}" \
            -H "Authorization: Bearer $OPERATOR_TOKEN" \
            -H 'content-type: application/json' -d @-
    )"
  else
    status="$(
      jq -n --argjson ttl "$TTL" '{ttl_minutes:$ttl}' \
        | curl -sS -o "$response_file" -w '%{http_code}' -X POST "${BROKER_URL}${path}" \
            -H "Authorization: Bearer $OPERATOR_TOKEN" \
            -H 'content-type: application/json' -d @-
    )"
  fi
  BROKER_RESPONSE_BODY="$(cat "$response_file")"
  [[ "$status" =~ ^2[0-9][0-9]$ ]]
}

case "$APPROVE_ENDPOINT_STYLE" in
  auto)
    if broker_request "/v1/leases/approve?request_id=$REQ_ID"; then
      printf '%s' "$BROKER_RESPONSE_BODY"
    elif broker_request "/v1/leases/$REQ_ID/approve"; then
      printf '%s' "$BROKER_RESPONSE_BODY"
    else
      printf '%s' "$BROKER_RESPONSE_BODY"
      exit 1
    fi
    ;;
  query)
    broker_request "/v1/leases/approve?request_id=$REQ_ID"
    printf '%s' "$BROKER_RESPONSE_BODY"
    ;;
  path)
    broker_request "/v1/leases/$REQ_ID/approve"
    printf '%s' "$BROKER_RESPONSE_BODY"
    ;;
  *)
    echo "APPROVE_ENDPOINT_STYLE must be one of: auto, query, path" >&2
    exit 1
    ;;
esac

echo
