# Key rotation and revocation runbook

This runbook documents operational steps for rotating and revoking PromptLock credentials.

## Scope
- operator token rotation
- session and grant revocation
- bootstrap token invalidation by expiry/cleanup

## 1) Operator token rotation

1. Generate new operator token in host secret manager.
2. Update PromptLock config (or `PROMPTLOCK_OPERATOR_TOKEN`) on host.
3. Restart PromptLock broker process.
4. Update host approval watcher environment.
5. Verify operator-auth endpoint access works (`/v1/requests/pending`).

### Post-rotation checks
- old operator token should fail with 401
- new token should succeed
- audit events continue writing correctly

## 2) Emergency revocation of agent access

If an agent/session is suspected compromised:

1. Revoke grant/session via operator endpoint:
   - `POST /v1/auth/revoke`
2. Restart affected agent container.
3. Re-run host pairing flow to issue fresh grant/session.
4. Review audit chain around compromise window.

## 3) Time-based hygiene
- Keep short session TTL.
- Keep absolute grant max bounded.
- Ensure cleanup loop is enabled (`cleanup_interval_seconds`).

## 4) Incident response checklist
- identify impacted agent_id/container_id
- revoke grant/session
- rotate operator token if needed
- inspect audit chain for suspicious request/access patterns
- document incident + remediation as ADR/incident note
