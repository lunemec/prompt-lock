# Red-Team E2E Abuse Suite (Initial Scaffold)

## Goal
Provide an executable adversarial regression gate for high-risk abuse classes:
- auth bypass,
- token/session replay-like misuse,
- execution policy bypass,
- network egress bypass.

## Initial implementation
This initial scaffold uses existing high-signal tests in `cmd/promptlockd` and groups them by abuse class via `scripts/run_redteam_e2e.sh`.

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
```

## CI integration
`make ci` now includes `security-redteam` so these checks run on every CI validation pass.

## Next hardening phase
- Add dedicated replay tests for grant/session re-use after revocation.
- Add full black-box harness for endpoint-level adversarial flows against a spawned broker process.
- Emit machine-readable security findings report artifact.
