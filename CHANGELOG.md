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

### Changed
- Project naming adopted: **PromptLock**.
- Documentation updated to mark PromptLock as the primary product name/tagline.
- Discovery finalized and v1 requirements captured in ADR-0002.
- Contract docs aligned to v1 policy (default TTL 5m, explicit secrets, reusable-until-expiry leases).
- README updated with agent-generated code note and codex-docker workflow alignment.

## [0.1.0] - 2026-03-07
### Added
- Draft repository structure and core documentation.
