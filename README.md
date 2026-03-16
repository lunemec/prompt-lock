# PromptLock

## Goal
Build **PromptLock**: a human-approved secret access broker for coding agents.

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

## Start Here

If you want the fastest proof that PromptLock works as an evaluator, start with:

```bash
go run ./cmd/promptlock setup
```

That generates a hardened local quickstart instance for this repo and prints the exact next commands for three terminals.
If you are evaluating from a repo checkout and prefer Make targets, `make setup-local-docker` is the equivalent convenience alias.

## Prerequisites
- Host has Go, Docker, and `jq`.
- The recommended quickstart below is the hardened local flow.
  It uses role-separated Unix sockets by default and does not expose the operator API to the container.

## Quick Mental Model
- `promptlockd`: the broker running on the host.
- Operator socket: host-only approval API used by `promptlock watch`.
- Agent socket: narrower API mounted into the container for agent requests.
- `intent`: named policy scope that maps approved secrets and egress rules to a task like `run_tests`.
- `--broker-exec`: the approved command runs on the broker host, not inside the container.

If you only remember one thing: the recommended flow is "host broker + host watch UI + containerized agent CLI waiting for approval".

## Recommended Quickstart: Hardened Local Docker Flow

This is the simplest copy-paste path for a first-time user/operator.
It runs the broker on the host, keeps operator access on a host-only socket, mounts only the agent socket into the container, waits for human approval, and then executes the approved command on the broker host via `--broker-exec`.

What this proves in one run:
- the container can request access without seeing the operator socket,
- the operator can approve from the host,
- the approved command executes through the broker-host policy boundary,
- the audit log records the decision trail.

### 1. Generate a host-side quickstart instance for this repo

```bash
go run ./cmd/promptlock setup
```

That runs `go run ./cmd/promptlock setup` and creates a per-workspace quickstart instance under your host state directory, outside the repo tree.
If you prefer Make targets from a repo checkout, `make setup-local-docker` runs the same command.
By default it writes:
- `config.json`
- `instance.env`
- audit/state/auth files
- per-workspace agent/operator Unix sockets

The command prints the exact next commands for your current repo.
The generated `instance.env` includes:
- `PROMPTLOCK_CONFIG`
- `PROMPTLOCK_OPERATOR_TOKEN`
- `PROMPTLOCK_AUTH_STORE_KEY`
- role-specific Unix-socket env vars
- a local demo `PROMPTLOCK_SECRET_GITHUB_TOKEN` value so the first container-originated approval flow works immediately

This quickstart intentionally sets `execution_policy.output_security_mode=raw` so the first broker-exec demo can print `go version`.
After you verify the flow, switch back to the hardened default `none` for stronger containment.

### 2. Terminal A: source the generated env file and start the broker

Runs on the host. This starts the broker daemon that owns approval, audit, and broker-exec policy.

```bash
. '<instance-env-file>'
go run ./cmd/promptlockd
```

`promptlock setup` prints the real `instance.env` path for your workspace.
The supported quickstart keeps config, audit, and durable state outside the repo so those files do not live in the agent-controlled workspace.

### 3. Terminal B: source the same env file and start the human watch UI

Runs on the host. This is the operator-side approval UI and talks to the host-only operator socket.

```bash
. '<instance-env-file>'
go run ./cmd/promptlock watch
```

### 4. Terminal C: build the agent image

Runs on the host once. The resulting container only gets the agent socket, not the operator socket.

```bash
docker build -t promptlock-agent-lab .
```

### 5. Terminal C: source the same env file and launch the agent container with a minted session

Runs on the host. This command bootstraps auth across both host sockets, launches `docker run`, and then the containerized CLI waits for approval.

```bash
. '<instance-env-file>'
go run ./cmd/promptlock auth docker-run \
  --agent toolbelt-agent \
  --container toolbelt-container-1 \
  --image promptlock-agent-lab \
  --entrypoint /usr/local/bin/promptlock \
  -- \
  exec \
  --agent toolbelt-agent \
  --task readme-lab \
  --intent run_tests \
  --reason "README host approval test" \
  --ttl 20 \
  --wait-approve 5m \
  --poll-interval 2s \
  --broker-exec \
  -- go version
```

Approve the request in Terminal B when prompted.
Expected output in Terminal C after approval:

```text
go version ...
```

What success looks like:
- Terminal B shows a pending request with the approved `run_tests` intent.
- After approval, Terminal C prints `go version ...`.
- The container never needs the operator socket.

Common first confusion:
- `promptlockd` runs on the host; `promptlock` is the client CLI.
- `promptlock auth docker-run` is also a host-side command even though it launches the container.
- `--broker-exec` runs the approved command on the broker host, not inside the container.

If your base image already includes `promptlock` (for example a derived toolbelt/Codex image), replace `--image promptlock-agent-lab` with that image and keep the same wrapper command.

### 6. Verify the audit log

```bash
. '<instance-env-file>'
go run ./cmd/promptlock audit-verify --file "$PROMPTLOCK_SETUP_INSTANCE_DIR/audit.jsonl"
```

Expected: `audit verify ok: ...`

If you want the full host+container lab walkthrough and troubleshooting map, use `docs/operations/REAL-E2E-HOST-CONTAINER.md`.

## Secondary Quickstart: Local Dev Demo

This path is faster, but it is for local testing only because it bypasses the external approval flow and uses the default local TCP listener at `http://127.0.0.1:8765` instead of the hardened dual-socket transport.

```bash
PROMPTLOCK_ALLOW_DEV_PROFILE=1 go run ./cmd/promptlockd
```

In a second terminal:

```bash
PROMPTLOCK_DEV_MODE=1 \
  go run ./cmd/promptlock exec --intent run_tests --ttl 5 --auto-approve -- env
```

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
- CLI behavior and approval semantics: `docs/operations/WRAPPER-EXEC.md`
- Operate PromptLock: `docs/operations/CONFIG.md`, `docs/operations/DOCKER.md`, `SECURITY.md`
- Change PromptLock: `docs/README.md`, `docs/standards/ENGINEERING-STANDARDS.md`, `docs/plans/ACTIVE-PLAN.md`, `docs/plans/BACKLOG.md`

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
