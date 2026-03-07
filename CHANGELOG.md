# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]
### Added
- Initial Secret Lease Broker prototype with request/approve/access flow.
- Agent skill for requesting short-lived secret leases.
- AGENTS/docs harness structure and security-focused standards.
- Final validation gate with changelog + security baseline checks.
- Initial Go implementation skeleton (`cmd/promptlockd`, core domain, ports/adapters, service layer).
- Unit tests for policy and lease request/approve/access flow.
- ADR-0003 documenting Codex token access and lease-renewal model.
- ADR-0004 documenting Codex CLI integration strategy.
- ADR-0005 documenting multi-CLI auth integration model.
- ADR-0006/0007/0008 for intent API, TTL-bypass prevention, and MCP-first protocol exposure.
- ADR-0009/0010 for secret delivery mechanisms and leakage containment policy.
- CLI compatibility matrix draft and integration discovery plan docs.
- Host configuration docs and sample config file.
- Secure execution flow and threat-model documentation.
- `promptlock exec` prototype wrapper command for intent/secrets-driven execution.
- Intent resolution endpoint and request status endpoint in Go broker.
- Lease query by request endpoint to support external approval flow.
- Pending requests endpoint for approval watcher.
- `promptlock approve-queue` host-side watcher CLI for approving/denying queued requests.
- Added explicit watcher subcommands: `approve-queue list|allow|deny`.
- Added auth foundation: pairing bootstrap/grant/session/revoke endpoints and in-memory auth store.
- Added auth config block for long-running container support (idle + absolute grant TTL).
- Added initial endpoint authz enforcement (operator token + agent session token) when auth is enabled.
- Added actor attribution fields in audit events (`actor_type`, `actor_id`) and operator action audit records.
- Added transport hardening defaults: localhost TCP default, optional unix socket listener, and non-local TCP guard when auth is enabled.
- Added tamper-evident audit hash-chain records in file sink.
- Added auth cleanup loop for expired bootstrap/session records and stale grant revocation.
- Added operator/session role auth headers wiring in CLI paths.
- Added authz unit tests for operator and agent session checks.
- Added policy gate to block plaintext secret-return endpoint when configured (`allow_plaintext_secret_return=false`).
- Added `/v1/meta/capabilities` endpoint and wrapper capability pre-checks.
- Added `/v1/leases/execute` broker-side execution endpoint for hardened mode path.
- Added wrapper `--broker-exec` support to run through execute-with-secret flow.
- Added execute-with-secret endpoint tests.
- Added broker execution policy controls (allowlist/denylist, output cap, timeout bounds) with tests.
- Added authz matrix tests (operator vs agent endpoint token separation).
- Added ADR-0012 and hardened migration checklist to prefer broker-exec mode.
- Added unix-socket client support in wrapper (`--broker-unix-socket`) and transport safety tests.
- Added experimental MCP adapter scaffold (`cmd/promptlock-mcp`) with capability-first tool (`execute_with_intent`).
- Added MCP adapter input validation and unit tests.
- Added initial MCP RPC response-shape tests and MCP integration test roadmap.
- Added MCP stdio harness tests for initialize/tools/list and tools/call mocked-broker roundtrip.
- Added MCP tools/call live broker-backed E2E positive-path harness test.
- Wrapper execution docs and intent examples in config.

### Changed
- Wrapper now waits for external approval by default, with polling/timeout controls.
- `--auto-approve` is gated behind `PROMPTLOCK_DEV_MODE=1`.
- Added basic risky-command policy gate in wrapper (override available for explicit use).
- Added command fingerprint binding between lease requests and secret access.
- Added working-directory fingerprint binding between lease requests and secret access.
- Project naming adopted: **PromptLock**.
- Documentation updated to mark PromptLock as the primary product name/tagline.
- Discovery finalized and v1 requirements captured in ADR-0002.
- Contract docs aligned to v1 policy (default TTL 5m, explicit secrets, reusable-until-expiry leases).
- README updated with agent-generated code note and codex-docker workflow alignment.
- Go broker now supports host config file loading (`PROMPTLOCK_CONFIG`) for address/audit/policy/secrets.
- Request TTL now defaults to policy default when omitted.

## [0.1.0] - 2026-03-07
### Added
- Draft repository structure and core documentation.
