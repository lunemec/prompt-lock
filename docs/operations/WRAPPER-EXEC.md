# promptlock exec

`promptlock exec` is a capability-first wrapper intended to run commands with lease-scoped secret injection.

## Example

```bash
# Terminal A: human watch UI (interactive)
PROMPTLOCK_OPERATOR_TOKEN=op_local_test_token \
  go run ./cmd/promptlock watch

# Terminal B: agent command waiting for approval
PROMPTLOCK_SESSION_TOKEN=sess_local_test \
  go run ./cmd/promptlock exec \
  --agent ralph-r1 \
  --task TASK-3001 \
  --intent run_tests \
  --ttl 5 \
  --broker-exec \
  -- go test ./...
```

## Watch commands

```bash
# list pending requests
PROMPTLOCK_OPERATOR_TOKEN=... \
  go run ./cmd/promptlock watch list

# allow specific request
PROMPTLOCK_OPERATOR_TOKEN=... \
  go run ./cmd/promptlock watch allow --ttl 5 <request_id>

# deny specific request
PROMPTLOCK_OPERATOR_TOKEN=... \
  go run ./cmd/promptlock watch deny --reason "scope too broad" <request_id>
```

For the smoothest local hardened quickstart, run `go run ./cmd/promptlock setup` (or `make setup-local-docker`) from the repo first, then source the generated `instance.env` before the commands below. That keeps config/state outside the repo workspace while exporting the role-specific socket paths and operator/session bootstrap env needed by the container-first flow.

These examples otherwise assume the supported local hardened default: `promptlock watch` auto-selects `/tmp/promptlock-operator.sock`. Add `--broker` only when you intentionally want TCP transport.

## Container launch shortcut

For containerized agents, the CLI can mint a fresh session and launch `docker run` in one step:

- Run `promptlock auth docker-run` on the host, not inside the container.
- The wrapper uses the operator socket for bootstrap, the agent socket for pair/mint, and mounts only the agent socket into the container.

```bash
go run ./cmd/promptlock auth docker-run \
  --operator-token op_local_test_token \
  --agent toolbelt-agent \
  --container toolbelt-container-1 \
  --image promptlock-agent-lab \
  --entrypoint /usr/local/bin/promptlock \
  -- \
  exec \
  --agent toolbelt-agent \
  --task wrapper-lab \
  --intent run_tests \
  --ttl 20 \
  --wait-approve 5m \
  --poll-interval 2s \
  --broker-exec \
  -- echo "hello from wrapper"
```

Useful flags:
- `--mount` to pass through workspace mounts.
- `--env` to add container env vars, except reserved PromptLock transport variables like `PROMPTLOCK_SESSION_TOKEN`, `PROMPTLOCK_BROKER_URL`, and `PROMPTLOCK_AGENT_UNIX_SOCKET`.
- `--workdir` to set the in-container working directory.
- `--docker-arg` as a narrow escape hatch for a small allowlist of extra `docker run` flags (`--pull`, `--init`, `--label`, `--label-file`, `--hostname`, `--add-host`, `--dns`, `--dns-option`, `--dns-search`, `--shm-size`, `--stop-timeout`, `--tmpfs`, `--ulimit`); PromptLock rejects raw env, env-file, mount, volume, user, workdir, and entrypoint overrides in both `--flag=value` and `--flag value` forms so the session/socket boundary cannot be bypassed accidentally.

For local demo only (no external approval watcher):

```bash
PROMPTLOCK_DEV_MODE=1 PROMPTLOCK_BROKER_URL=http://127.0.0.1:8765 \
  go run ./cmd/promptlock exec --intent run_tests --ttl 5 --auto-approve -- env
```

