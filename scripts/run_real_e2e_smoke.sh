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
ENV_PATH_ROOT="$TMPDIR/env-path-root"
ENV_PATH_FILE="$ENV_PATH_ROOT/secrets/github.env"
mkdir -p "$(dirname "$ENV_PATH_FILE")"
cat > "$ENV_PATH_FILE" <<EOF
github_token=envpath_github_token_value
EOF
export PROMPTLOCK_ENV_PATH_ROOT="$ENV_PATH_ROOT"

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

PYTHON_BIN=""
for dep in go jq docker python3 python; do
  if ! command -v "$dep" >/dev/null 2>&1; then
    if [[ "$dep" == "python3" || "$dep" == "python" ]]; then
      continue
    fi
    jq -n --arg dep "$dep" \
      '{ok:false,error:("missing required dependency: "+$dep),watch_tty_ok:false,allow_path_ok:false,deny_path_ok:false,deny_audit_ok:false,container_path_ok:false,skill_image_ok:false}' > "$REPORT"
    cat "$REPORT"
    exit 1
  fi
done

if command -v python3 >/dev/null 2>&1; then
  PYTHON_BIN=python3
elif command -v python >/dev/null 2>&1; then
  PYTHON_BIN=python
else
  jq -n \
    '{ok:false,error:"missing required dependency: python3",watch_tty_ok:false,allow_path_ok:false,deny_path_ok:false,deny_audit_ok:false,container_path_ok:false,skill_image_ok:false}' > "$REPORT"
  cat "$REPORT"
  exit 1
fi

run_pty_command() {
  local transcript_file="$1"
  local input_text="$2"
  shift 2
  "$PYTHON_BIN" - "$transcript_file" "$input_text" "$@" <<'PY'
import errno
import os
import pty
import select
import signal
import subprocess
import sys
import time

out_path = sys.argv[1]
input_steps = [step.encode("utf-8") for step in sys.argv[2].splitlines() if step]
cmd = sys.argv[3:]
master, slave = pty.openpty()
env = os.environ.copy()
env.setdefault("TERM", "xterm-256color")
proc = subprocess.Popen(
    cmd,
    stdin=slave,
    stdout=slave,
    stderr=slave,
    env=env,
    close_fds=True,
    start_new_session=True,
)
os.close(slave)

transcript = bytearray()
deadline = time.monotonic() + 30
next_input = 0
try:
    while True:
        if next_input == 0 and input_steps and b"Actions:" in transcript:
            time.sleep(0.2)
            os.write(master, input_steps[0])
            next_input = 1
        elif next_input == 1 and len(input_steps) > 1 and (b"approved:" in transcript or b"denied:" in transcript):
            time.sleep(0.1)
            os.write(master, input_steps[1])
            next_input = 2
        if proc.poll() is not None:
            try:
                chunk = os.read(master, 4096)
            except OSError as exc:
                if exc.errno in (errno.EIO, errno.EBADF):
                    break
                raise
            if not chunk:
                break
            transcript.extend(chunk)
            continue

        remaining = deadline - time.monotonic()
        if remaining <= 0:
            os.killpg(proc.pid, signal.SIGKILL)
            raise TimeoutError(f"PTY command timed out: {' '.join(cmd)}")
        ready, _, _ = select.select([master], [], [], min(0.1, remaining))
        if master not in ready:
            continue
        try:
            chunk = os.read(master, 4096)
        except OSError as exc:
            if exc.errno in (errno.EIO, errno.EBADF):
                break
            raise
        if not chunk:
            break
        transcript.extend(chunk)
finally:
    try:
        os.close(master)
    except OSError:
        pass
    rc = proc.wait()
    with open(out_path, "wb") as f:
        f.write(transcript)
    if rc != 0:
        sys.exit(rc)
PY
}

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
READY_OUT=""
for _ in $(seq 1 50); do
  READY_OUT="$(go run ./cmd/promptlock watch list --operator-token "$OP_TOKEN" 2>&1)" && {
    READY=true
    break
  } || READY_OUT="$READY_OUT"
  sleep 0.2
done

