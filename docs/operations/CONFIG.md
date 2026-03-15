# Host configuration

PromptLock loads host configuration from:

- `PROMPTLOCK_CONFIG` env var path, or
- default `/etc/promptlock/config.json`

Environment variables `PROMPTLOCK_ADDR`, `PROMPTLOCK_AUDIT_PATH`, `PROMPTLOCK_UNIX_SOCKET`, `PROMPTLOCK_AGENT_UNIX_SOCKET`, `PROMPTLOCK_OPERATOR_UNIX_SOCKET`, and state-store env overrides (`PROMPTLOCK_STATE_STORE_TYPE`, `PROMPTLOCK_STATE_STORE_EXTERNAL_URL`, `PROMPTLOCK_STATE_STORE_EXTERNAL_AUTH_TOKEN_ENV`, `PROMPTLOCK_STATE_STORE_EXTERNAL_TIMEOUT_SEC`) override config file values.
`PROMPTLOCK_SOPS_ENV_FILE` (optional) points to a SOPS-encrypted dotenv/JSON payload that is decrypted at startup to populate runtime key env vars before non-dev fail-closed checks.

## Example config

```json
{
  "security_profile": "hardened",
  "address": "127.0.0.1:8765",
  "agent_unix_socket": "/var/run/promptlock-agent.sock",
  "operator_unix_socket": "/var/run/promptlock-operator.sock",
  "audit_path": "/var/log/promptlock/audit.jsonl",
  "state_store_file": "/var/lib/promptlock/state-store.json",
  "state_store": {
    "type": "file"
  },
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
    "in_memory_hardened": "fail"
  }
}
```

This example shows the recommended modern pattern: hardened profile plus `secret_source`.
For demos that intentionally use `in_memory`, set `secret_source.type` to `in_memory` and provide `secrets[]` values explicitly.
For external HTTP-backed secret retrieval, set `secret_source.type` to `external` and keep `external_url` + `external_auth_token_env` configured.
Do not leave backend fields for inactive modes in canonical configs. `state_store.external_*` applies only when `state_store.type=external`, and `secret_source.file_path` / `secret_source.external_*` apply only to `secret_source.type=file|external`.
If hardened local config omits all socket fields and keeps the listener local-only, PromptLock defaults to `/tmp/promptlock-agent.sock` and `/tmp/promptlock-operator.sock`.

## Auth notes
- `enable_auth=true` enables pairing/session endpoints and is recommended for non-demo use.
- `rate_limit_window_seconds` and `rate_limit_max_attempts` control auth endpoint throttling and abuse protection.
- `grant_absolute_max_minutes` supports very long runs (days) while still enforcing eventual re-pairing.
- Keep session TTL short; use pairing grant for session re-mint while the grant itself remains active.
- `operator_token` is mandatory when auth is enabled.
- `allow_plaintext_secret_return=false` is recommended for hardened mode; plaintext secret API calls are blocked regardless of auth mode.
- `cleanup_interval_seconds` controls background auth garbage collection of expired/used records.
- `store_file` (optional) enables durable auth bootstrap/grant/session persistence to a host path.
- `store_encryption_key_env` configures which environment variable supplies the auth-store encryption key. In non-dev profiles, `store_file` requires this key and startup fails when it is missing.
- `PROMPTLOCK_SOPS_ENV_FILE` can preload encrypted runtime key env vars (for example auth-store encryption key) from a decrypted SOPS payload. Existing process env values are preserved if already set.
- SOPS integration expects the `sops` binary to be available on PATH at runtime; decryption failures fail startup closed.
- `state_store_file` enables durable local request/lease state persistence when `state_store.type=file` (default).
- `state_store.type=external` uses an HTTP-backed state adapter instead of local `state_store_file` persistence.
- `state_store.external_auth_token_env` should point to a bearer token env var for state backend authentication.
- `state_store.external_timeout_sec` controls HTTP timeout when `state_store.type=external`.
- Current limitation: `state_store.type=external` provides durability/availability integration but does not add concurrency-safe multi-writer request/lease transitions. Treat it as a single-writer or carefully controlled deployment path until atomic transition semantics are added.
- External state backend API contract:
  - `PUT /v1/state/requests/{id}`, `GET /v1/state/requests/{id}`, `DELETE /v1/state/requests/{id}`, `GET /v1/state/requests/pending`
  - `PUT /v1/state/leases/{token}`, `GET /v1/state/leases/{token}`, `DELETE /v1/state/leases/{token}`, `GET /v1/state/leases/by-request/{request_id}`
