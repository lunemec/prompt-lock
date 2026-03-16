# Strict Review Remediation

Status: Archived on 2026-03-16.

This initiative tracked the 2026-03-15 strict review remediation pass.
All items in the original document were closed before archival.

Historical implementation detail is retained here for auditability.
The active planning surface now lives in:
- `docs/plans/ACTIVE-PLAN.md`
- `docs/plans/BACKLOG.md`

## Archived source

The original initiative body is preserved below without status changes.

# Strict Review Remediation

Status: Active (2026-03-15; all known 2026-03-15 strict re-review items closed, pending archive/update sweep).

Purpose: track the open implementation work created by the 2026-03-15 strict re-review so remediation stays backlog-driven, acceptance-based, and aligned with the repository security story.

## Priority model
- **P0** = security/correctness gaps that materially overstate or weaken enforced controls
- **P1** = architecture and operator-facing reliability gaps
- **P2** = verification and tooling follow-up

---

## P0-05 — SEC-035 Resolve broker-exec commands before secret reads
- **Area:** Security / broker execute ordering
- **Status:** Closed (2026-03-15)
- **Problem:** broker-exec still resolved secret material before managed command resolution completed, so a command that never became runnable could still trigger secret backend or approved env-path reads and `secret_access*` audit events.
- **Delivered:** app-layer broker-exec sequencing now resolves the approved command before any secret backend/env-path read, and regression coverage proves failed command resolution causes no secret backend read, no env-path read, no `secret_access_started`, and no `secret_access`.

## P1-03 — UX-008 Ignore ambient broker URL for Unix-socket CLI flows
- **Area:** UX / transport reliability
- **Status:** Closed (2026-03-15)
- **Problem:** explicit or role-selected Unix-socket CLI flows still depended on ambient `PROMPTLOCK_BROKER_URL`, so malformed or stale broker URLs could break auth/watch/exec flows or silently override the intended dual-socket path.
- **Delivered:** CLI transport selection now prefers the selected Unix-socket path before ambient broker URL fallback, and Unix-socket request construction now uses a fixed local request base so malformed ambient URLs cannot break `--broker-unix-socket`, role-socket auth, or watch/exec requests.

## P2-03 — QA-008 Prove the real execute-time egress deny path in the live harness
- **Area:** Verification / red-team harness
- **Status:** Closed (2026-03-15)
- **Problem:** `cmd/promptlock-redteam-live` used a fake lease token for the SEC-031 deny check, so the harness could stay green on a generic invalid-lease `403` instead of the real execute-time egress block.
- **Delivered:** the live harness now creates and approves a real request/lease before `/v1/leases/execute`, and direct tests plus an approved-lease execute deny regression prove the check exercises the real execute-time deny path.

## P2-04 — DOC-006 Align README and changelog transport claims with the active product surface
- **Area:** Documentation / trustworthiness
- **Status:** Closed (2026-03-15)
- **Problem:** README demo guidance and `[Unreleased]` changelog text still contained transport wording that implied removed TLS/mTLS surface or otherwise drifted from the local-only hardened dual-socket story.
- **Delivered:** README dev/demo guidance now explicitly distinguishes the hardened dual-socket path from the local TCP dev demo, and `[Unreleased]` changelog transport notes no longer imply retained TLS/mTLS support in the active product surface.

---

## P0-01 — SEC-031 Harden `network_egress_policy` so approval scope matches execution scope
- **Area:** Security / execution policy
- **Status:** Closed (2026-03-15)
- **Problem:** the approved request/lease does not carry intent, but `/v1/leases/execute` uses a caller-supplied intent string to select the egress allowlist. The current argv parser also returns success when no inspectable destination is present, which weakens the intended guarantee.
- **Delivered:** approved intent now persists on request/lease state, execute-time intent widening is rejected, direct network clients without inspectable argv destinations are denied, and contract/ops docs were aligned with the exact defense-in-depth guarantee.
- **Scope:**
  - persist the approved intent on the request/lease path used by broker-exec
  - reject execute-time intent mismatch or widening
  - close the current fail-open path for commands that rely on broker egress validation but expose no inspectable destination in argv, or otherwise narrow the supported execution surface so the guarantee is honest
  - update contract/ops docs to match the enforced behavior exactly
