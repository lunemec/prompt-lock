#!/usr/bin/env python3
from pathlib import Path
import sys

# Basic policy guardrails for prototype repo
forbidden = [
    'ghp_',
    'sk-live-',
    'AKIA',
    '-----BEGIN PRIVATE KEY-----',
]

violations = []
for path in Path('.').rglob('*'):
    if not path.is_file():
        continue
    if '.git/' in str(path):
        continue
    if str(path).endswith('scripts/validate_security_basics.py'):
        continue
    if path.suffix in {'.pyc'}:
        continue
    try:
        txt = path.read_text(encoding='utf-8', errors='ignore')
    except Exception:
        continue
    for token in forbidden:
        if token in txt:
            violations.append((str(path), token))

if violations:
    print('Security baseline failed: possible secret patterns found:')
    for v in violations:
        print(f' - {v[0]} contains pattern {v[1]!r}')
    sys.exit(1)

print('Security baseline checks passed')
