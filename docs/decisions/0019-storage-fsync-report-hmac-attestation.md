# 0019 - Storage fsync report HMAC attestation

- Status: accepted
- Date: 2026-03-09

## Context
Storage fsync readiness evidence previously relied on metadata provenance fields (`schema_version`, `generated_at`, `generated_by`, `hostname`) without cryptographic attestation. Metadata-only provenance does not prove report origin or tamper resistance for release/readiness gates.

## Decision
1. Add HMAC attestation envelope to storage fsync JSON reports:
   - `signature.alg` (must be `hmac-sha256`)
   - `signature.key_id`
   - `signature.value` (base64-encoded HMAC)
2. Define deterministic signing payload serialization that excludes the `signature` envelope itself.
3. Require key material from environment variables (no embedded secrets):
   - `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY`
   - `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID`
4. Update validator logic to verify HMAC signature, algorithm, key-id match, and base64 validity.
5. Enforce fail-closed behavior in storage fsync release/readiness gates when signatures are missing or invalid.
6. Add validator key-rotation support with explicit overlap controls:
   - optional keyring env `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING` using `<key_id>:<env_var_name>` entries
   - optional overlap env `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE` to cap age accepted for non-primary key IDs
   - fail closed when rotated key IDs are unknown, keyring entries are malformed, key envs are missing/invalid, overlap is disabled, or overlap age is exceeded

## Consequences
- Release/readiness evidence now has cryptographic provenance, not just metadata attribution.
- Operators must provision and rotate fsync attestation keys in runtime/release environments.
- Fsync JSON generation/validation workflows fail fast when key env wiring is missing.
- Existing unsigned report artifacts are no longer acceptable for release/readiness gate validation.
- Rotation overlap policy is explicit and time-bounded, so old key acceptance does not remain open-ended.

## Security implications
- Mitigates metadata-only spoofing or tampering of fsync evidence reports.
- Improves chain-of-custody confidence for release gating by binding content to trusted key material.
- Introduces key-management operational responsibility; weak or leaked keys reduce trust guarantees and must be rotated.
- Limits blast radius of rotated key reuse by enforcing overlap-window expiry for non-primary verification keys.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
