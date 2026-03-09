# Dockerization

Yes, dockerizing this tool makes sense.

## Why
- consistent runtime across host OSes
- easier integration with codex-docker and CI
- cleaner separation between broker process and agent containers

## Recommended deployment split
1. **Broker container (trusted)**
   - holds policy + lease logic
   - writes audit trail to host-mounted protected path
   - reads host config from mounted `/etc/promptlock/config.json`
2. **Agent/workload container(s) (untrusted or mixed trust)**
   - do not hold raw secrets
   - request time-bound leases from broker

## Minimum Docker security defaults
- run as non-root where possible
- read-only root filesystem
- drop Linux capabilities
- no-new-privileges
- constrained writable mounts
- host-side protected audit mount
- prefer unix socket exposure over broad TCP bind for broker API

## Hardened production baseline (recommended)
- `security_profile: hardened`
- `auth.enable_auth: true`
- `auth.allow_plaintext_secret_return: false`
- `unix_socket` enabled and permission-restricted (preferred transport)
- TCP, if enabled, bound to localhost only or behind authenticated mTLS proxy
- host-protected audit path outside agent-writable mounts
- explicit secret/session backend strategy (do not rely only on in-memory defaults)

## Secure transport recipes
- Preferred: expose PromptLock via unix socket (`unix_socket`) and keep TCP local-only.
- If TCP is required: either enable native TLS (`tls.enable`) or place broker behind authenticated mTLS reverse proxy.
- Native mTLS can be enabled with `tls.require_client_cert=true` and `tls.client_ca_file`.
- Canonical hardened mTLS setup: `docs/operations/MTLS-HARDENED.md`.
- `PROMPTLOCK_ALLOW_INSECURE_TCP=1` is an explicit emergency override; use only for controlled testing and rotate credentials afterward.

## Canonical real flow
- Use `docs/operations/REAL-E2E-HOST-CONTAINER.md` for host daemon + container agent + interactive approval walkthrough.

## Future
- Provide docker-compose example:
  - broker service
  - optional approval service
  - local demo client container
