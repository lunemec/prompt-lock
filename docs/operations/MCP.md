# MCP adapter (experimental)

PromptLock now includes an experimental MCP stdio adapter:

- binary path: `cmd/promptlock-mcp`
- doctor command: `promptlock mcp doctor`
- baseline protocol methods: `initialize`, `initialized`, `notifications/initialized`, `notifications/cancelled`, `ping`, `shutdown`, `exit`, `tools/list`, `resources/list`, `prompts/list`, `tools/call`
- tool exposed: `execute_with_intent`

`execute_with_intent` currently accepts:

- `intent` (must exactly match a configured broker intent id; quickstart example: `run_tests`)
- `command`
- optional `ttl_minutes`
- optional `env_path`

`tools/list` also publishes this guidance in the tool description and JSON schema property descriptions, including a quickstart example payload for `make demo-print-github-token`.

Before launching a client, run `promptlock mcp doctor` in the same shell where the MCP server will start. It checks launcher availability, Codex config drift, live PromptLock session/transport env, and a real `initialize` / `tools/list` stdio handshake through `promptlock-mcp-launch`.

For wrapper-launched containers started via `promptlock auth docker-run`, the wrapper also mounts a live MCP env file at `/run/promptlock/promptlock-mcp.env`. `promptlock-mcp-launch` reads that file when clients like Codex strip most shell env before spawning MCP child processes.

The primary supported MCP UX is a client already running inside a long-lived container started by `promptlock auth docker-run`.
Direct host-run MCP clients are the secondary/manual path.

## Security model
- capability-first flow only (no plaintext secret fetch tool)
- uses broker lease + execute endpoints under the hood
- requires `PROMPTLOCK_SESSION_TOKEN` for agent auth
- in local hardened mode, defaults to the agent Unix socket and fails closed if no agent socket or explicit broker transport is configured
- keep the operator socket on the host side only; MCP clients should receive agent-side transport only

## `env_path`
- `env_path` is approval context only. The adapter forwards it on the lease request; execute-time secret lookup still happens on the broker after approval.
- `env_path` follows the same rules as `promptlock exec --env-path`.
- The broker canonicalizes `env_path` inside `PROMPTLOCK_ENV_PATH_ROOT`; traversal and symlink escapes are rejected there.
- In non-dev profiles, `.env`-backed MCP requests fail closed until the host broker is running with `PROMPTLOCK_ENV_PATH_ROOT` set.
- For the quickstart `run_tests` intent, the file content should use `github_token=...`, not `PROMPTLOCK_SECRET_GITHUB_TOKEN=...`.
- A repo-local `demo-envs/...` file is only a disposable demo convenience. In steady state, prefer a host-only `PROMPTLOCK_ENV_PATH_ROOT` outside the mounted workspace or launch the wrapper container with `--hide-path` so the mounted repo does not expose the same file directly.

Example tool arguments for a disposable demo token:

```json
{
  "intent": "run_tests",
  "command": ["make", "demo-print-github-token"],
  "ttl_minutes": 5,
  "env_path": "demo-envs/github.env"
}
```

Notes:
- If the command prints the token, use a disposable demo value only.
- Direct argv commands are the supported hardened shape; avoid `bash -lc` wrappers for broker-exec demos.
- `make demo-print-github-token` is a repo demo target that prints the leased `GITHUB_TOKEN` on the host broker. With the demo file shape from the README, the expected value is `FAKE_GITHUB_TOKEN`. For helper scripts, remember that broker-exec runs on the host, so the path must exist on the host repo root, not only in the container's `/workspace`.
- `make demo-run-env-showcase-tests` is a second repo demo target that runs `go test ./demo-envs/showcase` with extra demo metadata env values and verifies leased `GITHUB_TOKEN` in test output.
- For Codex-in-container flows, it is fine to ask Codex to inspect `/workspace/Makefile` first and then invoke the MCP tool with `["make","demo-print-github-token"]`; the repo README now includes that exact prompt shape.

## Client setup patterns

