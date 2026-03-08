# Architecture Review Tasks (2026-03-08)

Source: strict architecture review requested by Lukas.
Status: Open.

## A-001 — Move policy enforcement out of HTTP handlers
- **Priority:** P1
- **Task:** Refactor auth/policy/execute decision logic currently in `cmd/promptlockd/*` into app-layer services/use-cases.
- **Why:** Preserve hexagonal boundaries and increase testability.
- **Done when:**
  - [ ] Handler files are transport-only (decode/encode/error mapping).
  - [ ] Business/security decisions are in `internal/app` and/or core services.
  - [ ] Existing behavior parity verified with tests.

## A-002 — Split promptlockd inbound contexts
- **Priority:** P1
- **Task:** Break daemon handler package into bounded contexts (auth, lease, execute, host-ops, meta).
- **Why:** Reduce growth-driven coupling and improve maintainability.
- **Done when:**
  - [ ] New package/module layout documented.
  - [ ] Route registration remains explicit and test-covered.
  - [ ] No circular dependencies introduced.

## A-003 — Define explicit service interfaces for control-plane capabilities
- **Priority:** P1
- **Task:** Introduce app interfaces for execution policy evaluation, egress checks, and host-docker mediation.
- **Why:** Enable adapter substitution and stronger contract tests.
- **Done when:**
  - [ ] Interfaces live under ports/app boundary.
  - [ ] Default implementations injected in main wiring.
  - [ ] Unit tests run with mocked implementations.

## A-004 — Add architecture conformance tests
- **Priority:** P1
- **Task:** Add structural tests/linters ensuring inward dependencies only and no forbidden imports across layers.
- **Why:** Prevent gradual erosion of hexagonal architecture.
- **Done when:**
  - [ ] Conformance checks run in CI.
  - [ ] Failing example added to prove guardrail effectiveness.
  - [ ] Docs explain how to fix violations.

## A-005 — Formalize package boundaries in architecture docs
- **Priority:** P2
- **Task:** Update `docs/architecture/ARCHITECTURE.md` with current package map and ownership of each concern.
- **Why:** Keep implementation and architecture source-of-truth aligned.
- **Done when:**
  - [ ] Diagram or table maps handlers/app/core/adapters.
  - [ ] Each public endpoint mapped to owning use-case.
  - [ ] Review checklist references this mapping.

## A-006 — Add ADR for execution-surface boundaries
- **Priority:** P2
- **Task:** Write/update ADR describing where execute/host-ops policy belongs (transport vs app vs domain).
- **Why:** Avoid repeated architectural drift debates.
- **Done when:**
  - [ ] ADR accepted in `docs/decisions/`.
  - [ ] Code references align with ADR.
  - [ ] Follow-up tasks captured for any deferred items.

## A-007 — Standardize error taxonomy across handlers
- **Priority:** P2
- **Task:** Introduce consistent internal error types and HTTP mapping rules.
- **Why:** Improves observability, testing clarity, and client behavior.
- **Done when:**
  - [ ] Shared error mapping utility in inbound adapter.
  - [ ] Tests assert stable status-code mapping.
  - [ ] Docs include error contract summary.
