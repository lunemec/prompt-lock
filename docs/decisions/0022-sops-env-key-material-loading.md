# 0022 - SOPS-managed runtime and fsync key-material loading

- Status: accepted
- Date: 2026-03-10

## Context
PromptLock previously required sensitive runtime and release key material to be present as plaintext process environment variables before startup or fsync gate execution. This satisfied fail-closed checks but left `SEC-001` open for SOPS-managed secret workflows.

## Decision
1. Introduce a shared Go loader (`internal/sopsenv`) that:
   - decrypts a SOPS file via `sops --decrypt`
   - accepts decrypted JSON-object or dotenv payloads
   - loads key/value pairs into process env while preserving already-set env values
   - supports required-key enforcement and fails closed when required envs remain unset.
2. Add startup integration in `promptlockd`:
   - read optional `PROMPTLOCK_SOPS_ENV_FILE`
   - load SOPS-managed env values before non-dev validation checks.
3. Add fsync tooling integration:
   - `promptlock-storage-fsync-check --sops-env-file ...`
   - `promptlock-storage-fsync-validate --sops-env-file ...`
   - fallback to `PROMPTLOCK_SOPS_ENV_FILE` when flag is not set.
4. Update release/readiness Make workflows to accept `SOPS_ENV_FILE=...` for fsync report generation/validation gates.

## Consequences
- Operators can keep key material in SOPS-managed files while retaining existing env-name contracts.
- Existing workflows remain compatible; direct env injection still works without SOPS.
- Runtime now depends on `sops` availability when SOPS file loading is configured.
- PromptLock still performs fail-closed validation after SOPS load, preserving guardrail behavior.

## Security implications
- Reduces plaintext key handling in shell history/operator workflows by allowing encrypted-at-rest key bundles.
- Existing env vars are preserved by design, allowing explicit secure runtime overrides without unexpected replacement.
- If decryption fails, startup/gate execution fails closed.
- This does not replace secret-distribution governance by itself; file access controls and SOPS key-management policy remain operator responsibilities.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
