#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

SCRIPT="scripts/validate_repo_hygiene.sh"

# Guard against reintroducing GNU-specific find options that break BSD/macOS.
if grep -Eq -- '(^|[[:space:]])-regextype([[:space:]]|$)' "$SCRIPT"; then
  echo "Hygiene portability check failed: GNU-only find option '-regextype' is not allowed in $SCRIPT" >&2
  exit 1
fi

echo "Hygiene portability check passed."