- **Strict gates:**
  - execute-time egress policy cannot be widened by supplying a different intent than the one approved
  - hardened/profiled behavior is deterministic when no destination can be inspected
  - docs describe the exact guarantee rather than implying runtime traffic mediation
- **Test scenarios:**
  1. approved request for intent `run_tests`, execute with intent `deploy` => forbidden
  2. command with explicit URL to allowed host => allowed
  3. command that would rely on egress policy but exposes no inspectable destination => deterministic deny or explicitly unsupported path

## P0-02 — SEC-032 Sanitize durability-gate audit reasons
- **Area:** Security / audit integrity
- **Status:** Closed (2026-03-15)
- **Problem:** durability-gate audit events currently write upstream error text verbatim, so hostile external state/backend responses can inject arbitrary text into host audit metadata.
- **Delivered:** durability close paths now map upstream failures to coarse stable reason codes before audit dispatch, while actionable diagnostics remain in returned errors/logs.
- **Scope:**
  - classify durability/audit reason codes into safe coarse categories before audit sink dispatch
  - keep actionable diagnostics in returned errors/logs only where appropriate, without copying raw backend bodies into audit metadata
  - ensure the same rule applies across durability close paths, not only one adapter
- **Strict gates:**
  - audit metadata never includes raw external response bodies or bearer-style values from backend errors
  - operators still get actionable failure classes for troubleshooting
- **Test scenarios:**
  1. external state backend returns `500` with echoed token/body => audit reason is coarse and sanitized
  2. repeated durability close on same failure class => same stable audit reason code

## P0-03 — SEC-033 Make the security baseline scanner fail closed
- **Area:** Security / release gates
- **Status:** Closed (2026-03-15)
- **Problem:** `cmd/promptlock-validate-security` silently skips unreadable files, which weakens `make security` and conflicts with the repository’s fail-closed posture.
- **Delivered:** scan read failures now fail the gate unless the path falls into an explicit documented skip class, and regression coverage now exercises unreadable file and unreadable target paths.
- **Scope:**
  - make scan read failures surface as gate failures unless the path is explicitly and intentionally skipped
  - document any intentional skip classes
  - add regression coverage for unreadable/unscannable paths
- **Strict gates:**
  - unreadable files fail `make security`
  - skip behavior is explicit, documented, and test-covered
- **Test scenarios:**
  1. permission-denied file => scan fails
  2. intentionally skipped artifact/cache path => scan still skips

## P0-04 — SEC-034 Make output redaction claims honest and test-backed
- **Area:** Security / output handling
- **Status:** Closed (2026-03-15)
- **Problem:** `output_security_mode=redacted` currently performs only narrow literal substitutions and does not justify stronger security expectations.
- **Delivered:** `redacted` mode now applies token-aware masking for common bearer/env-style secret shapes, regression fixtures cover representative leak forms, and docs keep the mode positioned as best-effort hygiene rather than strong containment.
- **Scope:**
  - either replace the current redactor with token-aware scrubbing that covers common secret formats, or narrow the docs/default expectations so `redacted` is clearly best-effort hygiene only
  - expand regression fixtures beyond `secret=abc`
  - keep hardened guidance centered on `none` when strong containment is required
- **Strict gates:**
  - docs and config guidance match real behavior
  - regression suite covers common secret/token leak shapes
- **Test scenarios:**
  1. `Authorization: Bearer ...` output
  2. `GITHUB_TOKEN=...` output
  3. `OPENAI_API_KEY=...` output

---

