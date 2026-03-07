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