- Persistence writes now fail closed: if auth-store or request/lease state persistence fails at runtime, PromptLock closes a durability gate, audit-logs the failure, and mutating auth/lease endpoints return `503 Service Unavailable`.
- Persistence writes fsync both data files and parent directories after atomic rename. Use host storage/filesystems that support directory sync semantics for crash-consistent metadata updates.
- PromptLock acquires fail-closed single-writer locks for configured persistence files (`state_store_file.lock`, `auth.store_file.lock`) at startup; concurrent writer startups against the same files are blocked.
- When `state_store.type=external`, request/lease operations fail closed with `503` when backend connectivity/auth fails.
- Audit sink now syncs each appended record to disk; this improves crash durability of audit events at the cost of write-latency overhead.
- Use `make storage-fsync-preflight MOUNT_DIR=/path/to/mount` to validate mount behavior before rollout.
- For multi-mount evidence capture, use `make storage-fsync-report MOUNT_DIRS=/path/a,/path/b`.
- Signed fsync JSON reports require HMAC key material in environment:
  - `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY` (32+ character key)
  - `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID` (rotation/audit identifier)
- Optional verification key-rotation env for overlap windows:
  - `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEYRING` (comma-separated `<key_id>:<env_var_name>` entries for non-primary verification keys)
  - `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE` (Go duration, for example `168h`; reports signed with non-primary key IDs are rejected when older than this value)
  - each keyring entry env var must contain a 32+ character HMAC key (for example `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_PREV`)
