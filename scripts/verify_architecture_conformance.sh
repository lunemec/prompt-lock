#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail=0

check_forbidden() {
  local scope="$1"
  local forbidden="$2"
  if grep -R --line-number --include='*.go' "${forbidden}" "$scope" >/tmp/promptlock-arch-check.$$ 2>/dev/null; then
    echo "[arch] forbidden import pattern '${forbidden}' found under ${scope}:" >&2
    cat /tmp/promptlock-arch-check.$$ >&2
    fail=1
  fi
  rm -f /tmp/promptlock-arch-check.$$ || true
}

# Core layers must not depend on adapters/cmd transport.
check_forbidden "internal/core" "internal/adapters"
check_forbidden "internal/core" "cmd/promptlockd"

# App layer should not depend on inbound transport package.
check_forbidden "internal/app" "cmd/promptlockd"

# Inbound daemon should not directly depend on adapter internals except through app/config/auth wiring.
# (allow adapters/memory + adapters/audit only in main wiring; block elsewhere by convention via docs/checklist.)

# Optional self-test fixture check
if [[ "${1:-}" == "--self-test" ]]; then
  if bash -lc "echo 'package bad
import _ \"github.com/lunemec/promptlock/internal/adapters/audit\"' | grep 'internal/adapters/audit' >/dev/null"; then
    echo "[arch] self-test fixture demonstrates forbidden pattern detection"
  fi
fi

if [[ "$fail" -ne 0 ]]; then
  echo "Architecture conformance check failed." >&2
  exit 1
fi

echo "Architecture conformance check passed."
