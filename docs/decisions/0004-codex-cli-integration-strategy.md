# 0004 - Codex CLI integration strategy

- Status: accepted
- Date: 2026-03-07

## Context
PromptLock should be testable with Codex workflows while minimizing maintenance burden and upstream drift.

## Decision
1. Prefer **wrapper/adapter integration** over direct long-lived fork of Codex CLI.
2. Integration should happen through controlled secret resolution hooks (env/bootstrap/auth path), not broad source rewrites.
3. Keep any Codex-specific patch surface minimal and isolated.
4. Document upstream sync strategy and compatibility matrix.

## Implementation approach (v1)
- Build PromptLock broker and CLI contract first.
- Add codex-docker integration layer to call PromptLock for lease requests/access.
- Validate end-to-end with real Codex runs (lease issue, access, expiry, renewal).

## Consequences
- Faster iteration with lower maintenance risk.
- If deep Codex patching is required later, it should be ADR’d separately with rollback and sync plan.

## Security implications
- Wrapper-based approach reduces accidental broad trust expansion.
- Lease renewal and access events remain visible in PromptLock audit logs.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
