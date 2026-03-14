# 0017 - Dev-mode risk signaling and unauthenticated non-local TCP guard

- Status: accepted
- Date: 2026-03-09

## Context
PromptLock keeps a compatibility-oriented `dev` profile where auth may be disabled and plaintext secret return may remain enabled. This is useful for local workflows, but it can create high-risk exposure if operators bind the broker to non-local TCP or do not recognize they are in insecure mode.

A review on 2026-03-09 identified two concrete gaps:
1. Non-local TCP startup guard was enforced only when auth was enabled.
2. Plaintext-return deny logic depended on auth-enabled state, which could create policy ambiguity.

## Decision
1. Add fail-fast startup guard for `auth.enable_auth=false` on non-local TCP when no Unix socket is configured.
2. Keep an explicit override for this unsafe mode via `PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP=1`, with warning + audit event.
3. Enforce `allow_plaintext_secret_return=false` regardless of auth mode.
4. Surface explicit insecure runtime signal through `/v1/meta/capabilities` as `insecure_dev_mode`.
5. Emit startup warning + audit event when running with `auth=false` and plaintext return enabled.

## Consequences
- Safer defaults for accidental remote exposure in dev-like deployments.
- Clearer operator visibility into insecure runtime mode.
- Slightly stricter behavior for no-auth deployments that relied on non-local TCP.

## Security implications
- Reduces risk of unauthenticated remote lease/secret API exposure.
- Removes auth-mode coupling from plaintext-return policy enforcement.
- Preserves deliberate insecure testing through explicit, noisy override paths.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
