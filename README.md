# PromptLock

## Goal
Build **PromptLock**: a human-approved secret access broker for coding agents.

Instead of mounting raw long-lived secrets into agent containers, agents request a **time-bound lease** for one or more named secrets (e.g., `github_token`, `npm_token`) for **N minutes**. A human approves/denies the request. If approved, the agent can fetch only those secrets for the lease duration.

This reduces prompt-injection blast radius while keeping autonomous workflows practical.

## Name
Project name: **PromptLock**

Tagline: **Human-approved secret access for autonomous agents.**

Status: **pre-1.0 OSS release candidate**. The supported OSS deployment target is local-only hardened operation with role-separated Unix sockets. PromptLock is intended for public OSS use, but it is not yet making a 1.0 stability commitment.

## Core contract
- Agent requests: `secrets[] + ttl_minutes + reason + task_id + agent_id`
- Human approves or denies
- Broker issues short-lived lease token on approval
- Agent can fetch only approved secrets until expiry
- All requests and approvals are logged

See `docs/CONTRACT.md`.

## Start Here

If you want the fastest proof that PromptLock works, start with:

```bash
make e2e-compose
```

That builds the broker and runner images and exercises a real `promptlock exec --broker-exec` path in Docker.

## Prerequisites
- Host has Go, Docker, and `jq`.
- The recommended quickstart below is the hardened local flow.
  It uses role-separated Unix sockets by default and does not expose the operator API to the container.

## Recommended Quickstart: Hardened Local Docker Flow

This is the simplest copy-paste path for a first-time user/operator.
It runs the broker on the host, keeps operator access on a host-only socket, mounts only the agent socket into the container, waits for human approval, and then executes the approved command on the broker host via `--broker-exec`.

### 1. Create a minimal lab config

```bash
cat >/tmp/promptlock-readme.json <<'JSON'
{
  "security_profile": "hardened",
  "audit_path": "/tmp/promptlock-audit.jsonl",
  "state_store_file": "/tmp/promptlock-state-store.json",
  "auth": {
    "enable_auth": true,
    "operator_token": "op_real_test_token",
    "allow_plaintext_secret_return": false,
    "store_file": "/tmp/promptlock-auth-store.json"
  },
  "secret_source": {
    "type": "env",
    "env_prefix": "PROMPTLOCK_SECRET_",
    "in_memory_hardened": "fail"
  },
  "execution_policy": {
    "output_security_mode": "raw"
  },
  "intents": {
    "run_tests": ["github_token"]
  }
}
JSON
```

This demo overrides the hardened default `output_security_mode` to `raw` only so the example can print `go version`.
Use the hardened default `none` when you do not need command output.
`redacted` mode is only best-effort log scrubbing; it is not a strong secret-exfiltration control.

### 2. Terminal A: start the broker

```bash
export PROMPTLOCK_SECRET_GITHUB_TOKEN='demo_github_token_value'
export PROMPTLOCK_AUTH_STORE_KEY='replace_with_long_random_value'
PROMPTLOCK_CONFIG=/tmp/promptlock-readme.json go run ./cmd/promptlockd
```

In hardened local mode, PromptLock defaults to:
- agent socket: `/tmp/promptlock-agent.sock`
- operator socket: `/tmp/promptlock-operator.sock`

For the supported hardened flow, `promptlock exec`, `promptlock watch`, and `promptlock auth docker-run` auto-select these Unix sockets.
Use `--broker` or `PROMPTLOCK_BROKER_URL` only when you intentionally want the dev/demo local TCP listener instead of the hardened socket path.

### 3. Terminal B: start the human watch UI

```bash
PROMPTLOCK_OPERATOR_TOKEN=op_real_test_token go run ./cmd/promptlock watch
```

### 4. Terminal C: build the agent image

```bash
docker build -t promptlock-agent-lab .
```

### 5. Terminal C: launch the container with a minted session

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

If your base image already includes `promptlock` (for example a derived toolbelt/Codex image), replace `--image promptlock-agent-lab` with that image and keep the same wrapper command.

### 7. Verify the audit log

```bash
go run ./cmd/promptlock audit-verify --file /tmp/promptlock-audit.jsonl
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

## More Docs
- Full Docker/operator lab flow: `docs/operations/REAL-E2E-HOST-CONTAINER.md`
- Config reference: `docs/operations/CONFIG.md`
- Wrapper and approval CLI behavior: `docs/operations/WRAPPER-EXEC.md`
- Docker deployment guidance: `docs/operations/DOCKER.md`
- Security policy: `SECURITY.md`

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
