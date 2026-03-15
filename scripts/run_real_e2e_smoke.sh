#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
mkdir -p reports

OP_TOKEN="${PROMPTLOCK_OPERATOR_TOKEN:-op_smoke_token}"
AGENT_ID="${PROMPTLOCK_SMOKE_AGENT_ID:-smoke-agent}"
CONTAINER_ID="${PROMPTLOCK_SMOKE_CONTAINER_ID:-smoke-container}"

TMPDIR="$(mktemp -d)"
CFG="$TMPDIR/config.json"
AUDIT="$TMPDIR/audit.jsonl"
AUTH_STORE="$TMPDIR/auth-store.json"
STATE_STORE="$TMPDIR/state-store.json"
LOG="$TMPDIR/broker.log"
REPORT="reports/real-e2e-smoke.json"
KEEP_TMPDIR="${PROMPTLOCK_SMOKE_KEEP_TMPDIR:-0}"
AGENT_SOCKET="$TMPDIR/promptlock-agent.sock"
OPERATOR_SOCKET="$TMPDIR/promptlock-operator.sock"

find_pending_request_id() {
  local task="$1"
  local attempts="${2:-40}"
  local out req
  for _ in $(seq 1 "$attempts"); do
    out="$(go run ./cmd/promptlock watch list --operator-token "$OP_TOKEN" 2>/dev/null || true)"
    req="$(printf '%s\n' "$out" | awk -v task="$task" '$1 ~ /^req_/ && index($0, "task=" task " ") > 0 {print $1; exit}')"
    if [[ -n "$req" ]]; then
      echo "$req"
      return 0
    fi
    sleep 0.5
  done
  return 1
}

for dep in go jq; do
  if ! command -v "$dep" >/dev/null 2>&1; then
    jq -n --arg dep "$dep" \
      '{ok:false,error:("missing required dependency: "+$dep),allow_path_ok:false,deny_path_ok:false,deny_audit_ok:false}' > "$REPORT"
    cat "$REPORT"
    exit 1
  fi
done

cat > "$CFG" <<JSON
{
  "security_profile": "hardened",
  "agent_unix_socket": "$AGENT_SOCKET",
  "operator_unix_socket": "$OPERATOR_SOCKET",
  "audit_path": "$AUDIT",
  "state_store_file": "$STATE_STORE",
  "auth": {
    "enable_auth": true,
    "operator_token": "$OP_TOKEN",
    "allow_plaintext_secret_return": false,
    "session_ttl_minutes": 30,
    "grant_idle_timeout_minutes": 120,
    "grant_absolute_max_minutes": 240,
    "bootstrap_token_ttl_seconds": 300,
    "cleanup_interval_seconds": 60,
    "rate_limit_window_seconds": 60,
    "rate_limit_max_attempts": 50,
    "store_file": "$AUTH_STORE",
    "store_encryption_key_env": "PROMPTLOCK_AUTH_STORE_KEY"
  },
  "secret_source": {
    "type": "env",
    "env_prefix": "PROMPTLOCK_SECRET_",
    "in_memory_hardened": "fail"
  },
  "network_egress_policy": {
    "enabled": true,
    "require_intent_match": true,
    "allow_domains": ["api.github.com"],
    "intent_allow_domains": {"run_tests": ["api.github.com"]},
    "deny_substrings": ["169.254.169.254", "metadata.google.internal", "localhost", "127.0.0.1"]
  },
  "intents": {"run_tests": ["github_token"]}
}
JSON

export PROMPTLOCK_SECRET_GITHUB_TOKEN="demo_github_token_value"
export PROMPTLOCK_AGENT_UNIX_SOCKET="$AGENT_SOCKET"
export PROMPTLOCK_OPERATOR_UNIX_SOCKET="$OPERATOR_SOCKET"
PROMPTLOCK_AUTH_STORE_KEY="smoke_auth_store_key_012345" PROMPTLOCK_CONFIG="$CFG" go run ./cmd/promptlockd >"$LOG" 2>&1 &
BROKER_PID=$!
cleanup() {
  kill "$BROKER_PID" >/dev/null 2>&1 || true
  if [[ "$KEEP_TMPDIR" != "1" ]]; then
    rm -rf "$TMPDIR"
  else
    echo "preserving smoke tempdir: $TMPDIR" >&2
  fi
}
trap cleanup EXIT

READY=false
for _ in $(seq 1 50); do
  if go run ./cmd/promptlock watch list --operator-token "$OP_TOKEN" >/dev/null 2>&1; then
    READY=true
    break
  fi
  sleep 0.2
