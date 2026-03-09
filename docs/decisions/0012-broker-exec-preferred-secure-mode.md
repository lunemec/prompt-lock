# 0012 - Broker-exec as preferred secure mode

- Status: accepted
- Date: 2026-03-07

## Context
Plaintext secret-return APIs are high risk in adversarial/prompt-injection scenarios. PromptLock now has broker-side execute-with-secret capability.

## Decision
- Prefer `/v1/leases/execute` and wrapper `--broker-exec` mode for hardened environments.
- Treat plaintext `/v1/leases/access` as compatibility path only.
- In hardened deployments, set `auth.allow_plaintext_secret_return=false`.

## Migration guidance
1. Enable `--broker-exec` in wrappers.
2. Turn on execution policy controls (allowlist/denylist, timeout, output cap).
3. Disable plaintext secret-return via config.
4. Monitor audit events for blocked plaintext calls and policy denials.

## Security implications
- Reduces direct secret value exposure to caller process.
- Improves policy and audit control over secret-dependent execution.
- Requires careful command policy tuning to avoid overblocking needed workflows.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
