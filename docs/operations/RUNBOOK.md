# RUNBOOK

## Local dev
- Start mock broker: `python3 scripts/mock-broker.py`
- Request lease: `scripts/secretctl.sh request ...`
- Approve lease: `scripts/human-approve.sh <request_id> <ttl>`
- Access secret: `scripts/secretctl.sh access --lease <lease> --secret <name>`

## Security operations
- Keep audit trail on host storage (not container-writable paths).
- Rotate demo secrets before any non-local use.
- Treat this repository as prototype until production hardening is completed.

### Audit integrity verification
- Verify full hash-chain:
  - `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl`
- Optional checkpoint anchoring:
  - `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl --checkpoint /var/log/promptlock/audit.checkpoint --write-checkpoint`

### Periodic verification routine (recommended)
- Daily (or before release):
  1. `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl`
  2. `go run ./cmd/promptlock audit-verify --file /var/log/promptlock/audit.jsonl --checkpoint /var/log/promptlock/audit.checkpoint --write-checkpoint`
  3. Record result in ops log with timestamp + operator id.
- After any broker restart/config change:
  1. Run full verify once.
  2. Compare with last checkpoint window.
  3. If mismatch, trigger incident flow below.

### Incident response for audit integrity failures
- If verification fails, immediately:
  1. Freeze broker writes (stop service or switch to read-only mode).
  2. Preserve current audit files and host/system logs for forensics.
  3. Compare with last known checkpoint and identify divergence window.
  4. Rotate operator/session credentials and re-pair agents.
  5. Resume only after root-cause review and clean checkpoint re-established.
