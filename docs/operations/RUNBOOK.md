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
