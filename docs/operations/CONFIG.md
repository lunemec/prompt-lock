# Host configuration

PromptLock loads host configuration from:

- `PROMPTLOCK_CONFIG` env var path, or
- default `/etc/promptlock/config.json`

Environment variables `PROMPTLOCK_ADDR` and `PROMPTLOCK_AUDIT_PATH` override config file values.

## Example config

```json
{
  "address": ":8765",
  "audit_path": "/var/log/promptlock/audit.jsonl",
  "policy": {
    "default_ttl_minutes": 5,
    "min_ttl_minutes": 1,
    "max_ttl_minutes": 30,
    "max_secrets_per_request": 5
  },
  "auth": {
    "enable_auth": true,
    "session_ttl_minutes": 10,
    "grant_idle_timeout_minutes": 480,
    "grant_absolute_max_minutes": 10080,
    "bootstrap_token_ttl_seconds": 60
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

## Notes
- Keep this file host-owned and permission-restricted.
- Do not place this config in agent-writable workspace mounts.
- Prefer external secret backends for production (Vault/1Password/etc).
