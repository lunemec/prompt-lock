# Host configuration

PromptLock loads host configuration from:

- `PROMPTLOCK_CONFIG` env var path, or
- default `/etc/promptlock/config.json`

Environment variables `PROMPTLOCK_ADDR` and `PROMPTLOCK_AUDIT_PATH` override config file values.

## Example config

```json
{
  "security_profile": "hardened",
  "address": "127.0.0.1:8765",
  "unix_socket": "/var/run/promptlock.sock",
  "audit_path": "/var/log/promptlock/audit.jsonl",
  "state_store_file": "/var/lib/promptlock/state-store.json",
  "policy": {
    "default_ttl_minutes": 5,
    "min_ttl_minutes": 1,
    "max_ttl_minutes": 30,
    "max_secrets_per_request": 5
  },
  "auth": {
    "enable_auth": true,
    "operator_token": "CHANGE_ME_OPERATOR_TOKEN",
    "allow_plaintext_secret_return": false,
    "session_ttl_minutes": 10,
    "grant_idle_timeout_minutes": 480,
    "grant_absolute_max_minutes": 10080,
    "bootstrap_token_ttl_seconds": 60,
    "cleanup_interval_seconds": 60,
    "rate_limit_window_seconds": 60,
    "rate_limit_max_attempts": 20,
    "store_file": "/var/lib/promptlock/auth-store.json",
    "store_encryption_key_env": "PROMPTLOCK_AUTH_STORE_KEY"
  },
  "secret_source": {
    "type": "env",
    "env_prefix": "PROMPTLOCK_SECRET_",
    "external_url": "https://secrets.example.internal",
    "external_auth_token_env": "PROMPTLOCK_EXTERNAL_SECRET_TOKEN",
    "external_timeout_sec": 10,
    "in_memory_hardened": "fail"
  }
}
```

This example shows the recommended modern pattern: hardened profile plus `secret_source`.
For demos that intentionally use `in_memory`, set `secret_source.type` to `in_memory` and provide `secrets[]` values explicitly.
For external HTTP-backed secret retrieval, set `secret_source.type` to `external` and keep `external_url` + `external_auth_token_env` configured.

## Auth notes
- `enable_auth=true` enables pairing/session endpoints and is recommended for non-demo use.
- `rate_limit_window_seconds` and `rate_limit_max_attempts` control auth endpoint throttling and abuse protection.
- `grant_absolute_max_minutes` supports very long runs (days) while still enforcing eventual re-pairing.
- Keep session TTL short; use pairing grant for idle-resilient re-mint.
- `operator_token` is mandatory when auth is enabled.
- `allow_plaintext_secret_return=false` is recommended for hardened mode; plaintext secret API calls are blocked regardless of auth mode.
- `cleanup_interval_seconds` controls background auth garbage collection of expired/used records.
- `store_file` (optional) enables durable auth bootstrap/grant/session persistence to a host path.
- `store_encryption_key_env` configures which environment variable supplies the auth-store encryption key. In non-dev profiles, `store_file` requires this key and startup fails when it is missing.
- `state_store_file` (optional) enables durable request/lease state persistence to a host path.
- Persistence writes now fail closed: if auth-store or request/lease state persistence fails at runtime, PromptLock closes a durability gate, audit-logs the failure, and mutating auth/lease endpoints return `503 Service Unavailable`.
- For hardened local deployments, prefer `unix_socket` and keep TCP on localhost only.
- CLI clients can target unix socket with `--broker-unix-socket` / `PROMPTLOCK_BROKER_UNIX_SOCKET`.
- If auth is enabled and TCP is non-local without unix socket or TLS, broker fails to start unless `PROMPTLOCK_ALLOW_INSECURE_TCP=1` is set.
- Using `PROMPTLOCK_ALLOW_INSECURE_TCP=1` emits a startup warning and audit event (`startup_insecure_tcp_override`).
- If auth is disabled and TCP is non-local without unix socket or TLS, broker fails to start unless `PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP=1` is set.
- Using `PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP=1` emits a startup warning and audit event (`startup_insecure_noauth_tcp_override`).
- Running with `auth.enable_auth=false` and `allow_plaintext_secret_return=true` triggers insecure dev-mode warning/audit (`startup_insecure_dev_mode_warning`).

## TLS / mTLS transport settings
- `tls.enable=true` enables native HTTPS listener on `address`.
- `tls.cert_file` and `tls.key_file` are required when TLS is enabled.
- `tls.require_client_cert=true` enables mTLS and requires `tls.client_ca_file`.
- In mTLS mode, clients without valid cert chains signed by `client_ca_file` are rejected.
- Canonical hardened mTLS recipe: `docs/operations/MTLS-HARDENED.md`.

