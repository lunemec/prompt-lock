# Secure execution flow, threat model, and operator concerns

This document describes the recommended PromptLock runtime flow with explicit focus on security and developer/agent ergonomics.

## Design goals
- frictionless agent UX for real tasks
- strict control over secret exposure
- bounded lease semantics with clear auditability
- portability across multiple CLIs and agent runtimes

## Primary flow (recommended)

1. Agent/dev invokes:
   - `promptlock exec --intent run_tests --ttl 5 -- <command>`
2. PromptLock resolves intent -> allowed secret set via host policy.
3. PromptLock requests/renews approval if no valid active lease.
4. Human approves or denies.
5. On approval, PromptLock injects required env vars into the child process only.
6. Command runs.
7. PromptLock clears process-scoped env context and records completion/audit events.

## Why intent first
Agents request outcomes, not secret names. Intent-based requests reduce friction and prevent over-broad secret asks.

## Secondary flow (explicit secrets)
Use only when intent mapping is insufficient:
- `promptlock exec --secrets github_token,npm_token --ttl 5 -- <command>`

Still uses execute-time injection and non-disclosure defaults.

---

## Critical security concerns and mitigations

### Concern 1: TTL bypass by ENV capture
Example risk:
- `MY_TOKEN=$(promptlock read-secret github_token)`
- token persists past lease window.

Mitigation:
- no raw secret-return API by default.
- execute-with-secret model only.
- high-risk read/export modes require explicit break-glass policy.

### Concern 2: Prompt-injection exfiltration attempts
Risk:
- injected instructions attempt to print or exfiltrate secrets.

Mitigation:
- non-disclosure defaults
- strict auditing
- optional egress controls in secure runtime
- human approval for sensitive intents

### Concern 3: Reuse outside approved context
Risk:
- lease token replay in unrelated process/session.

Mitigation:
- bind lease to invocation/session where feasible
- deny out-of-context usage
- audit replay attempts

### Concern 4: Hidden policy drift
Risk:
- requirements change without traceability.

Mitigation:
- ADR requirement for significant decisions/requirement changes
- changelog governance + final validation gate

---

## Audit requirements in this flow
Must log at minimum:
- request_created
- request_approved / request_denied
- command_started
- secret_access (without secret value)
- lease_renewed
- lease_expired / lease_revoked
- command_finished (status)

Audit logs are host-side and outside agent-writable paths.

---

## Protocol strategy
- core contract: HTTP/UDS broker API
- first adapter: MCP
- later adapter: ACP if needed

Adapters must preserve non-disclosure defaults and full audit semantics.

### MCP-specific requirement
- MCP tools must expose **capability execution** (e.g., `run_with_intent`) rather than plaintext secret retrieval.
- Plaintext secret-return MCP methods are disallowed by default.

---

## Developer ergonomics
- default happy path: one command (`promptlock exec ...`)
- reusable lease until expiry to avoid repeated prompts
- host policy controls TTL ranges and intent->secret mappings

This balances practical autonomy with explicit, operator-controlled risk boundaries.
