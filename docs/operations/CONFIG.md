# Host configuration

PromptLock loads host configuration from:

- `PROMPTLOCK_CONFIG` env var path, or
- default `/etc/promptlock/config.json`

Environment variables `PROMPTLOCK_ADDR` and `PROMPTLOCK_AUDIT_PATH` override config file values.

## Example config

```json
{
  "address": "127.0.0.1:8765",
  "unix_socket": "/tmp/promptlock.sock",
  "audit_path": "/var/log/promptlock/audit.jsonl",
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
    "cleanup_interval_seconds": 60
  },
  "secrets": [
    { "name": "github_token", "value": "REPLACE_ME" },
    { "name": "npm_token", "value": "REPLACE_ME" }
  ]
}
```

## Auth notes
- `enable_auth=true` enables pairing/session endpoints and is recommended for non-demo use.
- `rate_limit_window_seconds` and `rate_limit_max_attempts` control auth endpoint throttling and abuse protection.
- `grant_absolute_max_minutes` supports very long runs (days) while still enforcing eventual re-pairing.
- Keep session TTL short; use pairing grant for idle-resilient re-mint.
- `operator_token` is mandatory when auth is enabled.
- `allow_plaintext_secret_return=false` is recommended for hardened mode; plaintext secret API calls are blocked.
- `cleanup_interval_seconds` controls background auth garbage collection of expired/used records.
- `store_file` (optional) enables durable auth bootstrap/grant/session persistence to a host path.
- For hardened local deployments, prefer `unix_socket` and keep TCP on localhost only.
- CLI clients can target unix socket with `--broker-unix-socket` / `PROMPTLOCK_BROKER_UNIX_SOCKET`.
- If auth is enabled and TCP is non-local without unix socket or TLS, broker fails to start unless `PROMPTLOCK_ALLOW_INSECURE_TCP=1` is set.
- Using `PROMPTLOCK_ALLOW_INSECURE_TCP=1` emits a startup warning and audit event (`startup_insecure_tcp_override`).

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

Startup guardrails:
- Any non-dev profile requires `auth.enable_auth=true`.
- `security_profile=insecure` fails fast unless `PROMPTLOCK_ALLOW_INSECURE_PROFILE=1` is set.

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

## Notes
- Keep this file host-owned and permission-restricted.
- Do not place this config in agent-writable workspace mounts.
- If using `auth.store_file`, place it on host-protected storage (not agent-writable paths).
- Prefer external secret backends for production (Vault/1Password/etc).
