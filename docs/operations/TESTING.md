# Testing Guide

## Standard checks

```bash
make validate-final
```

Includes the `make toolchain-guard` drift check plus lint/security/docs/changelog/tests.

## PR-grade validation

```bash
make release-readiness-gate-core
```

This is the non-Docker validation path mirrored by CI for pull requests.

## Fuzzing (quick pass)

```bash
make fuzz
```

Current fuzz targets:
- MCP input parsing/validation
- broker execute-command policy validation

## Real-path docker-compose smoke

```bash
make e2e-compose
```

This builds broker + runner images and executes a real promptlock flow via `promptlock exec --broker-exec`.

## Notes
- `e2e-compose` is a practical integration smoke test, not a full security proof.
- Keep hardened config defaults in normal deployments.
- `.toolchain.env` is the canonical source for repository Go and Docker base-image pins.
- Update `.toolchain.env` first, then run `make toolchain-guard` and `make release-readiness-gate` when changing Go or Docker base-image versions.