if [[ "$READY" != "true" ]]; then
  DIAG="broker readiness probe failed via operator unix socket $OPERATOR_SOCKET"
  jq -n --arg error "$DIAG" --arg log "$LOG" --arg tmp "$TMPDIR" --arg ready_out "$READY_OUT" \
    '{ok:false,error:$error,watch_tty_ok:false,allow_path_ok:false,deny_path_ok:false,deny_audit_ok:false,broker_log:$log,tmpdir:$tmp,readiness_output:$ready_out}' > "$REPORT"
  cat "$REPORT"
  {
    echo "real e2e smoke diagnostics:"
    if [[ -n "$READY_OUT" ]]; then
      printf '%s\n' "$READY_OUT"
    fi
    tail -n 40 "$LOG" || true
  } >&2
  exit 1
fi

boot=$(go run ./cmd/promptlock auth bootstrap --operator-token "$OP_TOKEN" --agent "$AGENT_ID" --container "$CONTAINER_ID" | jq -r '.bootstrap_token')
grant=$(go run ./cmd/promptlock auth pair --token "$boot" --container "$CONTAINER_ID" | jq -r '.grant_id')
session=$(go run ./cmd/promptlock auth mint --grant "$grant" | jq -r '.session_token')

export PROMPTLOCK_SESSION_TOKEN="$session"

# interactive TTY approval path
(go run ./cmd/promptlock exec --agent "$AGENT_ID" --task "smoke-watch-tty" --intent run_tests --reason "smoke tty watch" --ttl 5 --wait-approve 20s --poll-interval 1s --broker-exec -- go version >"$TMPDIR/watch-tty.out" 2>"$TMPDIR/watch-tty.err") &
TTY_PID=$!
TTY_REQ_ID="$(find_pending_request_id "smoke-watch-tty" 40 || true)"
WATCH_TTY_OK=false
TTY_DECISION_OK=false
if [[ -n "$TTY_REQ_ID" ]]; then
  TTY_WATCH_OUT="$TMPDIR/watch-tty-watch.out"
  if run_pty_command "$TTY_WATCH_OUT" $'y\nq\n' go run ./cmd/promptlock watch --operator-token "$OP_TOKEN"; then
    if grep -aq 'PromptLock Watch' "$TTY_WATCH_OUT" && grep -aq 'Status: approved:' "$TTY_WATCH_OUT"; then
      TTY_DECISION_OK=true
    fi
  fi
fi
TTY_EXIT=0
wait "$TTY_PID" || TTY_EXIT=$?
if [[ "$TTY_DECISION_OK" == "true" ]] && [[ "$TTY_EXIT" -eq 0 ]]; then
  WATCH_TTY_OK=true
fi

# approve path with env-path context
(go run ./cmd/promptlock exec --agent "$AGENT_ID" --task "smoke-allow" --intent run_tests --reason "smoke allow" --env-path "secrets/github.env" --ttl 5 --wait-approve 20s --poll-interval 1s --broker-exec -- go version >"$TMPDIR/allow.out" 2>"$TMPDIR/allow.err") &
ALLOW_PID=$!
ALLOW_DECISION_OK=false
REQ_ID="$(find_pending_request_id "smoke-allow" 40 || true)"
if [[ -n "$REQ_ID" ]]; then
  ALLOW_WATCH_OUT="$TMPDIR/watch-allow.out"
  if run_pty_command "$ALLOW_WATCH_OUT" $'y\nq\n' go run ./cmd/promptlock watch --operator-token "$OP_TOKEN"; then
    if grep -aq 'PromptLock Watch' "$ALLOW_WATCH_OUT" && grep -aq 'Status: approved:' "$ALLOW_WATCH_OUT" && grep -aq 'Env Path:' "$ALLOW_WATCH_OUT" && grep -aq "secrets/github.env" "$ALLOW_WATCH_OUT"; then
      ALLOW_DECISION_OK=true
    fi
  fi
fi
ALLOW_EXIT=0
wait "$ALLOW_PID" || ALLOW_EXIT=$?
ALLOW_OK=false
if [[ "$ALLOW_EXIT" -eq 0 ]] && [[ "$ALLOW_DECISION_OK" == "true" ]]; then ALLOW_OK=true; fi

