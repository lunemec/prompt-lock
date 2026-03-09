# Go-First Tooling Migration Plan (2026-03-08)

Context: single-stack tooling discipline for PromptLock (Go-first), avoiding secondary runtimes unless explicitly approved.

Status: ✅ Completed (2026-03-09).

## Goal
Replace Python helper tooling with Go equivalents while preserving behavior and CI gates.

## Migrated tooling
- `scripts/validate_security_basics.py` -> `cmd/promptlock-validate-security`
- `scripts/validate_changelog.py` -> `cmd/promptlock-validate-changelog`
- `scripts/run_redteam_live.py` -> `cmd/promptlock-redteam-live`
- `scripts/mock-broker.py` -> `cmd/promptlock-mock-broker`

## Strict gates
- [x] New Go validators produce equivalent pass/fail behavior.
- [x] Make targets use Go binaries/`go run` path.
- [x] Python validator scripts removed.
- [x] Red-team live harness supports both `dev` and `hardened` profiles.
- [x] Red-team live harness emits machine-readable JSON report to existing report paths.
- [x] `make ci-redteam-full` remains green with Go tooling.
- [x] Equivalent mock-broker command examples updated in README/docs.

## CI and contributor workflows
- [x] Required CI/release/security automation runs with Go + shell only.
- [x] Python is no longer required for normal contributor workflows.
- [x] Docs reflect single-stack contributor expectations.
