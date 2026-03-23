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

func TestResolveVersionFallsBackToVCSRevisionForDevelBuilds(t *testing.T) {
	got := resolveVersion("dev", func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "0123456789abcdef0123456789abcdef"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	})
	if got != "dev+0123456789ab-dirty" {
		t.Fatalf("ResolveVersion() = %q, want dev+0123456789ab-dirty", got)
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
