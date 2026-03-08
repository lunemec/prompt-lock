# Release and versioned deployment guide

This project follows SemVer and Keep-a-Changelog.

## Pre-release checklist

1. Ensure changelog is updated:
   - new work in `## [Unreleased]`
   - release section added as `## [X.Y.Z] - YYYY-MM-DD`
2. Run full validation:

```bash
make validate-final
make fuzz
```

3. Run smoke integration:

```bash
make e2e-compose
```

4. Confirm beta readiness checklist status:
   - `docs/plans/BETA-READINESS.md`
5. Confirm security policy and disclosure path are published:
   - `SECURITY.md`

## Build release artifacts

```bash
scripts/release-package.sh v0.2.0
```

Produces:
- `dist/promptlock-0.2.0.tar.gz`

## Tag and publish

```bash
git tag -a v0.2.0 -m "PromptLock v0.2.0"
git push origin v0.2.0
```

GitHub release workflow should attach build artifacts for tagged versions.

## Deployment notes

### Hardened deployment (recommended)
- `security_profile: hardened`
- `auth.enable_auth: true`
- `auth.allow_plaintext_secret_return: false`
- prefer `unix_socket` transport
- protect audit log path on host

### Compatibility deployment (temporary)
- `security_profile: dev`
- only for migration/testing scenarios
- document exceptions and timeline for hardening
