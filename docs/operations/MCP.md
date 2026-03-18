# MCP adapter (experimental)

PromptLock now includes an experimental MCP stdio adapter:

- binary path: `cmd/promptlock-mcp`
- baseline protocol methods: `initialize`, `initialized`, `notifications/initialized`, `notifications/cancelled`, `ping`, `shutdown`, `exit`, `tools/list`, `resources/list`, `prompts/list`, `tools/call`
- tool exposed: `execute_with_intent`

## Security model
- capability-first flow only (no plaintext secret fetch tool)
- uses broker lease + execute endpoints under the hood
- requires `PROMPTLOCK_SESSION_TOKEN` for agent auth
- in local hardened mode, defaults to the agent Unix socket and fails closed if no agent socket or explicit broker transport is configured
- keep the operator socket on the host side only; MCP clients should receive agent-side transport only

## Client setup patterns

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
- Adapter now includes baseline input validation for intent, command, and TTL bounds.
- Non-scalar JSON-RPC request ids are rejected with `-32600` across initialize, tool-call, and cancellation handling.
- `initialize` responses advertise `protocolVersion` plus `capabilities.tools` (resources/prompts remain unadvertised until their namespaces are fully implemented).
- `tools/call` now returns JSON-RPC `-32602` when params are null/empty or `name` is missing, preventing ambiguous unknown-tool errors for malformed requests.
- `tools/list` now publishes a stricter `execute_with_intent` JSON Schema (`additionalProperties=false`, required `intent`/`command`, bounded string lengths, integer TTL range), and runtime validation rejects non-string command args and fractional TTL values.
- `notifications/cancelled` now cancels matching in-flight `tools/call` requests by JSON-RPC request id (`requestId`/`id`), and best-effort propagates cleanup to broker `POST /v1/leases/cancel?request_id=...` so pending lease requests are not left waiting for operator action.
- If broker cleanup propagation fails during cancellation, adapter stderr includes a warning with the pending `request_id` so operators can manually deny stale requests.
- `tools/call` notifications without an `id` are ignored (no response, no broker side effects), preventing untracked execution attempts through notification frames.
- Broker-facing MCP HTTP/Unix-socket calls use a bounded `10s` client deadline, so stalled peers fail with `broker request timed out after 10s` instead of hanging indefinitely.
- Harness now covers positive and selected negative paths (deny/timeout/missing session token).
- Harness includes a single-session lifecycle sequence check (`initialize`, `notifications/initialized`, `tools/list`, `tools/call`) to guard notification-ordering regressions.
- Conformance coverage includes target-client profiles for string-ID and numeric-ID JSON-RPC flows, including strict error-envelope checks (`id: null` for parse/batch errors).
- `make mcp-conformance-report` writes `reports/mcp-conformance.json` from the current `cmd/promptlock-mcp` test suite.
