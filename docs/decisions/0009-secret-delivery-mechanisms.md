# 0009 - Secret delivery mechanisms: env, ephemeral files, and in-memory handles

- Status: accepted
- Date: 2026-03-07

## Context
Some tools only support environment variables; others require file-based credentials (e.g., auth JSON). PromptLock needs a secure, practical delivery model across both.

## Decision
PromptLock supports delivery mechanisms in this preference order:

1. **Process-scoped env injection**
2. **Ephemeral file in tmpfs with strict permissions**
3. **In-memory file descriptor / memfd style delivery** (where supported)

All mechanisms are tied to approved lease scope and command execution context.

## Required controls
- Least-privilege file permissions (0600 equivalent).
- Time-bounded lifecycle with immediate cleanup on process exit/timeout.
- No persistence of secret material in repository/workspace paths.
- Full audit trail for secret materialization events.

## Consequences
- Gives adapters a clear preference order without forcing a single mechanism for every client.
- Preserves portability across tools that require file-based credentials.

## Security implications
- File-based delivery can still leak if process is malicious/compromised.
- Mechanism choice reduces accidental exposure but does not eliminate intentional exfiltration once plaintext is read.
- Must be paired with containment controls (ADR-0010).

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: none
