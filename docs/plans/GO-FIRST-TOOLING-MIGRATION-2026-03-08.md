# Go-First Tooling Migration Plan (2026-03-08)

Context: Lukas requested single-stack tooling discipline for PromptLock (Go-first), avoiding secondary runtimes unless explicitly approved.

Status: Open.

## Goal
Replace Python helper tooling with Go equivalents where practical, while preserving behavior and CI gates.

## Current non-Go tooling inventory
- `scripts/mock-broker.py`
- `scripts/validate_security_basics.py`
- `scripts/validate_changelog.py`
- `scripts/run_redteam_live.py`

## Migration order (risk-aware)

### M1 (low risk): validators
1. `validate_security_basics.py` -> `cmd/promptlock-validate-security`
2. `validate_changelog.py` -> `cmd/promptlock-validate-changelog`

**Strict gates:**
- [ ] New Go validators produce equivalent pass/fail behavior.
- [ ] Make targets use Go binaries/`go run` path.
- [ ] Python validator scripts removed or deprecated with explicit sunset note.

### M2 (medium risk): red-team live harness
3. `run_redteam_live.py` -> `cmd/promptlock-redteam-live`

**Strict gates:**
- [ ] Supports both `dev` and `hardened` profiles.
- [ ] Emits machine-readable JSON report to same path conventions.
- [ ] `make ci-redteam-full` remains green.

### M3 (optional): mock broker replacement
4. `mock-broker.py` -> Go mock broker binary (only if still needed after broker maturity)

**Strict gates:**
- [ ] Clear reason to keep mock broker.
- [ ] Equivalent command examples in README/docs.

## CI and release integration
- [ ] Add check that no new Python tooling is introduced without explicit approval note.
- [ ] Keep shell tooling only where appropriate (wrapper scripts, packaging), with Go for core logic.

## Definition of done
- [ ] All required automation for CI/release/security is runnable with Go + shell only.
- [ ] Python is no longer required for normal contributor workflows.
- [ ] Docs updated to reflect single-stack contributor expectations.
