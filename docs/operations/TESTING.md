# Testing Guide

## Standard checks

```bash
make validate-final
```

Includes lint/security/docs/changelog/tests.

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
