# Next step plan: execute-with-secret mode

- Status: planned

## Why
Current wrapper flow still depends on plaintext secret-return endpoint for env injection. In hardened mode (`allow_plaintext_secret_return=false`), this path is blocked.

## Goal
Implement non-plaintext "execute-with-secret" mode where broker-controlled execution path can inject secrets without returning raw values to caller.

## Scope (proposed)
1. Add `/v1/leases/execute` (or equivalent) contract.
2. Enforce command + workdir binding on execute path.
3. Ensure audit events include actor and command metadata.
4. Keep plaintext path disabled in hardened mode.

## Risks to handle
- command exfiltration through stdout/stderr
- command policy bypass attempts
- runtime isolation of execution environment

## Expected outcome
PromptLock can operate in hardened mode without needing plaintext secret-return API.
