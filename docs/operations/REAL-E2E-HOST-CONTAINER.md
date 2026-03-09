# Real E2E: Host Daemon + Container Agent (CLI-first Lab Flow)

This document shows the canonical CLI-first lab validation flow for PromptLock with:
- host running `promptlockd`
- host operator approving requests interactively
- containerized agent executing via `promptlock` CLI

No curl is required in the primary flow.

This is not the hardened production TCP baseline. The current CLI docs do not yet cover direct client-certificate handling, so this walkthrough uses the explicit insecure TCP override for controlled local validation only. For hardened non-local TCP deployments, use [MTLS-HARDENED.md](./MTLS-HARDENED.md) or an authenticated mTLS proxy in front of PromptLock.

## Prerequisites
- Host has PromptLock repo and Go toolchain.
- Docker can run a container that reaches host (`host.docker.internal`).
- You have chosen an operator token and container identity.
- You understand that `PROMPTLOCK_ALLOW_INSECURE_TCP=1` below is for local validation only and must not be used as a production deployment pattern.

## Topology guidance
- Preferred production baseline: `unix_socket` for local-only deployments, or TLS/mTLS for any non-local TCP listener.
- This walkthrough: hardened policy controls plus explicit insecure TCP override to validate the host-plus-container approval path with the current CLI surface.

## 1) Host: start daemon for lab validation

Create config (example):

```json
{
  "security_profile": "hardened",
  "address": "0.0.0.0:8765",
  "unix_socket": "",
  "audit_path": "/tmp/promptlock-audit.jsonl",
  "state_store_file": "/tmp/promptlock-state-store.json",
  "auth": {
    "enable_auth": true,
    "operator_token": "op_real_test_token",
    "allow_plaintext_secret_return": false,
    "session_ttl_minutes": 30,
    "grant_idle_timeout_minutes": 240,
    "grant_absolute_max_minutes": 1440,
    "bootstrap_token_ttl_seconds": 300,
    "cleanup_interval_seconds": 60,
    "rate_limit_window_seconds": 60,
    "rate_limit_max_attempts": 50,
    "store_file": "/tmp/promptlock-auth-store.json",
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
    "intent_allow_domains": { "run_tests": ["api.github.com"] },
    "deny_substrings": ["169.254.169.254", "metadata.google.internal", "localhost", "127.0.0.1"]
  },
  "intents": { "run_tests": ["github_token"] }
}
```

Export host-provided secret and start daemon:

```bash
export PROMPTLOCK_SECRET_GITHUB_TOKEN='demo_github_token_value'
export PROMPTLOCK_AUTH_STORE_KEY='replace_with_long_random_value'
PROMPTLOCK_ALLOW_INSECURE_TCP=1 PROMPTLOCK_CONFIG=/tmp/promptlock-real.json go run ./cmd/promptlockd
```

This startup mode is acceptable only for controlled local testing. If you need a hardened remote-access path, stop here and switch to [MTLS-HARDENED.md](./MTLS-HARDENED.md).

## 2) Host: run interactive approval queue

In a second host terminal:

```bash
PROMPTLOCK_BROKER_URL=http://127.0.0.1:8765 \
PROMPTLOCK_OPERATOR_TOKEN=op_real_test_token \
go run ./cmd/promptlock approve-queue
```

## 3) Container: obtain session token via CLI auth commands

Inside container (or host, if you prefer), run:

```bash
BROKER=http://host.docker.internal:8765
OP=op_real_test_token
AGENT=toolbelt-agent
CID=toolbelt-container-1

BOOT=$(go run ./cmd/promptlock auth bootstrap --broker "$BROKER" --operator-token "$OP" --agent "$AGENT" --container "$CID" | jq -r '.bootstrap_token')
GRANT=$(go run ./cmd/promptlock auth pair --broker "$BROKER" --token "$BOOT" --container "$CID" | jq -r '.grant_id')
SESSION=$(go run ./cmd/promptlock auth mint --broker "$BROKER" --grant "$GRANT" | jq -r '.session_token')

echo "$SESSION"
```

## 4) Container: request and execute with approval

```bash
PROMPTLOCK_BROKER_URL=http://host.docker.internal:8765 \
PROMPTLOCK_SESSION_TOKEN="$SESSION" \
go run ./cmd/promptlock exec \
  --agent toolbelt-agent \
  --task real-e2e \
  --intent run_tests \
  --reason "real host approval test" \
  --ttl 20 \
  --wait-approve 5m \
  --poll-interval 2s \
  --broker-exec \
  -- echo "hello from toolbelt"
```

Approve request in host `approve-queue` terminal when prompted.

## 5) Verify audit

```bash
go run ./cmd/promptlock audit-verify --file /tmp/promptlock-audit.jsonl
```

Expected: `audit verify ok: ...`

## Troubleshooting quick map
- `connection refused`: broker not running/bound on expected host/port.
- `operator auth required`: wrong/missing operator token for auth bootstrap or queue commands.
- `agent session token required`: session token missing for agent commands.
- `request denied`: operator denied in approval queue.
- `secret backend unavailable`: secret source backend misconfigured/unavailable.
- Want a secure non-local TCP deployment instead of this lab path: use [MTLS-HARDENED.md](./MTLS-HARDENED.md).
- For command-to-endpoint/token mapping and remediation by failure text: use [CLI-ENDPOINT-CONTRACT-MATRIX.md](./CLI-ENDPOINT-CONTRACT-MATRIX.md).