## P1-01 — ARCH-004 Move execute and host-ops orchestration into app-layer use-cases
- **Area:** Architecture
- **Status:** Closed (2026-03-15)
- **Problem:** HTTP handlers still own security-critical workflow orchestration, and env-path adapter construction is still performed lazily from server/handler code instead of the composition root.
- **Delivered:** execute and host-docker orchestration now delegate to app-layer use-cases, handlers are transport-only, env-path adapter wiring happens at startup, and startup/unit regressions cover the new composition boundary.
- **Scope:**
  - add app-layer use-cases for broker execute and host-docker execute
  - move policy sequencing, audit sequencing, timeout selection, command resolution, and process-launch orchestration behind app-layer boundaries
  - remove `ensureEnvPathSecretStore` lazy construction and require adapter wiring at startup
  - route handler reads/writes through app-layer APIs instead of reaching directly into stores where practical
- **Strict gates:**
  - handlers are transport-focused: auth gate, decode, delegate, map response
  - adapters are wired in the composition root only
  - business logic becomes unit-testable without `net/http`
- **Test scenarios:**
  1. execute allow/deny paths covered at app layer
  2. startup fails closed when env-path adapter wiring is invalid
  3. host-docker audit/rollback behavior verified without handler-only coupling

## P1-02 — UX-007 Add explicit broker client deadlines for CLI and MCP
- **Area:** UX / reliability
- **Status:** Closed (2026-03-15)
- **Problem:** CLI and MCP broker clients currently have no deadline and can hang indefinitely on stalled TCP or Unix-socket peers.
- **Delivered:** CLI and MCP broker calls now use bounded `10s` client deadlines with actionable timeout messages, and regression coverage exercises stalled TCP and Unix-socket peers.
- **Scope:**
  - add explicit request/client deadlines for broker calls in `cmd/promptlock` and `cmd/promptlock-mcp`
  - expose timeout behavior clearly in error messages and docs where operator-facing
  - preserve current Unix-socket and explicit TCP transport selection behavior
- **Strict gates:**
  - stalled broker peers fail within a bounded time
  - MCP cancellation/timeout behavior remains deterministic
- **Test scenarios:**
  1. unresponsive Unix-socket listener => timeout
  2. stalled TCP peer => timeout
  3. timeout error surfaces actionable message in CLI/MCP paths

---

## P2-01 — QA-006 Raise coverage on release/security gate binaries
- **Area:** Verification
- **Status:** Closed (2026-03-15)
- **Problem:** `cmd/promptlock-readiness-check` and large parts of `cmd/promptlock-redteam-live` are lightly or not at all exercised despite feeding release/security decisions.
- **Delivered:** direct tests now cover malformed/missing readiness inputs, `blocking=true` release-gating semantics, stdout/stderr + exit-code handling, red-team report write failures, finalize aggregation, readiness/startup negative paths, helper request/response failures, log tailing, and auth-stage aborts without relying only on Make-level coverage.
- **Scope:**
  - add tests for malformed readiness files and blocking classification logic
  - add scenario-level tests for live red-team runner report creation and failure paths
  - add timeout-path coverage for CLI/MCP broker request paths introduced by `UX-007`
- **Strict gates:**
  - release/security gate code paths have direct tests, not only indirect Make coverage
  - negative/error classification paths are covered

## P2-02 — QA-007 Expand static analysis over shipped shell workflows
- **Area:** Tooling / developer UX
- **Status:** Closed (2026-03-15)
- **Problem:** `make lint` only syntax-checks two shell scripts while more trusted release/security scripts remain outside static analysis.
- **Delivered:** `make lint` now syntax-checks every shipped shell workflow under `scripts/`, covering the helpers invoked by Make, CI, and release gates while keeping the operator-facing targets unchanged.
- **Scope:**
  - either add static analysis coverage for the shipped shell workflows or migrate the remaining trusted shell gates into Go-based commands
  - keep `Makefile` workflows stable for operators
- **Strict gates:**
  - `make lint` covers the scripts the project asks users and CI to trust
  - quoting/portability regressions are caught before runtime
