# Threat model and attack simulation notes

## Assets
- secrets managed by PromptLock
- operator credential
- agent session credentials
- audit trail integrity

## Trust boundaries
- host operator environment
- PromptLock broker
- agent containers
- network transport (TCP/UDS)

## Adversary assumptions
- prompt injection can influence agent commands
- compromised agent container possible
- accidental misconfiguration possible

## Key attack scenarios and controls

### A1) Agent attempts plaintext secret exfiltration
- Risk: explicit secret dump command or export to long-lived env.
- Controls:
  - broker-exec preferred mode
  - plaintext return can be disabled
  - command policy allow/deny
  - exact executable allowlist checks
  - broker-managed executable resolution from trusted search directories
  - minimal child-process environment plus leased secrets only
  - hardened default output suppression; redaction is best-effort log hygiene only
  - audit events

### A2) Role confusion (agent calls operator endpoints)
- Risk: privilege escalation via endpoint misuse.
- Controls:
  - endpoint authz split (operator vs session token)
  - authz matrix tests

### A3) Non-local exposure over TCP
- Risk: remote unauthorized access.
- Controls:
  - localhost default bind
  - unix socket option
  - startup guard for non-local TCP with auth
  - CLI fails closed when expected local role sockets are missing instead of silently downgrading to localhost TCP

### A4) Tampering with audit records
- Risk: hide malicious actions.
- Controls:
  - host-side audit path
  - hash-chain records

### A6) Agent-controlled `.env` path confusion
- Risk: agent points approval flow at an unexpected `.env` file or a symlink/traversal target.
- Controls:
  - broker canonicalizes `env_path` against `PROMPTLOCK_ENV_PATH_ROOT`
  - traversal and symlink escape rejection
  - watch UI shows both requested and canonical path
  - execute-time canonical-path revalidation before reading secrets

### A5) Stale long-lived sessions
- Risk: forgotten credentials remain valid.
- Controls:
  - idle + absolute grant expiry
  - cleanup loop
  - revoke endpoints

## Simulation checklist (manual)
1. Attempt agent call to operator endpoint (expect 401).
2. Disable plaintext and attempt `/v1/leases/access` (expect blocked).
3. Run policy-denied command via broker-exec (expect forbidden).
4. Start auth-enabled non-local TCP without unix socket (expect startup failure unless override).
5. Revoke active grant/session and verify subsequent failures.
