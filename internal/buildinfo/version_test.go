package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersionPrefersExplicitInjectedVersion(t *testing.T) {
	got := resolveVersion("v1.2.3", func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v9.9.9"}}, true
	})
	if got != "1.2.3" {
		t.Fatalf("ResolveVersion() = %q, want 1.2.3", got)
	}
}

func TestResolveVersionFallsBackToBuildInfoModuleVersion(t *testing.T) {
	got := resolveVersion("", func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Version: "v0.4.0"}}, true
	})
	if got != "0.4.0" {
		t.Fatalf("ResolveVersion() = %q, want 0.4.0", got)
	}
}

func TestResolveVersionNormalizesPseudoVersionsToDev(t *testing.T) {
	for _, version := range []string{
		"0.0.0-20260323102103-06079e3985f8",
		"0.0.0-20260323102103-06079e3985f8+dirty",
	} {
		got := resolveVersion("", func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{
				Main: debug.Module{Version: version},
			}, true
		})
		if got != "dev" {
			t.Fatalf("ResolveVersion(%q) = %q, want dev", version, got)
		}
	}
}

func TestResolveVersionDefaultsToDevWithoutBuildInfo(t *testing.T) {
	got := resolveVersion("", func() (*debug.BuildInfo, bool) {
		return nil, false
	})
	if got != "dev" {
		t.Fatalf("ResolveVersion() = %q, want dev", got)
	}
}
