#!/usr/bin/env bash
set -euo pipefail

PKG="./cmd/promptlockd"

run_group() {
  local name="$1"
  local pattern="$2"
  echo "[redteam] group: $name"
  if go test "$PKG" -run "$pattern" -count=1; then
    echo "[redteam] PASS: $name"
  else
    echo "[redteam] FAIL: $name" >&2
    return 1
  fi
}

run_group "auth-bypass-role-confusion" "TestOperatorEndpointRejectsAgentToken|TestAgentEndpointRejectsOperatorToken|TestRequireOperator|TestRequireAgentSession|TestAuthRateLimiterThresholdAndRecovery"
run_group "replay-and-stale-misuse-boundaries" "TestPairCompleteRejectsContainerMismatch|TestAccessBlockedWhenPlaintextDisabled"
run_group "execution-policy-bypass" "TestValidateExecuteCommand|TestExecuteHardenedRejectsMissingIntent|TestExecuteHardenedRejectsShellWrapper"
run_group "network-egress-bypass" "TestNetworkEgressPolicy|TestNetworkEgressExtractsNonURLDomainForms|TestNetworkEgressIntentDeterministic|TestNetworkEgressBlocksPrivateIPTargets|TestNetworkEgressRejectsDirectNetworkClientWithoutInspectableDestination|TestExecuteRejectsDirectNetworkClientWithoutInspectableDestinationForApprovedLease"

echo "[redteam] all groups passed"