- Optional SOPS-backed key loading for fsync tooling:
  - set `PROMPTLOCK_SOPS_ENV_FILE=/path/to/fsync-keys.sops.env` or pass `--sops-env-file /path/to/fsync-keys.sops.env` to `promptlock-storage-fsync-check` / `promptlock-storage-fsync-validate`
  - Make targets accept `SOPS_ENV_FILE=/path/to/fsync-keys.sops.env`
  - decrypted payload keys must use the same env names referenced by fsync settings (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY`, `PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_ID`, optional keyring env vars)
- Validate report JSON via `make storage-fsync-validate FSYNC_REPORT=reports/storage-fsync-report.json` (fails if malformed, signature is missing/invalid, or any mount reports `ok=false`).
- Report validation checks metadata fields (`schema_version`, `generated_at` RFC3339, `generated_by`, `hostname`) and signature envelope fields (`signature.alg=hmac-sha256`, `signature.key_id`, `signature.value` base64 HMAC).
- Validator remains fail closed during rotation: unknown `signature.key_id`, malformed keyring entries, missing keyring env vars, disabled overlap windows (`PROMPTLOCK_STORAGE_FSYNC_HMAC_KEY_OVERLAP_MAX_AGE` unset/zero), or expired overlap age all fail validation.
- For release/readiness, use one-shot gate `make storage-fsync-release-gate MOUNT_DIRS=/path/a,/path/b FSYNC_REPORT=reports/storage-fsync-report.json` (fails closed when signature validation fails).
- For hardened local deployments, prefer `agent_unix_socket` + `operator_unix_socket` and do not expose the operator socket to containers.
- CLI now auto-selects local sockets by role when no broker transport flags/env are given:
  - `promptlock watch` and `promptlock auth bootstrap` target the operator socket.
  - `promptlock exec`, `promptlock auth pair`, and `promptlock auth mint` target the agent socket.
  - `promptlock auth login` and `promptlock auth docker-run` use operator socket for bootstrap and agent socket for pair/mint.
- If the expected local role socket is missing, the CLI now fails closed instead of silently falling back to `http://127.0.0.1:8765`.
- Use `--broker` or `PROMPTLOCK_BROKER_URL` only as an explicit opt-in when you intentionally want TCP transport.
- Role-specific env overrides:
  - `PROMPTLOCK_OPERATOR_UNIX_SOCKET`
  - `PROMPTLOCK_AGENT_UNIX_SOCKET`
- Compatibility env/flag remain available for legacy or single-socket flows:
  - `--broker-unix-socket`
  - `PROMPTLOCK_BROKER_UNIX_SOCKET`
  - `unix_socket`
- If auth is enabled and TCP is non-local without a unix socket, broker fails to start unless `PROMPTLOCK_ALLOW_INSECURE_TCP=1` is set.
- Using `PROMPTLOCK_ALLOW_INSECURE_TCP=1` emits a startup warning and audit event (`startup_insecure_tcp_override`).
- If auth is disabled and TCP is non-local without a unix socket, broker fails to start unless `PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP=1` is set.
- Using `PROMPTLOCK_ALLOW_INSECURE_NOAUTH_TCP=1` emits a startup warning and audit event (`startup_insecure_noauth_tcp_override`).
- Running with `auth.enable_auth=false` and `allow_plaintext_secret_return=true` triggers insecure dev-mode warning/audit (`startup_insecure_dev_mode_warning`).

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
- Any non-dev profile requires `auth.store_file` to be configured.
- Any non-dev profile requires either:
  - `state_store.type=file` with `state_store_file` configured, or
  - `state_store.type=external` with `state_store.external_url` (`https://...`) and a non-empty token env value from `state_store.external_auth_token_env`.
- Any non-dev profile requires `secret_source.type` to be `env`, `file`, or `external` (not `in_memory`).

## Execution policy notes
- `execution_policy.exact_match_executables` is the canonical broker-exec executable allowlist key. Matching is exact executable identity after basename normalization, not prefix matching. `go` allows `go` and `/usr/local/bin/go`; it does not allow `goevil`.
- `execution_policy.allowlist_prefixes` remains a legacy migration alias. If both keys are present, `exact_match_executables` wins and `allowlist_prefixes` is ignored.
- In `security_profile: hardened`, broker-exec additionally requires `intent`, rejects raw shell wrappers (`bash`/`sh`/`zsh`), and tightens allowlist defaults to non-shell tool entrypoints (`npm`, `node`, `go`, `python`, `pytest`, `make`, `git`).
- In `security_profile: hardened`, explicit `execution_policy.exact_match_executables` additions are merged with the hardened defaults, but raw shell wrappers remain disallowed even if configured.
- `execution_policy.command_search_paths` defines the broker-managed executable lookup path for host-side execution. Bare command names are resolved only from these directories.
- When `command_search_paths` is unset, PromptLock defaults to common host-owned tool directories (for example `/usr/local/bin`, `/usr/local/sbin`, `/usr/local/go/bin`, `/opt/homebrew/bin`, `/usr/bin`, and `/bin` on Unix-like hosts).
- Path-like `command[0]` values are allowed only when the supplied path itself points inside one of `command_search_paths`.
- Broker-exec child processes receive the same managed `command_search_paths` as their `PATH` baseline instead of inheriting the broker process `PATH`.
- In hardened mode, command-smuggling markers (`&&`, `||`, `;`, `$(`, backticks) are denied by default.
- `execution_policy.denylist_substrings` blocks suspicious command patterns.
- Broker and local CLI child processes receive only a minimal baseline environment plus leased secrets. The built-in baseline is limited to `PATH`, `HOME`, `TMPDIR`, `TMP`, `TEMP`, `SYSTEMROOT`, `COMSPEC`, `PATHEXT`, and `USERPROFILE`.
- Broker-managed path resolution is stronger than basename-only matching, but it is still not immutable or cryptographic provenance. PromptLock trusts the configured `command_search_paths` directories and the executable entries they expose, including common package-manager symlink entries.
- Residual limit: PromptLock does not verify file hashes, signatures, ownership, or mount immutability for resolved binaries. If you need stronger provenance than trusted-directory resolution, use explicit host hardening or add absolute-path/hash pinning outside the current OSS baseline.
- Local CLI non-broker exec still carries its minimal ambient `PATH` baseline. The managed-path provenance control applies to broker-host execution (`--broker-exec`) and host-docker mediation on the broker.
- Policy-denied responses include remediation hints (e.g., remove shell wrapper, add intent-specific domain, use allowlisted flags).
- `output_security_mode` controls broker-exec output exposure: `none` (suppress), `redacted`, or `raw`.
- Base config default is `redacted`, but the hardened profile defaults to `none` unless you explicitly override it.
- `redacted` mode applies token-aware best-effort masking for common bearer and env-style secret shapes. It must not be treated as a strong containment control for hostile command output.
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
- The approved request/lease intent is persisted and reused at execute time. A caller cannot widen egress scope by supplying a different execute-time intent.
- `network_egress_policy.require_intent_match` requires intent-specific domain mapping.
- `network_egress_policy.allow_domains` defines fallback global domains.
- `network_egress_policy.intent_allow_domains` defines per-intent destination domains.
- `network_egress_policy.deny_substrings` blocks dangerous target patterns (metadata endpoints, local pivots, etc.).
- Direct network clients that rely on broker argv inspection (`curl`, `wget`, `fetch`) are denied when no inspectable destination is present in argv.
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

## `env_path` approval boundary
- `PROMPTLOCK_ENV_PATH_ROOT` defines the only root from which approved `.env` files may be resolved.
- If `PROMPTLOCK_ENV_PATH_ROOT` is unset, the broker falls back to its current working directory. Treat that as a compatibility default, not a hardened one.
- Broker startup fails closed if the chosen env-path root cannot be initialized.
- `env_path` values are canonicalized through symlink resolution and rejected if they escape the configured root.
- Request payloads do not define `env_path_canonical`; the broker computes it from `env_path`. Any legacy client-supplied `env_path_canonical` value is ignored for compatibility.
- Approved requests store both `env_path` and `env_path_canonical` so operators can review the agent-supplied path and the broker-confirmed path together.
- Execute-time secret access fails closed if the approved canonical path is missing or does not match the current resolved path.

## Notes
- Keep this file host-owned and permission-restricted.
- Do not place this config in agent-writable workspace mounts.
- If using `auth.store_file`, place it on host-protected storage (not agent-writable paths).
- Prefer external secret backends for production (Vault/1Password/etc).
