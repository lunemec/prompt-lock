package main

import (
	"path/filepath"
	"testing"
)

func TestNewDaemonFlagsUsesConfigScopedPIDFileWhenUnset(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "workspace-instance", "config.json")

	flags := newDaemonFlags("", "promptlockd", configPath, "", false)

	want := filepath.Join(filepath.Dir(configPath), defaultDaemonPIDFileName)
	if flags.PIDFile != want {
		t.Fatalf("pid file = %q, want %q", flags.PIDFile, want)
	}
	wantLogFile := filepath.Join(filepath.Dir(configPath), defaultDaemonLogFileName)
	if flags.LogFile != wantLogFile {
		t.Fatalf("log file = %q, want %q", flags.LogFile, wantLogFile)
	}
}

func TestNewDaemonFlagsPreservesExplicitPIDFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "workspace-instance", "config.json")
	explicitPIDFile := filepath.Join(t.TempDir(), "custom", "daemon.pid")

	flags := newDaemonFlags(explicitPIDFile, "promptlockd", configPath, "", false)

	if flags.PIDFile != explicitPIDFile {
		t.Fatalf("pid file = %q, want explicit %q", flags.PIDFile, explicitPIDFile)
	}
}

func TestNewDaemonFlagsFallsBackToLegacyPIDFileWithoutConfig(t *testing.T) {
	flags := newDaemonFlags("", "promptlockd", "", "", false)

	if flags.PIDFile != defaultDaemonPIDFile {
		t.Fatalf("pid file = %q, want %q", flags.PIDFile, defaultDaemonPIDFile)
	}
	if flags.LogFile != "" {
		t.Fatalf("log file = %q, want empty fallback", flags.LogFile)
	}
}
