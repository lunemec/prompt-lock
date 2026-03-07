# Next implementation piece: auth foundation

- Status: in-progress

## Scope (next coding slice)
1. Add auth config model (session/grant TTLs).
2. Add in-memory stores for bootstrap tokens, grants, and sessions.
3. Add pair/mint/revoke endpoints (minimal skeleton).
4. Add middleware to require session token on agent endpoints (initially optional toggle).
5. Add unit tests for expiry/revocation logic.

## Out of scope for this slice
- full mTLS
- production-grade persistent auth store
- external OIDC integration

## Security checks
- ensure bootstrap token one-time usage
- ensure grant idle + absolute expiry both enforced
- ensure revoked tokens fail immediately
