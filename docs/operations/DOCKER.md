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
- prefer role-separated unix sockets over broad TCP bind for broker API

## Hardened OSS baseline (recommended)
- `security_profile: hardened`
- `auth.enable_auth: true`
- `auth.allow_plaintext_secret_return: false`
- `agent_unix_socket` and `operator_unix_socket` enabled and permission-restricted (preferred local transport)
- no non-local TCP dependency in the supported OSS deployment story
- host-protected audit path outside agent-writable mounts
- host-protected `state_store_file` and `auth.store_file` paths outside agent-writable mounts
- `PROMPTLOCK_AUTH_STORE_KEY` (or configured `auth.store_encryption_key_env`) supplied via orchestrator secret, not baked into images
- explicit secret/session backend strategy (do not rely only on in-memory defaults)

## Secure transport recipes
- Preferred local shape: expose PromptLock via dual unix sockets on the host and give the untrusted container agent-side PromptLock transport only.
- Host operator commands (`promptlock watch`, `auth bootstrap`) should use the operator socket only from the host.
- Container agent commands should use only agent-side transport: the agent Unix socket on Linux, or the daemon-owned loopback bridge URL on non-Linux desktop Docker runtimes.
- On non-Linux desktop Docker runtimes, local hardened dual-socket mode now starts a daemon-owned loopback agent bridge by default (`agent_bridge_address`, using a dynamic loopback port) because host Unix-socket bind mounts are not reliable there. The bridge forwards only agent-socket traffic and keeps the operator socket host-only; use `promptlock daemon status --json` to discover the live container URL.
- Non-local TCP is not part of the supported OSS v1 release target.
- `PROMPTLOCK_ALLOW_INSECURE_TCP=1` is an explicit emergency override; use only for controlled testing and rotate credentials afterward.

## Canonical host-plus-container walkthrough
- Use `docs/operations/REAL-E2E-HOST-CONTAINER.md` for the CLI-first host daemon + container agent + interactive approval lab walkthrough.
- For local developer/operator ergonomics, `promptlock auth docker-run` can mint a short-lived session and launch the agent container in one command instead of requiring a separate auth-login step. On Linux it mounts the agent socket; on non-Linux desktop Docker runtimes it injects the daemon-owned bridge URL or a short-lived fallback relay.

## Future
- Provide docker-compose example:
  - broker service
  - optional approval service
  - local demo client container
