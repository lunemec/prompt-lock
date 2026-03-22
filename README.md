# PromptLock

**PromptLock**: a human-approved secret access broker for coding agents.

Instead of mounting raw long-lived secrets into agent containers, agents request a **time-bound lease** for one or more named secrets (for example `github_token` or `npm_token`) for **N minutes**. A human approves or denies the request. If approved, the agent can fetch only those secrets for the lease duration.

This reduces prompt-injection blast radius while keeping autonomous workflows practical.

## Name

Project name: **PromptLock**

Tagline: **Human-approved secret access for autonomous agents.**

Status: **pre-1.0 OSS release candidate**. The supported OSS deployment target is local-only hardened operation with role-separated Unix sockets. PromptLock is intended for public OSS use, but it is not yet making a 1.0 stability commitment.

## Who this is for

- You run local or containerized coding agents and do not want to inject long-lived secrets into their runtime by default.
- You want a host-side approval step and an audit trail before secrets or privileged host actions are used.
- You are evaluating whether a hardened local-only secret broker fits your workflow.

## Who this is not for yet

- Teams looking for a polished one-command cloud service or a stable 1.0 API contract.
- Users who want a supported remote TCP/TLS deployment story. The supported OSS path is local-only Unix sockets.

## Core contract

- Agent requests: `secrets[] + ttl_minutes + reason + task_id + agent_id`
- Human approves or denies
- Broker issues short-lived lease token on approval
- Agent can fetch only approved secrets until expiry
- All requests and approvals are logged

See `docs/CONTRACT.md`.

## Install

Install the binaries with Go:

```bash
go install github.com/lunemec/promptlock/cmd/promptlock@latest
go install github.com/lunemec/promptlock/cmd/promptlockd@latest
go install github.com/lunemec/promptlock/cmd/promptlock-mcp@latest
go install github.com/lunemec/promptlock/cmd/promptlock-mcp-launch@latest
```

Notes:

- `promptlock` is the main CLI most users will run.
- `promptlockd` is the underlying broker runtime if you want to run the daemon separately.
- `promptlock-mcp` is the experimental stdio MCP adapter.
- `promptlock-mcp-launch` is the wrapper-aware launcher for MCP clients that should not persist session token or transport values.
- The quickstart below still expects a checkout of this repo because it runs `promptlock setup` from the workspace root and builds the local lab image from this repo's `agent-lab` Docker target.

## Minimal Quickstart

This is the shortest reproducible repo flow for the GitHub demo secret.

Prerequisites:

- PromptLock installed from the section above
- Docker
- `jq`

Clone this repo and run these commands from the repo root.
This demo flow is intentionally self-contained in this repository so anyone can test it as-is.
Use two host terminals.

1. Prepare the workspace, build the demo image, and create a disposable demo env file:

```bash
promptlock setup
docker build --target agent-lab -t promptlock-agent-lab .
mkdir -p demo-envs
cat > demo-envs/github.env <<'EOF'
github_token=FAKE_GITHUB_TOKEN
EOF
```

2. Terminal A (host approval flow):

```bash
PROMPTLOCK_ENV_PATH_ROOT="$PWD" promptlock watch
```

3. Terminal B (host wrapper launch with repo demo env hidden from the container):

```bash
promptlock auth docker-run \
  --agent codex-agent \
  --container codex-agent-1 \
  --image promptlock-agent-lab \
  --entrypoint /bin/bash \
  --workdir /workspace \
  --mount type=bind,src="$PWD",dst=/workspace \
  --hide-path demo-envs \
  --mount type=bind,src="$HOME/.codex",dst=/home/promptlock/.codex \
  --env TERM="${TERM:-xterm-256color}" \
  -- \
```

4. Inside the container shell:

```bash
promptlock mcp doctor
codex mcp add promptlock -- promptlock-mcp-launch
codex -C /workspace --no-alt-screen
```

5. In Codex chat, ask for the demo secret via PromptLock MCP:

```text
Read /workspace/Makefile and tell me what demo-print-github-token does. Then call execute_with_intent with intent run_tests, env_path "demo-envs/github.env", and command ["make","demo-print-github-token"]. Tell me the exact tool result.
```

Or run a small env-driven test showcase through the same intent/env_path:

```text
Use the promptlock MCP server and call execute_with_intent with intent run_tests, env_path "demo-envs/github.env", and command ["make","demo-run-env-showcase-tests"]. Tell me the exact tool result.
```

Approve the request in Terminal A. Expected result from the tool path:

```text
FAKE_GITHUB_TOKEN
```

Then verify the audit log:

```bash
. '<instance-env-file>'
promptlock audit-verify --file "$PROMPTLOCK_SETUP_INSTANCE_DIR/audit.jsonl"
```

Notes:

