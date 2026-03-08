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

## Secure transport recipes
- Preferred: expose PromptLock via unix socket (`unix_socket`) and keep TCP local-only.
- If TCP is required: keep `address` on localhost or put behind authenticated mTLS reverse proxy.
- `PROMPTLOCK_ALLOW_INSECURE_TCP=1` is an explicit emergency override; use only for controlled testing and rotate credentials afterward.

## Future
- Provide docker-compose example:
  - broker service
  - optional approval service
  - local demo client container