# deny path
(go run ./cmd/promptlock exec --agent "$AGENT_ID" --task "smoke-deny" --intent run_tests --reason "smoke deny" --ttl 5 --wait-approve 10s --poll-interval 1s --broker-exec -- echo deny_should_fail >"$TMPDIR/deny.out" 2>"$TMPDIR/deny.err") &
DENY_PID=$!
DENY_DECISION_OK=false
REQ_ID2="$(find_pending_request_id "smoke-deny" 30 || true)"
if [[ -n "$REQ_ID2" ]]; then
  DENY_WATCH_OUT="$TMPDIR/watch-deny.out"
  if run_pty_command "$DENY_WATCH_OUT" $'n\nq\n' go run ./cmd/promptlock watch --operator-token "$OP_TOKEN"; then
    if grep -aq 'PromptLock Watch' "$DENY_WATCH_OUT" && grep -aq 'Status: denied:' "$DENY_WATCH_OUT"; then
      DENY_DECISION_OK=true
    fi
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
    | select(.event.metadata.reason == "denied by operator")
  ' "$AUDIT" >/dev/null 2>&1; then
    DENY_AUDIT_OK=true
  fi
fi

IMAGE="promptlock-agent-lab"
docker build --target agent-lab -t "$IMAGE" . >/dev/null
SKILL_IMAGE_OK=false
if docker run --rm --entrypoint /bin/sh "$IMAGE" -lc 'test -x /usr/local/bin/secretctl.sh && test -f /opt/promptlock/skills/secret-request/SKILL.md'; then
  SKILL_IMAGE_OK=true
fi

(go run ./cmd/promptlock auth docker-run \
  --operator-token "$OP_TOKEN" \
  --agent "$AGENT_ID" \
  --container "${CONTAINER_ID}-container-e2e" \
  --image "$IMAGE" \
  --entrypoint /usr/local/bin/promptlock \
  -- \
  exec \
  --agent "$AGENT_ID" \
  --task "smoke-container" \
  --intent run_tests \
  --reason "smoke container" \
  --ttl 5 \
  --wait-approve 20s \
  --poll-interval 1s \
  --broker-exec \
  -- go version >"$TMPDIR/container.out" 2>"$TMPDIR/container.err") &
CONTAINER_PID=$!
CONTAINER_DECISION_OK=true
REQ_ID3="$(find_pending_request_id "smoke-container" 40 || true)"
if [[ -n "$REQ_ID3" ]]; then
  CONTAINER_WATCH_OUT="$TMPDIR/watch-container.out"
  if run_pty_command "$CONTAINER_WATCH_OUT" $'y\nq\n' go run ./cmd/promptlock watch --operator-token "$OP_TOKEN"; then
    if ! grep -aq 'PromptLock Watch' "$CONTAINER_WATCH_OUT" || ! grep -aq 'Status: approved:' "$CONTAINER_WATCH_OUT" || ! grep -aq 'smoke-container' "$CONTAINER_WATCH_OUT"; then
      CONTAINER_DECISION_OK=false
    fi
  else
    CONTAINER_DECISION_OK=false
  fi
else
  CONTAINER_DECISION_OK=false
fi
CONTAINER_EXIT=0
wait "$CONTAINER_PID" || CONTAINER_EXIT=$?
CONTAINER_OK=false
if [[ "$CONTAINER_EXIT" -eq 0 ]] && [[ "$CONTAINER_DECISION_OK" == "true" ]]; then CONTAINER_OK=true; fi

jq -n \
  --argjson watch_tty "$WATCH_TTY_OK" \
  --argjson allow "$ALLOW_OK" \
  --argjson deny "$DENY_OK" \
  --argjson deny_audit "$DENY_AUDIT_OK" \
  --argjson container "$CONTAINER_OK" \
  --argjson skill_image "$SKILL_IMAGE_OK" \
  '{ok: ($watch_tty and $allow and $deny and $deny_audit and $container and $skill_image), watch_tty_ok: $watch_tty, allow_path_ok: $allow, deny_path_ok: $deny, deny_audit_ok: $deny_audit, container_path_ok: $container, skill_image_ok: $skill_image}' > "$REPORT"
cat "$REPORT"

if [[ "$(jq -r '.ok' "$REPORT")" != "true" ]]; then
  {
    echo "real e2e smoke failed"
    tail -n 40 "$LOG" || true
  } >&2
  exit 1
fi

echo "real e2e smoke passed"