- `promptlockd` is the host broker
- `promptlock watch` is the host approval UI and auto-starts `promptlockd` when needed
- `promptlock auth docker-run` is a host command that launches the container safely
- `--hide-path demo-envs` masks the repo demo env folder inside the container while still allowing host-side broker env-path resolution
- `make demo-print-github-token` prints `GITHUB_TOKEN` on the host broker through approved `execute_with_intent`
- `make demo-run-env-showcase-tests` runs `go test ./demo-envs/showcase` and verifies leased `GITHUB_TOKEN` plus demo metadata env values set by the target

If you want the full walkthrough and troubleshooting guide, use `docs/operations/REAL-E2E-HOST-CONTAINER.md`.

## Secondary Quickstart: Local Dev Demo

This path is faster, but it is for local testing only because it bypasses the external approval flow and uses the default local TCP listener at `http://127.0.0.1:8765` instead of the hardened dual-socket transport.

```bash
PROMPTLOCK_ALLOW_DEV_PROFILE=1 promptlockd
```

In a second terminal:

```bash
PROMPTLOCK_DEV_MODE=1 \
  promptlock exec --intent run_tests --ttl 5 --auto-approve -- env
```

## CLI Unification Status

Decision `0030` is now implemented:

- `promptlock daemon <start|stop|status>` is the primary lifecycle surface,
- `promptlock watch` can auto-start a local daemon when no explicit broker transport is configured,
- `promptlockd` remains available as the underlying broker runtime and for compatibility/internal use.

The published container image now includes:

- `secretctl.sh` on `PATH`
- agent skill guidance at `/opt/promptlock/skills/secret-request/SKILL.md`

## Developer And Release Workflows

- Full validation gate: `make validate-final`
- Toolchain and Docker base-image drift guard: `make toolchain-guard`
- PR-grade validation gate: `make release-readiness-gate-core`
- Docker smoke test: `make e2e-compose`
- Release-quality validation: `make release-readiness-gate`
- Quick fuzzing pass: `make fuzz`
- Storage fsync release evidence and release packaging: `docs/operations/RELEASE.md`

Canonical Go and Docker base-image pins live in `.toolchain.env`; update that file first when bumping versions.

## If You're Contributing

- Human contributors: start with `CONTRIBUTING.md`.
- Agent contributors: start with `AGENTS.md`.
- For docs or CLI UX changes, validate both the text and the actual command behavior so README examples, `--help` output, and setup summaries do not drift apart.

## More Docs

- Evaluate PromptLock: `docs/operations/REAL-E2E-HOST-CONTAINER.md`
- MCP adapter and client setup: `docs/operations/MCP.md`
- CLI behavior and approval semantics: `docs/operations/WRAPPER-EXEC.md`
- Operate PromptLock: `docs/operations/CONFIG.md`, `docs/operations/DOCKER.md`, `SECURITY.md`
- Change PromptLock: `docs/README.md`, `docs/standards/ENGINEERING-STANDARDS.md`, `docs/plans/ACTIVE-PLAN.md`, `docs/plans/BACKLOG.md`

## MCP Setup (Reference)

If you only want the reproducible demo for this repo, use `Minimal Quickstart` above.
This section is the detailed MCP wiring reference.

PromptLock ships an experimental stdio MCP adapter in `cmd/promptlock-mcp` with one capability-first tool today:

- `execute_with_intent`

`execute_with_intent` accepts:

- `intent`
- `command`
- optional `ttl_minutes`
- optional `env_path` for approved `.env` / env-like file lookup

The MCP adapter needs:

- a current agent `session_token`
- agent-side PromptLock transport only
- a host-side operator approval flow (`promptlock watch`)

With `broker-exec`, the approved command still runs on the host broker, not inside the agent container.
Running the MCP client directly on the host is the manual setup path.

Before wiring a client, you can preflight the exact shell where the MCP server will start:

```bash
promptlock mcp doctor
```

Use `promptlock mcp doctor --json` for structured output.

### Manual host MCP path: client runs on the host

Use this when the MCP-capable client is not already inside a wrapper-launched container.

Run this on the host after `promptlock setup`:

```bash
. '<instance-env-file>'

auth_json="$(
  promptlock auth login \
    --operator-token "$PROMPTLOCK_OPERATOR_TOKEN" \
    --agent mcp-agent \
    --container mcp-client \
    --show-secrets
)"

export PROMPTLOCK_MCP_BIN=promptlock-mcp
export PROMPTLOCK_SESSION_TOKEN="$(printf '%s' "$auth_json" | jq -r '.session_token')"
export PROMPTLOCK_MCP_AGENT_ID=mcp-agent
export PROMPTLOCK_MCP_TASK_ID=mcp-task
```

Choose one transport block next. On non-Linux hardened quickstarts, `promptlockd` binds the agent bridge to a dynamic loopback port to avoid collisions across concurrent local workspaces.

