#!/usr/bin/env bash
set -euo pipefail

# Best-effort guard for accidental secret/token leakage in generated reports/fixtures.

TARGETS=("reports" "fixtures" "testdata")
PATTERN='(sk-[A-Za-z0-9]{10,}|api[_-]?key[=:][^[:space:],;]+|secret[=:][^[:space:],;]+|token[=:][^[:space:],;]+|Bearer [A-Za-z0-9._-]{10,})'

found=0
for t in "${TARGETS[@]}"; do
  if [[ -d "$t" ]]; then
    if grep -R -n -E "$PATTERN" "$t" >/tmp/promptlock-leak-scan.$$ 2>/dev/null; then
      echo "[leak-guard] potential secret leak patterns found under $t:" >&2
      cat /tmp/promptlock-leak-scan.$$ >&2
      found=1
    fi
    rm -f /tmp/promptlock-leak-scan.$$ || true
  fi
done

if [[ "$found" -ne 0 ]]; then
  echo "Secret leakage guard failed." >&2
  exit 1
fi

echo "Secret leakage guard passed."
