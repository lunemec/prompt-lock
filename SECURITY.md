# Security Policy

## Supported versions

PromptLock is currently pre-1.0. Security fixes are applied to the latest `main` branch and latest tagged prerelease.

## Reporting a vulnerability

Please **do not open a public issue** for suspected vulnerabilities.

Report privately to maintainers via:
- GitHub Security Advisory (preferred), or
- direct maintainer contact listed in repository settings.

Include:
- affected version/commit,
- reproduction steps,
- impact assessment,
- any suggested remediation.

## Response targets

- Initial acknowledgement: within **72 hours**
- Triage decision: within **7 days**
- Fix or mitigation plan for confirmed high/critical issues: as soon as practical, target **14 days**

## Disclosure process

- We follow coordinated disclosure.
- Reporter credit is provided unless anonymity is requested.
- Public advisory is published after mitigation is available.

## Scope notes

PromptLock is currently experimental and pre-1.0, with active production-readiness hardening underway. Current scope includes non-production defaults and in-memory stores for some components.
Please include deployment profile details (`security_profile`, transport mode, auth settings) in reports.