<details>
<summary>Host process or Linux container: use the agent Unix socket directly</summary>

```bash
export PROMPTLOCK_MCP_TRANSPORT_KEY=PROMPTLOCK_AGENT_UNIX_SOCKET
export PROMPTLOCK_MCP_TRANSPORT_VALUE="$PROMPTLOCK_AGENT_UNIX_SOCKET"
```

</details>

<details>
<summary>macOS desktop Docker container: use the daemon-owned agent bridge</summary>

Direct host Unix-socket bind mounts are not reliable for manual containerized MCP clients on macOS desktop Docker runtimes.
In the supported local hardened flow, `promptlockd` now starts an agent-only loopback bridge automatically when dual sockets are enabled.

The safe shape is:

- keep the operator socket on the host only
- let the daemon expose only agent-side routes on `127.0.0.1`
- point the containerized MCP client at the live `agent_bridge_container_url` from `promptlock daemon status --json`

In the host shell, use:

```bash
export PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL="$(
  promptlock daemon status --json | jq -r '.agent_bridge_container_url'
)"

export PROMPTLOCK_MCP_TRANSPORT_KEY=PROMPTLOCK_BROKER_URL
export PROMPTLOCK_MCP_TRANSPORT_VALUE="$PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL"
```

If you are not using `promptlock setup`, configure `agent_bridge_address` in the daemon config to a loopback-only address such as `127.0.0.1:0`, then use `promptlock daemon status --json` after startup to get the actual container URL.

To verify that the daemon-owned bridge is actually up before starting the containerized MCP client, run:

```bash
. '<instance-env-file>'
promptlock daemon status
```

Expected on a healthy non-Linux hardened quickstart:

- `agent api: reachable ...`
- `agent bridge: reachable on host ...`
- `container bridge url: http://host.docker.internal:...`

</details>

### Add PromptLock MCP to major agent CLIs

After one of the setup paths above, the examples below register the PromptLock MCP server with an agent CLI. They are intentionally capability-first and wire only agent-side transport. Do not expose the operator socket to MCP clients.

<details>
<summary>Codex CLI</summary>

```bash
codex mcp add promptlock \
  --env "${PROMPTLOCK_MCP_TRANSPORT_KEY}=${PROMPTLOCK_MCP_TRANSPORT_VALUE}" \
  --env "PROMPTLOCK_SESSION_TOKEN=${PROMPTLOCK_SESSION_TOKEN}" \
  --env "PROMPTLOCK_AGENT_ID=${PROMPTLOCK_MCP_AGENT_ID}" \
  --env "PROMPTLOCK_TASK_ID=${PROMPTLOCK_MCP_TASK_ID}" \
  -- "${PROMPTLOCK_MCP_BIN}"

codex mcp list --json
```

Example prompt:

```text
Use the promptlock MCP server and call execute_with_intent with intent run_tests and command ["go","version"].
```

</details>

<details>
<summary>Claude Code</summary>

```bash
claude mcp add --scope project \
  -e "${PROMPTLOCK_MCP_TRANSPORT_KEY}=${PROMPTLOCK_MCP_TRANSPORT_VALUE}" \
  -e "PROMPTLOCK_SESSION_TOKEN=${PROMPTLOCK_SESSION_TOKEN}" \
  -e "PROMPTLOCK_AGENT_ID=${PROMPTLOCK_MCP_AGENT_ID}" \
  -e "PROMPTLOCK_TASK_ID=${PROMPTLOCK_MCP_TASK_ID}" \
  promptlock \
  -- "${PROMPTLOCK_MCP_BIN}"

claude mcp list
```

Example prompt:

```text
Use the promptlock MCP server and run execute_with_intent for ["go","version"] under intent run_tests.
```

</details>

<details>
<summary>Gemini CLI</summary>

```bash
gemini mcp add --scope project \
  -e "${PROMPTLOCK_MCP_TRANSPORT_KEY}=${PROMPTLOCK_MCP_TRANSPORT_VALUE}" \
  -e "PROMPTLOCK_SESSION_TOKEN=${PROMPTLOCK_SESSION_TOKEN}" \
  -e "PROMPTLOCK_AGENT_ID=${PROMPTLOCK_MCP_AGENT_ID}" \
  -e "PROMPTLOCK_TASK_ID=${PROMPTLOCK_MCP_TASK_ID}" \
  promptlock \
  "${PROMPTLOCK_MCP_BIN}"

gemini mcp list
```

Example prompt:

```text
Use the promptlock MCP server and call execute_with_intent with intent run_tests and command ["go","version"].
```

</details>

<details>
<summary>Cursor Agent</summary>

Cursor Agent currently reads MCP servers from `mcp.json` rather than a stable `cursor-agent mcp add` command.

