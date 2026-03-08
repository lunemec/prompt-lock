# Hardened Smoke Suite

Purpose: quick confidence check for hardened deployment paths.

## Run

```bash
make hardened-smoke
```

## What it covers
1. Transport safety checks including unix-socket and TLS paths.
2. TLS/mTLS runtime behavior tests (`cmd/promptlockd` transport/runtime tests).
3. Live hardened broker abuse-path smoke run (`security-redteam-live-hardened`).

## Expected output
- `Hardened smoke suite passed.`
- `reports/redteam-live-hardened.json` updated.

If it fails, inspect the failing test/log section first, then re-run just that step.
