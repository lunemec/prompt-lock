#!/usr/bin/env bash
set -euo pipefail

# Hardened profile smoke matrix:
# 1) unix socket safety path
# 2) live hardened red-team harness

go test ./cmd/promptlockd -run 'TestValidateTransportSafety' -count=1

make security-redteam-live-hardened

echo "Hardened smoke suite passed."