```bash
mkdir -p .cursor

cat > .cursor/mcp.json <<JSON
{
  "mcpServers": {
    "promptlock": {
      "command": "${PROMPTLOCK_MCP_BIN}",
      "args": [],
      "env": {
        "${PROMPTLOCK_MCP_TRANSPORT_KEY}": "${PROMPTLOCK_MCP_TRANSPORT_VALUE}",
        "PROMPTLOCK_SESSION_TOKEN": "${PROMPTLOCK_SESSION_TOKEN}",
        "PROMPTLOCK_AGENT_ID": "${PROMPTLOCK_MCP_AGENT_ID}",
        "PROMPTLOCK_TASK_ID": "${PROMPTLOCK_MCP_TASK_ID}"
      }
    }
  }
}
JSON

cursor-agent mcp list
cursor-agent mcp list-tools promptlock
```

Example prompt:

```text
Use the promptlock MCP server to run execute_with_intent for ["go","version"] with intent run_tests.
```

</details>

### Wrapper auto-injected env (reference)

When a client runs inside a container launched by `promptlock auth docker-run`, the wrapper auto-injects and mounts:

- `PROMPTLOCK_SESSION_TOKEN`
- agent-side transport (`PROMPTLOCK_AGENT_UNIX_SOCKET` on Linux, `PROMPTLOCK_BROKER_URL` on non-Linux desktop Docker)
- wrapper identity env (`PROMPTLOCK_WRAPPER_AGENT_ID`, `PROMPTLOCK_WRAPPER_TASK_ID`)
- wrapper-scoped current session/transport env
- live MCP env file at `/run/promptlock/promptlock-mcp.env`

For wrapper-launched containers, prefer `promptlock-mcp-launch` over persisting raw transport/session env values in MCP client config.
No extra `promptlock auth login` step is needed inside that container.

For more adapter detail, protocol notes, and constraints, see `docs/operations/MCP.md`.

## Repository contents

- `AGENTS.md` — project map and non-negotiable engineering/security rules
- `CHANGELOG.md` — Keep-a-Changelog history (`[Unreleased]` required)
- `Makefile` — exposed commands for developers/users
- `docs/README.md` — documentation map and maintenance rules
- `docs/CONTRACT.md` — API and security contract
- `docs/NOTE-project-style-adoption.md` — reusable agent/docs style for other projects
- `docs/architecture/` — architecture source of truth (hexagonal required)
  - includes secure execution flow and threat-model notes (`SECURE-EXEC-FLOW.md`)
- `docs/decisions/` — ADRs for architecture and requirement changes
  - `docs/decisions/INDEX.md` is the ADR entrypoint
- `docs/standards/` — engineering standards (Red-Green-Blue TDD, security reporting)
- `docs/plans/` — `ACTIVE-PLAN.md` handoff, `BACKLOG.md` open work, plus typed subdirectories for initiatives, checklists, notes, status files, and archives
- `docs/operations/` — runbooks, Dockerization, config, wrapper execution notes, MCP adapter notes, key rotation/revocation, and release guide
- `docs/context/` — product context and trust boundaries
- `cmd/promptlock-mock-broker` — minimal local broker (demo)
- `scripts/secretctl.sh` — agent-facing CLI wrapper
- `scripts/human-approve.sh` — human approval helper
- `skills/secret-request/SKILL.md` — skill instructions for agents
- `examples/` — sample workflow commands

## Agent-generated code note

This repository is primarily **agent-generated code and documentation**, following the same agent-first workflow style as the `codex-docker` project.

## Important

This repository targets a **public pre-1.0 OSS release**. Hardened deployment is the supported path for real-world use; dev-profile defaults and demo helpers remain for local testing and migration.

Current implementation uses in-memory request/lease/auth/session stores by default unless configured with durable host-backed state files. For OSS-targeted use, configure the hardened local controls, encrypted auth persistence (`auth.store_encryption_key_env`), durable request/lease state via either local file persistence (`state_store_file`, `state_store.type=file`) or an external HTTP state adapter (`state_store.type=external`), and external secret backend adapters.

Startup guardrails now enforce fail-closed production posture: non-dev profiles require durable state files and non-`in_memory` secret source; dev profile startup requires explicit opt-in (`PROMPTLOCK_ALLOW_DEV_PROFILE=1`).

Supported OSS hardening centers on local dual unix sockets, policy enforcement, encrypted at-rest auth persistence, local audit hash-chain integrity verification, and external secret backend integration (Vault/1Password/etc.). Non-local TCP TLS/mTLS transport support has been removed from the supported code path; PromptLock is a local-only Unix-socket deployment.

**Critical:** audit trail must be persisted on the host (outside agent-controlled workspace/container paths) so request/approval/access history cannot be silently altered by agent workloads.

Docker deployment guidance: `docs/operations/DOCKER.md`.
Security policy: `SECURITY.md`.

## License

MIT — see `LICENSE`.
