#!/usr/bin/env bash
set -euo pipefail

# Hardened profile smoke matrix:
# 1) unix socket safety path
# 2) TLS/mTLS transport runtime checks
# 3) live hardened red-team harness

go test ./cmd/promptlockd -run 'TestValidateTransportSafety|TestValidateTransportSafetyWithTLS|TestValidateTLSConfig|TestMTLSRejectsClientWithoutCertificate|TestTLSListenerStartsWithCertKey' -count=1

make security-redteam-live-hardened

echo "Hardened smoke suite passed."
