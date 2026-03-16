# Real E2E: Host Daemon + Container Agent (CLI-first Lab Flow)

This document shows the canonical CLI-first lab validation flow for PromptLock with:
- host running `promptlockd`
- host operator approving requests interactively
- containerized agent executing via `promptlock` CLI

No curl is required in the primary flow.

This is the hardened local baseline and the supported OSS deployment shape: PromptLock runs on the host with role-separated Unix sockets, the operator uses the host-only operator socket, and the container gets only the agent socket.

## Prerequisites
- Host has PromptLock repo and Go toolchain.
- Host has `jq` installed to parse CLI JSON responses during bootstrap/pair/mint.
- You have chosen an operator token and container identity.
- The `--broker-exec` path runs the approved command on the broker host, not inside the agent container. The container only needs the `promptlock` CLI.

## Topology guidance
- Supported OSS baseline: dual Unix sockets for local-only hardened deployments.
- This walkthrough: hardened policy controls plus agent/operator socket separation for local lab verification.

## 1) Host: start daemon for lab validation

Create config (example):

```json
{
  "security_profile": "hardened",
  "agent_unix_socket": "/tmp/promptlock-agent.sock",
  "operator_unix_socket": "/tmp/promptlock-operator.sock",
  "audit_path": "/tmp/promptlock-audit.jsonl",
  "state_store_file": "/tmp/promptlock-state-store.json",
  "state_store": {
    "type": "file"
  },
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
  "execution_policy": {
    "exact_match_executables": ["echo"],
    "denylist_substrings": ["&&", "||", ";", "$(", "`"],
    "output_security_mode": "raw",
    "max_output_bytes": 65536,
    "default_timeout_sec": 30,
    "max_timeout_sec": 60
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
PROMPTLOCK_CONFIG=/tmp/promptlock-real.json go run ./cmd/promptlockd
```

This startup mode is the preferred hardened local path and the supported OSS release target.
If you switch to `state_store.type=external`, treat it as a durability/availability adapter rather than a concurrency-safe multi-node coordinator.
The example `execution_policy` above allowlists the exact executable name `echo` so step 5 can verify broker-exec with a harmless direct command. `echo-helper` or other prefixed names are still rejected. Broker resolution uses the broker-managed `command_search_paths` defaults, so the host only resolves `echo` from trusted system tool directories rather than the broker process `PATH`. The example also overrides hardened output suppression to `raw` only because the approved command prints a fixed non-sensitive string. Keep the hardened default `none` unless visible output is explicitly required.

## 2) Host: run interactive watch UI

In a second host terminal:

```bash
PROMPTLOCK_OPERATOR_TOKEN=op_real_test_token go run ./cmd/promptlock watch
```

## 3) Container: build an agent CLI image

Build the repo image once so the container has the shipped `promptlock` CLI available:

```bash
docker build -t promptlock-agent-lab .
```

## 4) Container: launch via one-command auth wrapper

Run on the host:

This is still a host-side command. It bootstraps on the operator socket, pairs and mints on the agent socket, then launches the container with only the agent socket mounted.

```bash
go run ./cmd/promptlock auth docker-run \
  --operator-token op_real_test_token \
  --agent toolbelt-agent \
  --container toolbelt-container-1 \
  --image promptlock-agent-lab \
  --entrypoint /usr/local/bin/promptlock \
  -- \
  exec \
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

This wrapper performs `auth login`, injects the fresh session token into the container environment, and then runs `docker run` with the requested image/command.
In local hardened mode it mounts only the agent socket into the container and leaves the operator socket on the host.

## 5) Approve in the host watch UI

Approve the request in the host `watch` terminal when prompted.

Expected output after approval:

```text
hello from toolbelt
```

## 6) Verify audit

```bash
go run ./cmd/promptlock audit-verify --file /tmp/promptlock-audit.jsonl
```

Expected: `audit verify ok: ...`

## Troubleshooting quick map
- `connection refused`: broker not running or the expected socket path is missing.
- `operator auth required`: wrong/missing operator token for auth wrapper or watch commands.
- `agent session token required`: session token missing for agent commands.
- `command "echo" not allowed by execution policy`: the broker config is missing the example `execution_policy.exact_match_executables=["echo"]` block from step 1, or you changed the lab command without updating the allowlist.
- `request denied`: operator denied in the watch UI.
- `secret backend unavailable`: secret source backend misconfigured/unavailable.
- For command-to-endpoint/token mapping and remediation by failure text: use [CLI-ENDPOINT-CONTRACT-MATRIX.md](./CLI-ENDPOINT-CONTRACT-MATRIX.md).
