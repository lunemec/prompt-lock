# Red-Team E2E Abuse Suite (Initial Scaffold)

## Goal
Provide an executable adversarial regression gate for high-risk abuse classes:
- auth bypass,
- token/session replay-like misuse,
- execution policy bypass,
- network egress bypass.

## Initial implementation
This implementation now has two layers:
1. In-process grouped security test runs via `scripts/run_redteam_e2e.sh`
2. Live broker black-box harness via `scripts/run_redteam_live.py` (machine-readable JSON report)

## Abuse classes mapped to tests

### 1) Auth bypass and role confusion
- `TestOperatorEndpointRejectsAgentToken`
- `TestAgentEndpointRejectsOperatorToken`
- `TestRequireOperator`
- `TestRequireAgentSession`
- `TestAuthRateLimiterThresholdAndRecovery`

### 2) Replay / stale session misuse boundaries
- `TestPairCompleteRejectsContainerMismatch`
- `TestAccessBlockedWhenPlaintextDisabled`

### 3) Execution policy bypass
- `TestValidateExecuteCommand`
- `TestExecuteHardenedRejectsMissingIntent`
- `TestExecuteHardenedRejectsShellWrapper`

### 4) Egress bypass
- `TestNetworkEgressPolicy`
- `TestNetworkEgressExtractsNonURLDomainForms`
- `TestNetworkEgressIntentDeterministic`
- `TestNetworkEgressBlocksPrivateIPTargets`

## Run

```bash
make security-redteam
make security-redteam-live
```

## CI integration
- `make ci` includes `security-redteam` on every validation pass.
- `make ci-redteam-full` runs `validate-final` plus the live black-box harness and writes `reports/redteam-live.json`.

## Next hardening phase
- Add dedicated replay tests for grant/session re-use after revocation.
- Add full black-box harness for endpoint-level adversarial flows against a spawned broker process.
- Emit machine-readable security findings report artifact.
