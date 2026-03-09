package main

import "testing"

func TestValidateChangelogPasses(t *testing.T) {
	text := `# Changelog

## [Unreleased]
### Added
- x

## [1.2.3] - 2026-03-09
### Added
- y
`
	if err := validateChangelog(text); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestValidateChangelogFailsMissingUnreleased(t *testing.T) {
	text := "## [1.2.3] - 2026-03-09"
	if err := validateChangelog(text); err == nil {
		t.Fatalf("expected missing unreleased to fail")
	}
}

func TestValidateChangelogFailsInvalidVersion(t *testing.T) {
	text := `## [Unreleased]
## [1.2] - 2026-03-09`
	if err := validateChangelog(text); err == nil {
		t.Fatalf("expected invalid version to fail")
	}
}

func TestValidateChangelogFailsOrdering(t *testing.T) {
	text := `## [1.2.3] - 2026-03-09
## [Unreleased]`
	if err := validateChangelog(text); err == nil {
		t.Fatalf("expected ordering to fail")
	}
}
