# OSS Publish TODO (Security + Architecture)

Status: in-progress (Wave 0 started, 2026-03-08)

## MUST before broader OSS launch
- [x] Add `LICENSE` file.
- [x] Add `SECURITY.md` (disclosure/reporting policy).
- [ ] Add `CONTRIBUTING.md` with security/threat-model guardrails.
- [x] Switch operator token comparison to constant-time compare.
- [x] Add hardened deployment guide (unix socket first, no insecure TCP by default).
- [x] Add explicit production warning for in-memory secret/auth stores.
- [ ] Add red-team style e2e suite (auth bypass, replay, egress bypass, policy bypass attempts).

## SHOULD before v1.0
- [ ] Move more auth/policy/exec enforcement from handlers into app/domain services.
- [ ] Split growing `promptlockd` handler surface into bounded contexts.
- [ ] Strengthen audit anchoring guidance + verification workflow docs.

## NICE post-launch
- [ ] mTLS transport profile.
- [ ] Durable secret/auth backends (Vault/1Password/KMS adapters).
- [ ] Additional protocol conformance tests for MCP clients.
