# 0025 - Local-only OSS v1 transport scope and TLS/mTLS quarantine

- Status: superseded
- Date: 2026-03-14

## Context
PromptLock now has a strong local hardened deployment shape:
- host-only operator socket
- agent-only socket mounted into the untrusted container
- agent session auth plus operator auth
- host-mediated execution and audit paths

The repository also contains TCP TLS/mTLS transport code and client flags. That transport work is useful for future or private deployments, but it adds certificate-provisioning complexity and broadens the operational surface. For the first public OSS release, the goal is a secure, teachable, copy-pasteable local workflow that users will actually adopt.

## Decision
1. The supported OSS v1 deployment target is local-only hardened operation with dual Unix sockets.
2. Non-local TCP TLS/mTLS is not part of the OSS v1 release bar, sign-off checklist, or primary operator docs.
3. Existing TLS/mTLS code remains in the repository for future/private evaluation, but it is quarantined in documentation as experimental and out of OSS v1 scope.
4. Release, runbook, config, and walkthrough docs must present the dual-socket local path as the canonical flow.

## Consequences
- The public OSS story is simpler: users can run `promptlockd`, `promptlock watch`, and `promptlock auth docker-run ...` without broker transport flags or certificate setup.
- Release readiness can focus on the transport model that matches the documented day-1 deployment shape.
- TLS/mTLS code can continue to evolve without being misrepresented as a supported OSS guarantee.

## Security implications
- This narrows the supported surface to the transport model with the smallest practical attack surface for local host-plus-container operation.
- It avoids creating false confidence around partially operationalized remote transport features.
- Quarantining TLS/mTLS from the OSS v1 story does not weaken local security; it clarifies the trust boundary and expected deployment shape.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: `0028 - Local-only transport and removal of TCP TLS/mTLS paths`