## Notes
- `--intent` resolves secrets via broker intent map.
- `--secrets` can be used explicitly instead of intent.
- Wrapper computes command and working-directory fingerprints and includes them in lease/access calls.
- Wrapper checks broker capabilities and fails fast if hardened mode disables plaintext secret return unless `--broker-exec` is used.
- `--broker-exec` uses `/v1/leases/execute` and is the preferred secure mode.
- In hardened policy, `--broker-exec` requires `--intent` for intent-aware egress enforcement.
- Broker-exec binds execute-time egress validation to the approved request intent; supplying a different execute payload intent does not widen scope.
- In hardened policy, server-side execution rejects raw shell wrappers (`bash`/`sh`/`zsh`) and expects intent-bound direct commands.
- Direct network clients validated through argv inspection (`curl`, `wget`, `fetch`) are denied when no inspectable destination is present or when argv includes opaque, unclassified, or destination-override inputs the broker cannot truthfully inspect and enforce.
- Broker-side execution policy can enforce exact executable allowlisting, broker-managed executable resolution, denylist checks, output limits, and timeouts.
- Both broker-side and local CLI exec paths build the child-process environment from a minimal baseline (`PATH`, `HOME`, temp-dir vars, and platform-required Windows keys) plus leased secrets only. Ambient shell env vars are not forwarded by default.
- Broker-host execution uses `execution_policy.command_search_paths` as its managed `PATH` and only resolves bare executable names from those directories.
- `execution_policy.exact_match_executables` is the only supported config key for broker-exec executable allowlisting.
- `redacted` output mode applies token-aware best-effort masking for common bearer and env-style secret shapes. It is not a strong barrier against secret exfiltration through command output.
- When auth is enabled, wrapper uses `--session-token` (or `PROMPTLOCK_SESSION_TOKEN`) for agent endpoints.
- `promptlock auth docker-run` can mint a short-lived session and inject it into a new `docker run` invocation with secure defaults (`--read-only`, `--cap-drop ALL`, `--security-opt no-new-privileges`, tmpfs `/tmp`, current user identity).
- `promptlock auth login` omits raw bearer output by default; use `--show-grant-id` or `--show-secrets` only when you intentionally need those values for plumbing/debugging.
- In local hardened mode, wrapper commands default to role-separated sockets with no broker flags:
  - operator flows use `/tmp/promptlock-operator.sock`
  - agent flows use `/tmp/promptlock-agent.sock`
- `promptlock auth docker-run` mounts only the agent socket into the container, injects `PROMPTLOCK_AGENT_UNIX_SOCKET`, and passes `PROMPTLOCK_SESSION_TOKEN` through the child environment rather than embedding bearer material in `docker run` argv.
- Wrapper still supports explicit TCP broker URL (`--broker`) and compatibility unix socket transport (`--broker-unix-socket`) when needed, but Unix-socket selection is only valid when the chosen path is an actual Unix-socket node.
- Legacy `PROMPTLOCK_BROKER_UNIX_SOCKET` remains available for single-socket compatibility, but it must reference an actual Unix-socket node; missing paths or non-socket filesystem nodes fail closed instead of falling through to another transport.
- If the expected local role socket is missing, wrapper commands fail closed instead of silently downgrading to localhost TCP. Use `--broker` only when you intentionally want TCP transport.
- `promptlock watch allow` and `promptlock watch deny` accept either `--broker` or `--broker-unix-socket`; otherwise they auto-select the operator socket in the supported dual-socket flow when that path is an actual Unix socket.
- `promptlock auth docker-run --container-broker-socket` sets the in-container mount path whenever the selected agent transport uses an actual Unix socket, including the default dual-socket and legacy compatibility socket paths.
- Broker-facing CLI requests use a bounded `10s` client deadline on both Unix-socket and TCP transports. Stalled peers fail with `broker request timed out after 10s`.
- Default mode waits for external human approval (`--wait-approve`, `--poll-interval`).
- `promptlock watch` is a host-side queue watcher with a minimal terminal UI for approving/denying pending requests.
- In a terminal, `promptlock watch` clears and redraws when the queue changes so new requests are visually distinct.
- `--auto-approve` exists only for local prototyping and requires `PROMPTLOCK_DEV_MODE=1` and operator token.
- Basic command policy blocks risky secret-dumping command patterns unless `--allow-risky-command` is explicitly set.

## `--env-path`
- `--env-path` attaches a `.env` file path to the lease request as approval context.
- The broker computes `env_path_canonical` from `--env-path`; wrapper/API callers should not send `env_path_canonical` as request input.
- Operators see both the original `env_path` and the broker-confirmed `env_path_canonical` in `promptlock watch`.
- Approved `env_path` requests switch execute-time secret lookup from broker process env to the approved `.env` file.
- The broker resolves `env_path` only within `PROMPTLOCK_ENV_PATH_ROOT`; traversal and symlink escapes are rejected.
- In `security_profile=dev`, an unset `PROMPTLOCK_ENV_PATH_ROOT` falls back to the broker working directory for local testing.
- In non-dev profiles, `--env-path` requests fail closed until `PROMPTLOCK_ENV_PATH_ROOT` is set explicitly.
- Requests with `--env-path` do not reuse active leases across identical requests because the approved file path is part of the decision context.

## Security direction
- Long-term default should require external human approval path.
- Wrapper should avoid exposing plaintext secrets in command output where feasible.
- Add command-policy controls for high-risk command forms.
