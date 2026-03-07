# Secret Lease Broker (Draft)

## Goal
Build a human-approved secret access broker for coding agents.

Instead of mounting raw long-lived secrets into agent containers, agents request a **time-bound lease** for one or more named secrets (e.g., `github_token`, `npm_token`) for **N minutes**. A human approves/denies the request. If approved, the agent can fetch only those secrets for the lease duration.

This reduces prompt-injection blast radius while keeping autonomous workflows practical.

## Core contract
- Agent requests: `secrets[] + ttl_minutes + reason + task_id + agent_id`
- Human approves or denies
- Broker issues short-lived lease token on approval
- Agent can fetch only approved secrets until expiry
- All requests and approvals are logged

See `docs/CONTRACT.md`.

## Repository contents
- `docs/CONTRACT.md` — API and security contract
- `scripts/mock-broker.py` — minimal local broker (demo)
- `scripts/secretctl.sh` — agent-facing CLI wrapper
- `scripts/human-approve.sh` — human approval helper
- `skills/secret-request/SKILL.md` — skill instructions for agents
- `examples/` — sample workflow commands

## Quick demo

Start broker:

```bash
python3 scripts/mock-broker.py
```

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

Human approves:

```bash
scripts/human-approve.sh <request_id> 20
```

Agent fetches secret by lease token:

```bash
scripts/secretctl.sh access --lease <lease_token> --secret github_token
```

## Important
This is a draft prototype for flow design and integration testing, not a production-grade secret manager.

Production hardening should include mTLS, unix sockets, policy engine, encrypted at-rest storage, tamper-evident audit logs, and external secret backend integration (Vault/1Password/etc.).
