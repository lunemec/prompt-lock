#!/usr/bin/env python3
from pathlib import Path
import re
import sys

p = Path('CHANGELOG.md')
if not p.exists():
    print('ERROR: CHANGELOG.md missing')
    sys.exit(1)

text = p.read_text(encoding='utf-8')

if '## [Unreleased]' not in text:
    print('ERROR: CHANGELOG.md must contain "## [Unreleased]" section')
    sys.exit(1)

versions = re.findall(r'^## \[(?!Unreleased)([^\]]+)\]', text, flags=re.M)
for v in versions:
    if not re.match(r'^\d+\.\d+\.\d+$', v):
        print(f'ERROR: Invalid release version heading: {v} (expected SemVer like 1.2.3)')
        sys.exit(1)

# ensure unreleased appears before first version section
unrel_idx = text.find('## [Unreleased]')
other_idx = min([text.find(f'## [{v}]') for v in versions], default=10**9)
if unrel_idx > other_idx:
    print('ERROR: [Unreleased] section must be first among release sections')
    sys.exit(1)

print('CHANGELOG validation passed')
