# PromptLock (Draft)

## Goal
Build **PromptLock**: a human-approved secret access broker for coding agents.

Instead of mounting raw long-lived secrets into agent containers, agents request a **time-bound lease** for one or more named secrets (e.g., `github_token`, `npm_token`) for **N minutes**. A human approves/denies the request. If approved, the agent can fetch only those secrets for the lease duration.

This reduces prompt-injection blast radius while keeping autonomous workflows practical.

## Name
Project name: **PromptLock**

Tagline: **Human-approved secret access for autonomous agents.**

## Core contract
- Agent requests: `secrets[] + ttl_minutes + reason + task_id + agent_id`
- Human approves or denies
- Broker issues short-lived lease token on approval
- Agent can fetch only approved secrets until expiry
- All requests and approvals are logged

See `docs/CONTRACT.md`.

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

## Quick demo

Install local git hook (recommended):

```bash
git config core.hooksPath .githooks
chmod +x .githooks/pre-commit
```

Run final validation gate manually:

```bash
make validate-final
```

Run quick fuzzing pass:

```bash
make fuzz
```

Run docker-compose real-path smoke test:

```bash
make e2e-compose
```

Start broker (prototype Go mock):

```bash
go run ./cmd/promptlock-mock-broker
```

Start broker (Go v1 skeleton):

```bash
PROMPTLOCK_ALLOW_DEV_PROFILE=1 go run ./cmd/promptlockd
```

Start broker with host config:

```bash
PROMPTLOCK_CONFIG=./examples/config.example.json go run ./cmd/promptlockd
```

Run a command via PromptLock wrapper (prototype):

```bash
PROMPTLOCK_BROKER_URL=http://127.0.0.1:8765 \
  go run ./cmd/promptlock exec --intent run_tests --ttl 5 --auto-approve -- env
```

(For local demo only: `--auto-approve` bypasses external human approval flow.)

Agent requests secrets:

```bash
scripts/secretctl.sh request \
  --agent ralph-r1 \
  --task TASK-1001 \
  --ttl 20 \
  --reason "Run integration tests against GitHub + npm" \
  --secret github_token \
  --secret npm_token
```

Human approves (interactive queue):

```bash
go run ./cmd/promptlock approve-queue
```

Auth lifecycle helpers (CLI):

```bash
go run ./cmd/promptlock auth bootstrap --agent <agent_id> --container <container_id> --operator-token <token>
go run ./cmd/promptlock auth pair --token <bootstrap_token> --container <container_id>
go run ./cmd/promptlock auth mint --grant <grant_id>
```

CLI-first host+container walkthrough:

```bash
cat docs/operations/REAL-E2E-HOST-CONTAINER.md
```

## Agent-generated code note
This repository is primarily **agent-generated code and documentation**, following the same agent-first workflow style as the `codex-docker` project.

## Important
This is a draft prototype for flow design and integration testing, not a production-grade secret manager.

Current implementation uses in-memory request/lease/auth/session stores by default unless configured with durable host-backed state files. For production, use hardened deployment controls, encrypted auth persistence (`auth.store_encryption_key_env`), durable request/lease state persistence (`state_store_file`), and external secret backend adapters.

Startup guardrails now enforce fail-closed production posture: non-dev profiles require durable state files and non-`in_memory` secret source; dev profile startup requires explicit opt-in (`PROMPTLOCK_ALLOW_DEV_PROFILE=1`).

Production hardening should include mTLS, unix sockets, policy engine, encrypted at-rest storage, tamper-evident audit logs, and external secret backend integration (Vault/1Password/etc.).

**Critical:** audit trail must be persisted on the host (outside agent-controlled workspace/container paths) so request/approval/access history cannot be silently altered by agent workloads.

Docker deployment guidance: `docs/operations/DOCKER.md`.
Security policy: `SECURITY.md`.

## License
MIT — see `LICENSE`.
