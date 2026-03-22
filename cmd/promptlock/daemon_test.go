package main

import (
	"os"
	"path/filepath"
	"strings"
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
	origGetwd := setupGetwd
	setupGetwd = func() (string, error) { return t.TempDir(), nil }
	t.Cleanup(func() { setupGetwd = origGetwd })

	flags := newDaemonFlags("", "promptlockd", "", "", false)

	if flags.PIDFile != defaultDaemonPIDFile {
		t.Fatalf("pid file = %q, want %q", flags.PIDFile, defaultDaemonPIDFile)
	}
	if flags.LogFile != "" {
		t.Fatalf("log file = %q, want empty fallback", flags.LogFile)
	}
}

func TestBuildDaemonCommandFallsBackToBuiltSourceBinary(t *testing.T) {
	origLookPath := daemonLookPath
	origBuildSource := daemonBuildSourceBinary
	origSourceBuildAllowed := daemonSourceBuildAllowed
	t.Cleanup(func() {
		daemonLookPath = origLookPath
		daemonBuildSourceBinary = origBuildSource
		daemonSourceBuildAllowed = origSourceBuildAllowed
	})

	var buildConfig string
	daemonLookPath = func(name string) (string, error) {
		if name == "promptlockd" {
			return "", os.ErrNotExist
		}
		return "/usr/bin/" + name, nil
	}
	daemonBuildSourceBinary = func(configPath string) (string, error) {
		buildConfig = configPath
		return filepath.Join(t.TempDir(), "promptlockd-autostart"), nil
	}
	daemonSourceBuildAllowed = func() bool { return true }

	configPath := filepath.Join(t.TempDir(), "workspace-instance", "config.json")
	cmd, launchDesc, err := buildDaemonCommand(daemonFlags{
		Binary: "promptlockd",
		Config: configPath,
	})
	if err != nil {
		t.Fatalf("buildDaemonCommand: %v", err)
	}
	if cmd.Path == "" || strings.Contains(cmd.Path, "go") {
		t.Fatalf("expected built promptlockd path, got %q", cmd.Path)
	}
	if strings.Contains(launchDesc, "go run") {
		t.Fatalf("expected non-go-run launch description, got %q", launchDesc)
	}
	if buildConfig != configPath {
		t.Fatalf("build config path = %q, want %q", buildConfig, configPath)
	}
}

func TestProcessAliveTreatsZombieStateAsDead(t *testing.T) {
	origProcessState := daemonReadProcessState
	daemonReadProcessState = func(pid int) (string, error) { return "Z", nil }
	t.Cleanup(func() { daemonReadProcessState = origProcessState })

	if processAlive(os.Getpid()) {
		t.Fatalf("expected zombie-marked process to be treated as dead")
	}
}
