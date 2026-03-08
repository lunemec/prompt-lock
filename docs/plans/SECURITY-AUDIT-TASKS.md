# SECURITY AUDIT TASKS

Status: Proposed  
Owner: PromptLock maintainers  
Source: Security audit findings (2026-03-08)

This plan decomposes audit findings into implementable tasks with explicit success gates.

---

## SEC-001: Replace predictable tokens with cryptographically secure tokens

Status: ✅ Completed (2026-03-08)

### Problem
Bootstrap/grant/session/lease tokens are currently sequence-derived and guessable.

### Scope
- Replace sequence-based token generation for:
  - bootstrap token
  - grant id/token
  - session token
  - lease token
- Use CSPRNG-backed entropy (minimum 32 bytes raw entropy).
- Keep token formats stable enough for logs/tests where possible (`boot_`, `grant_`, `sess_`, `lease_` prefix is acceptable).

### Success gates
- [x] No auth/lease token is generated from counters or timestamps.
- [x] Unit tests assert token uniqueness across large sample.
- [x] Unit tests assert minimum entropy/length constraints.
- [x] Existing auth and lease integration tests pass.

---

## SEC-002: Bind bootstrap token to container identity and enforce match

Status: ✅ Completed (2026-03-08)

### Problem
Pairing flow accepts caller-supplied container identity without strict bind verification against bootstrap issuance.

### Scope
- Persist container binding metadata at bootstrap creation.
- Enforce exact match on pair completion.
- Reject pair completion on mismatch.
- Keep one-time-use + short TTL behavior.

### Success gates
- [x] Pairing with mismatched container identity returns 403.
- [x] Pairing with matching identity succeeds.
- [x] Replay of consumed bootstrap token fails deterministically.
- [x] Audit event clearly records mismatch denials (without secret/token disclosure).

---

## SEC-003: Harden defaults for non-dev deployments

Status: ✅ Completed (2026-03-08)

### Problem
Current defaults allow insecure operation (auth disabled and plaintext return allowed) unless operator hardens manually.

### Scope
- Introduce strict startup profile behavior:
  - production/non-dev requires auth enabled
  - plaintext secret return disabled by default
  - unsafe profile requires explicit opt-in
- Clarify profile behavior in docs and examples.

### Success gates
- [x] Starting in non-dev profile with auth disabled fails fast with actionable error.
- [x] Plaintext secret return is off by default in hardened profile.
- [x] Example config and operations docs match runtime behavior.
- [x] Backward-compatible dev mode remains available and explicitly labeled insecure.

---

## SEC-004: Remove sensitive token material from audit and logs

Status: ✅ Completed (2026-03-08)

### Problem
Audit/log records may expose lease/session identifiers and high-risk command details.

### Scope
- Redact or hash sensitive token fields before writing audit events.
- Avoid logging raw secrets or raw bearer-like values.
- Ensure command logging is policy-aware and minimally necessary.

### Success gates
- [x] No raw lease/session/bootstrap tokens appear in audit file.
- [x] Security tests verify redaction/masking for token-like values.
- [x] Manual grep over test fixtures/log output shows no bearer/token leakage.

---

## SEC-005: Strengthen execution policy beyond command[0] allowlisting

Status: ✅ Completed (2026-03-08)

### Problem
Allowlisting only the executable prefix (e.g., `bash`) is too permissive and allows policy bypass via shell payloads.

### Scope
- Introduce hardened-mode constraints for broker-exec:
  - intent-bound command templates and/or disallow raw shell wrappers
  - stricter deny patterns for shell metacharacter misuse
- Keep compatibility mode for development with explicit warning.

### Success gates
- [x] Hardened profile rejects raw shell wrappers that do not match policy.
- [x] Intent-bound allowed command patterns are enforced server-side.
- [x] Policy rejection reasons are explicit and test-covered.

---

## SEC-006: Improve network egress enforcement reliability

Status: ✅ Completed (2026-03-08)

### Problem
Current URL/domain extraction is partial and can be bypassed by non-URL forms.

### Scope
- Expand parser coverage for common CLI patterns (`curl host`, tool-specific flags, URL-less host args).
- Normalize/validate against canonical hostnames and reject direct IP metadata targets.
- Document boundary: broker-level checks are advisory unless combined with host/network controls.

### Success gates
- [x] Tests cover URL + non-URL domain argument forms.
- [x] Denylist catches known metadata/local pivot targets.
- [x] Intent/domain allowlist behavior is deterministic and tested.
- [x] Docs clearly state guarantees and limitations.

---

## SEC-007: Add auth abuse protections (rate limits + lockout signals)

Status: ✅ Completed (2026-03-08)

### Problem
Auth and pairing endpoints currently lack brute-force and abuse throttling controls.

### Scope
- Add configurable request rate limiting for:
  - bootstrap create
  - pair complete
  - session mint
  - protected endpoint auth failures
- Emit audit events for threshold breaches.

### Success gates
- [x] Repeated invalid auth attempts are throttled.
- [x] Throttling policy is configurable and documented.
- [x] Tests verify limit behavior and recovery windows.

---

## SEC-008: Improve transport hardening defaults

### Problem
Localhost checks exist, but expanded deployments need stronger mandatory transport controls.

### Scope
- Require unix socket or explicit secure transport policy for non-local usage.
- Keep insecure override behind explicit, noisy flag.
- Tighten docs around secure deployment modes.

### Success gates
- [ ] Non-local auth-enabled startup fails unless secure transport requirements are met.
- [ ] Insecure override path is explicit and audit-logged at startup.
- [ ] Operations docs include clear secure deployment recipes.

---

## SEC-009: Strengthen tamper resistance for audit chain anchoring

### Problem
Hash-chain audit is tamper-evident but can still be reset without external anchoring.

### Scope
- Add optional checkpoint anchoring (e.g., signed periodic checkpoint or external append-only sink).
- Provide verification tooling for chain integrity from genesis/checkpoint.

### Success gates
- [ ] Verification command can detect truncation/rewrite.
- [ ] Optional anchor/checkpoint mode documented and test-covered.
- [ ] Runbook includes incident steps for audit integrity failures.

---

## SEC-010: Repository hygiene for tmp/sync/conflict artifacts

### Problem
Syncthing/tmp/conflict files are present in tree and increase leak/misconfiguration risk.

### Scope
- Expand `.gitignore` coverage for `.syncthing.*`, conflict artifacts, and runtime junk.
- Add CI gate to fail on prohibited file patterns.
- Clean existing tracked/untracked artifacts as appropriate.

### Success gates
- [ ] CI fails when forbidden temp/conflict artifacts are introduced.
- [ ] Workspace tree is clean from sync tmp artifacts.
- [ ] Developer docs mention local hygiene expectations.

---

## Suggested execution order
1. SEC-001, SEC-002
2. SEC-003, SEC-004
3. SEC-005, SEC-006
4. SEC-007, SEC-008
5. SEC-009, SEC-010

This order front-loads token/auth correctness and secret leakage prevention before policy-depth and hygiene improvements.