done

if [[ "$READY" != "true" ]]; then
  DIAG="broker readiness probe failed via operator unix socket $OPERATOR_SOCKET"
  jq -n --arg error "$DIAG" --arg log "$LOG" --arg tmp "$TMPDIR" \
    '{ok:false,error:$error,allow_path_ok:false,deny_path_ok:false,deny_audit_ok:false,broker_log:$log,tmpdir:$tmp}' > "$REPORT"
  cat "$REPORT"
  {
    echo "real e2e smoke diagnostics:"
    tail -n 40 "$LOG" || true
  } >&2
  exit 1
fi

boot=$(go run ./cmd/promptlock auth bootstrap --operator-token "$OP_TOKEN" --agent "$AGENT_ID" --container "$CONTAINER_ID" | jq -r '.bootstrap_token')
grant=$(go run ./cmd/promptlock auth pair --token "$boot" --container "$CONTAINER_ID" | jq -r '.grant_id')
session=$(go run ./cmd/promptlock auth mint --grant "$grant" | jq -r '.session_token')

export PROMPTLOCK_SESSION_TOKEN="$session"

# approve path
(go run ./cmd/promptlock exec --agent "$AGENT_ID" --task "smoke-allow" --intent run_tests --reason "smoke allow" --ttl 5 --wait-approve 20s --poll-interval 1s --broker-exec -- go version >"$TMPDIR/allow.out" 2>"$TMPDIR/allow.err") &
ALLOW_PID=$!
ALLOW_DECISION_OK=true
REQ_ID="$(find_pending_request_id "smoke-allow" 40 || true)"
if [[ -n "$REQ_ID" ]]; then
  if ! go run ./cmd/promptlock watch allow --operator-token "$OP_TOKEN" "$REQ_ID" >/dev/null; then
    ALLOW_DECISION_OK=false
  fi
else
  ALLOW_DECISION_OK=false
fi
ALLOW_EXIT=0
wait "$ALLOW_PID" || ALLOW_EXIT=$?
ALLOW_OK=false
if [[ "$ALLOW_EXIT" -eq 0 ]] && [[ "$ALLOW_DECISION_OK" == "true" ]]; then ALLOW_OK=true; fi

# deny path
(go run ./cmd/promptlock exec --agent "$AGENT_ID" --task "smoke-deny" --intent run_tests --reason "smoke deny" --ttl 5 --wait-approve 10s --poll-interval 1s --broker-exec -- echo deny_should_fail >"$TMPDIR/deny.out" 2>"$TMPDIR/deny.err") &
DENY_PID=$!
DENY_DECISION_OK=true
REQ_ID2="$(find_pending_request_id "smoke-deny" 30 || true)"
if [[ -n "$REQ_ID2" ]]; then
  if ! go run ./cmd/promptlock watch deny --operator-token "$OP_TOKEN" --reason "smoke deny" "$REQ_ID2" >/dev/null; then
    DENY_DECISION_OK=false
  fi
else
  DENY_DECISION_OK=false
fi
DENY_EXIT=0
wait "$DENY_PID" || DENY_EXIT=$?
DENY_OK=false
if [[ "$DENY_EXIT" -ne 0 ]] && [[ "$DENY_DECISION_OK" == "true" ]] && grep -qi "request denied" "$TMPDIR/deny.err"; then DENY_OK=true; fi

DENY_AUDIT_OK=false
if [[ -n "${REQ_ID2:-}" ]] && [[ -f "$AUDIT" ]]; then
  if jq -e --arg req "$REQ_ID2" '
    select(.event.event == "request_denied")
    | select(.event.request_id == $req)
    | select(.event.metadata.reason == "smoke deny")
  ' "$AUDIT" >/dev/null 2>&1; then
    DENY_AUDIT_OK=true
  fi
fi

jq -n \
  --argjson allow "$ALLOW_OK" \
  --argjson deny "$DENY_OK" \
  --argjson deny_audit "$DENY_AUDIT_OK" \
  '{ok: ($allow and $deny and $deny_audit), allow_path_ok: $allow, deny_path_ok: $deny, deny_audit_ok: $deny_audit}' > "$REPORT"
cat "$REPORT"

if [[ "$(jq -r '.ok' "$REPORT")" != "true" ]]; then
  {
    echo "real e2e smoke failed"
    tail -n 40 "$LOG" || true
  } >&2
  exit 1
fi

echo "real e2e smoke passed"
