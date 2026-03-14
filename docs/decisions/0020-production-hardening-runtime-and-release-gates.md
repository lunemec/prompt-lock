# 0020 - Production hardening: client mTLS UX, release gates, and persistence safety

- Status: accepted
- Date: 2026-03-10

## Context
PromptLock's security and usability posture had remaining production gaps:
- Tagged release workflow did not enforce all checklist gates (`fuzz`, compose E2E smoke) before packaging.
- Audit writes were append-only but not explicitly synced per record.
- Persisted auth/request/lease files could be targeted by concurrent writer startups.

## Decision
1. Add `make release-readiness-gate` (`validate-final`, readiness gate, fuzz, compose E2E smoke) and run it in tagged release CI before fsync attestation/package steps.
2. Sync audit sink file descriptors on each appended record.
3. Acquire fail-closed single-writer lock files for configured persistence paths (`state_store_file.lock`, `auth.store_file.lock`) at broker startup.

## Consequences
- Tagged release CI has stronger parity with documented release checklist gates.
- Audit durability improves under crash conditions, with additional write-latency overhead.
- Concurrent writer races on local persisted state/auth files are blocked at startup.
- Multi-node/distributed persistence remains open work and is tracked separately.

## Security implications
- Improves release-time assurance by requiring fuzz and compose E2E gates.
- Lowers probability of lost/torn audit evidence during abrupt process failures.
- Reduces risk of state corruption from competing writer processes.

## Supersedes / Superseded by
- Supersedes: none
- Superseded by: `0028 - Local-only transport and removal of TCP TLS/mTLS paths` (transport portion only)
