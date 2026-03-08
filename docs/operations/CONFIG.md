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
- `grant_absolute_max_minutes` supports very long runs (days) while still enforcing eventual re-pairing.
- Keep session TTL short; use pairing grant for idle-resilient re-mint.
- `operator_token` is mandatory when auth is enabled.
- `allow_plaintext_secret_return=false` is recommended for hardened mode; plaintext secret API calls are blocked.
- `cleanup_interval_seconds` controls background auth garbage collection of expired/used records.
- For hardened local deployments, prefer `unix_socket` and keep TCP on localhost only.
- CLI clients can target unix socket with `--broker-unix-socket` / `PROMPTLOCK_BROKER_UNIX_SOCKET`.
- If auth is enabled and TCP is non-local without unix socket, broker fails to start unless `PROMPTLOCK_ALLOW_INSECURE_TCP=1` is set.

## Profile presets
- `security_profile: dev` keeps compatibility-oriented defaults.
- `security_profile: hardened` tightens execution limits and disables plaintext secret return by default.

## Execution policy notes
- `execution_policy.allowlist_prefixes` restricts executable entrypoints for broker-exec mode.
- `execution_policy.denylist_substrings` blocks suspicious command patterns.
- `output_security_mode` controls broker-exec output exposure: `none` (suppress), `redacted` (default), or `raw`.
- `max_output_bytes` limits output returned from broker execution (applied after output mode processing).
- `default_timeout_sec` and `max_timeout_sec` enforce execution time bounds.

## Host Docker mediation policy notes
- `host_ops_policy.docker_allow_subcommands` allowlists Docker subcommands for host execution.
- `host_ops_policy.docker_compose_allow_verbs` allowlists compose verbs.
- `host_ops_policy.docker_ps_allowed_flags` and `docker_images_allowed_flags` restrict accepted flags.
- `host_ops_policy.docker_deny_substrings` blocks dangerous argument patterns.
- `host_ops_policy.docker_timeout_sec` limits host Docker command runtime.

## Network egress policy notes
- `network_egress_policy.enabled` toggles domain checks for broker-exec commands.
- `network_egress_policy.require_intent_match` requires intent-specific domain mapping.
- `network_egress_policy.allow_domains` defines fallback global domains.
- `network_egress_policy.intent_allow_domains` defines per-intent destination domains.
- `network_egress_policy.deny_substrings` blocks dangerous target patterns (metadata endpoints, local pivots, etc.).
- Denials are audit-logged as `network_egress_blocked`.

## Notes
- Keep this file host-owned and permission-restricted.
- Do not place this config in agent-writable workspace mounts.
- Prefer external secret backends for production (Vault/1Password/etc).
