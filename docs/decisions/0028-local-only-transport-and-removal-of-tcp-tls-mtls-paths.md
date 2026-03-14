# 0028 - Local-only transport and removal of TCP TLS/mTLS paths

- Status: accepted
- Date: 2026-03-14

## Context
PromptLock's supported and teachable deployment story is local-only hardened operation with dual Unix sockets:
- host-only operator socket
- agent-only socket mounted into the untrusted container
- agent session auth plus operator auth
- host-mediated execution and audit paths

The repository still retained TCP TLS/mTLS transport code, client flags, tests, and docs as "experimental" or "quarantined" transport work. That retention now creates avoidable trust debt:
- it broadens the operational surface for a product that is explicitly local-only
- it leaves a misleading impression that non-local transport remains a viable future or private option
- it forces config, docs, tests, and release logic to keep discussing a transport model the project does not want to support

## Decision
1. PromptLock is a local-only tool and uses Unix sockets as its only supported transport model.
2. TCP TLS/mTLS server transport support is removed from the broker rather than retained in quarantined form.
3. Client-side broker TLS flags and env vars are removed from the CLI rather than retained for future/private use.
4. Active operator docs, architecture docs, and release docs must describe Unix-socket-only local operation.
5. Any future reintroduction of non-local transport requires a new ADR with full implementation, tests, and support commitments.

## Consequences
- The transport story becomes simpler and less misleading: PromptLock is for local host-plus-container operation only.
- Configuration surface is smaller and fail-closed by construction because unsupported remote transport options no longer exist.
- TLS/mTLS runtime bugs and stale tests stop consuming maintenance attention for a mode the project does not intend to ship.

## Security implications
- This removes a misleading retained transport surface and narrows the attack surface to the transport model the project actually supports.
- It avoids false confidence around partially operationalized or insufficiently validated remote transport features.
- It makes the documented trust boundary match the enforced transport boundary.

## Supersedes / Superseded by
- Supersedes: `0025 - Local-only OSS v1 transport scope and TLS/mTLS quarantine`
- Superseded by: none