## Profile presets
- `security_profile: dev` keeps compatibility-oriented defaults (**insecure for production**).
- `security_profile: hardened` tightens execution limits and disables plaintext secret return by default.
- `security_profile: insecure` is only allowed with explicit runtime opt-in: `PROMPTLOCK_ALLOW_INSECURE_PROFILE=1`.
- `security_profile: dev` now requires explicit startup opt-in: `PROMPTLOCK_ALLOW_DEV_PROFILE=1`.

Dev-mode visibility:
- `/v1/meta/capabilities` exposes `insecure_dev_mode=true` when running with auth disabled and plaintext return enabled.

Startup guardrails:
- Any non-dev profile requires `auth.enable_auth=true`.
- `security_profile=insecure` fails fast unless `PROMPTLOCK_ALLOW_INSECURE_PROFILE=1` is set.
- `security_profile=dev` fails fast unless `PROMPTLOCK_ALLOW_DEV_PROFILE=1` is set.
- Any non-dev profile requires `state_store_file` and `auth.store_file` to be configured.
- Any non-dev profile requires `secret_source.type` to be `env`, `file`, or `external` (not `in_memory`).

## Execution policy notes
- `execution_policy.allowlist_prefixes` restricts executable entrypoints for broker-exec mode.
- In `security_profile: hardened`, broker-exec additionally requires `intent`, rejects raw shell wrappers (`bash`/`sh`/`zsh`), and tightens allowlist defaults to non-shell tool entrypoints (`npm`, `node`, `go`, `python`, `pytest`, `make`, `git`).
- In hardened mode, command-smuggling markers (`&&`, `||`, `;`, `$(`, backticks) are denied by default.
- `execution_policy.denylist_substrings` blocks suspicious command patterns.
- Policy-denied responses include remediation hints (e.g., remove shell wrapper, add intent-specific domain, use allowlisted flags).
- `output_security_mode` controls broker-exec output exposure: `none` (suppress), `redacted` (default), or `raw`.
- `max_output_bytes` limits output returned from broker execution (applied after output mode processing).
- `default_timeout_sec` and `max_timeout_sec` enforce execution time bounds.

## Host Docker mediation policy notes
- `host_ops_policy.docker_allow_subcommands` allowlists Docker subcommands for host execution.
- `host_ops_policy.docker_compose_allow_verbs` allowlists compose verbs.
- In `security_profile: hardened`, compose verbs are reduced to read-only flow (`config`, `ps`).
- `host_ops_policy.docker_ps_allowed_flags` and `docker_images_allowed_flags` restrict accepted flags.
- `host_ops_policy.docker_deny_substrings` blocks dangerous argument patterns.
- `host_ops_policy.docker_timeout_sec` limits host Docker command runtime.

Example hardened allowlist snippet:
```json
"host_ops_policy": {
  "docker_allow_subcommands": ["version", "ps", "images", "compose"],
  "docker_compose_allow_verbs": ["config", "ps"]
}
```

## Network egress policy notes
- `network_egress_policy.enabled` toggles domain checks for broker-exec commands.
- Broker-level egress checks are defense-in-depth input validation, not a complete network firewall. For strong guarantees, combine with host-level egress controls.
- `network_egress_policy.require_intent_match` requires intent-specific domain mapping.
- `network_egress_policy.allow_domains` defines fallback global domains.
- `network_egress_policy.intent_allow_domains` defines per-intent destination domains.
- `network_egress_policy.deny_substrings` blocks dangerous target patterns (metadata endpoints, local pivots, etc.).
- Denials are audit-logged as `network_egress_blocked`.

## Secret source settings
- `secret_source.type` supports:
  - `in_memory` (default; uses values from `secrets[]` and demo env fallbacks)
  - `env` (reads from environment as `<env_prefix><UPPER_SECRET_NAME>`)
  - `file` (reads from JSON object at `secret_source.file_path`)
  - `external` (reads from HTTP endpoint `secret_source.external_url + /v1/secrets/{name}`)
- `secret_source.env_prefix` defaults to `PROMPTLOCK_SECRET_`.
- `secret_source.file_path` defaults to `/etc/promptlock/secrets.json` when type is `file`.
- `secret_source.external_auth_token_env` controls which environment variable supplies bearer auth for `external` source (default `PROMPTLOCK_EXTERNAL_SECRET_TOKEN`).
- `secret_source.external_timeout_sec` controls HTTP timeout for `external` source.
- `secret_source.in_memory_hardened` controls hardened behavior when using in-memory secrets:
  - `warn` (default): startup warning + audit event
  - `fail`: startup is blocked

## Notes
- Keep this file host-owned and permission-restricted.
- Do not place this config in agent-writable workspace mounts.
- If using `auth.store_file`, place it on host-protected storage (not agent-writable paths).
- Prefer external secret backends for production (Vault/1Password/etc).
