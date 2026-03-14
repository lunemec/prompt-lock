# Hardened migration checklist

Use this checklist to move from compatibility mode to hardened mode.

## 1) Auth + transport
- [ ] `auth.enable_auth=true`
- [ ] `auth.operator_token` set from host secret source
- [ ] use `agent_unix_socket` + `operator_unix_socket` for local hardened mode
- [ ] ensure non-local TCP guard not bypassed

## 2) Execution mode
- [ ] wrappers use `--broker-exec`
- [ ] execution policy configured (allowlist/denylist)
- [ ] timeout and output caps tuned

## 3) Plaintext return de-risk
- [ ] verify `/v1/meta/capabilities` reports `allow_plaintext_secret_return=false` (hardened default)
- [ ] verify no workflows depend on `/v1/leases/access`
- [ ] monitor `plaintext_secret_access_blocked` audit events

## 4) Audit and operations
- [ ] host-side audit path protected
- [ ] review hash-chain audit integrity process
- [ ] review auth cleanup interval
- [ ] run periodic access/approval report