Start with the wrapper-launched container path from the README when the agent already lives inside `promptlock auth docker-run`.
Use the host-minted session-token setup only when the MCP client really runs on the host or in some other manually managed process.

Build or install `promptlock-mcp`, mint a fresh agent session token with `promptlock auth login --show-secrets`, then configure the MCP client to launch the stdio adapter with:

- `PROMPTLOCK_SESSION_TOKEN`
- `PROMPTLOCK_AGENT_ID`
- `PROMPTLOCK_TASK_ID`
- one agent-side transport variable:
  - `PROMPTLOCK_AGENT_UNIX_SOCKET` for host processes and Linux containers
  - `PROMPTLOCK_BROKER_URL` for desktop-Docker container access to the daemon-owned agent bridge on macOS

The README now includes copy-paste examples for:

- Codex CLI
- Claude Code
- Gemini CLI
- Cursor Agent (`.cursor/mcp.json`)

## macOS desktop Docker workaround

For manual containerized MCP clients on macOS desktop Docker runtimes, do not rely on a direct bind mount of the host PromptLock Unix socket into the container.
That container leg is not reliable there.

Use this safer pattern instead:

- keep the operator socket on the host only
- let `promptlockd` start its built-in agent-only loopback bridge
- source `instance.env` from `promptlock setup`
- after daemon startup, use `promptlock daemon status --json` to get the live `agent_bridge_container_url`
- point the containerized MCP adapter at that URL via `PROMPTLOCK_BROKER_URL`

Before launching the containerized MCP client, you can confirm the bridge with:

```bash
. '<instance-env-file>'
go run ./cmd/promptlock daemon status
```

That status output now reports:

- agent API reachability over the selected local transport
- host-loopback bridge reachability
- the container-facing URL derived from `host.docker.internal`

This is the same transport boundary the README demonstrates for manual MCP client setup.

## Notes
- This is an early adapter scaffold for interoperability testing.
- Keep hardened broker config enabled (`allow_plaintext_secret_return=false` and broker-exec path).
- Adapter now includes baseline input validation for intent, command, TTL, and `env_path` bounds.
- Non-scalar JSON-RPC request ids are rejected with `-32600` across initialize, tool-call, and cancellation handling.
- `initialize` responses advertise `protocolVersion` plus `capabilities.tools` (resources/prompts remain unadvertised until their namespaces are fully implemented).
- `tools/call` now returns JSON-RPC `-32602` when params are null/empty or `name` is missing, preventing ambiguous unknown-tool errors for malformed requests.
- `tools/list` now publishes a stricter `execute_with_intent` JSON Schema (`additionalProperties=false`, required `intent`/`command`, bounded string lengths, integer TTL range, optional `env_path`), and runtime validation rejects non-string command args, fractional TTL values, and invalid `env_path` values.
- `notifications/cancelled` now cancels matching in-flight `tools/call` requests by JSON-RPC request id (`requestId`/`id`), and best-effort propagates cleanup to broker `POST /v1/leases/cancel?request_id=...` so pending lease requests are not left waiting for operator action.
- If broker cleanup propagation fails during cancellation, adapter stderr includes a warning with the pending `request_id` so operators can manually deny stale requests.
- `tools/call` notifications without an `id` are ignored (no response, no broker side effects), preventing untracked execution attempts through notification frames.
- Broker-facing MCP HTTP/Unix-socket calls use a bounded `10s` client deadline, so stalled peers fail with `broker request timed out after 10s` instead of hanging indefinitely.
- Harness now covers positive and selected negative paths (deny/timeout/missing session token).
- Harness includes a single-session lifecycle sequence check (`initialize`, `notifications/initialized`, `tools/list`, `tools/call`) to guard notification-ordering regressions.
- Conformance coverage includes target-client profiles for string-ID and numeric-ID JSON-RPC flows, including strict error-envelope checks (`id: null` for parse/batch errors).
- `make mcp-conformance-report` writes `reports/mcp-conformance.json` from the current `cmd/promptlock-mcp` test suite.
