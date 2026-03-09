#!/usr/bin/env bash
set -euo pipefail

BROKER_URL="${BROKER_URL:-http://127.0.0.1:8765}"
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

jq -n --argjson ttl "$TTL" '{ttl_minutes:$ttl}' \
  | curl -sS -X POST "$BROKER_URL/v1/leases/approve?request_id=$REQ_ID" \
      -H "Authorization: Bearer $OPERATOR_TOKEN" \
      -H 'content-type: application/json' -d @-

echo
