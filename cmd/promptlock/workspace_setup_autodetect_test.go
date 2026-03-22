package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestNewDaemonFlagsAutoDetectsWorkspaceSetupConfig(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	t.Setenv("XDG_STATE_HOME", stateDir)
	restoreRand := stubSetupRandomBytes(t)
	defer restoreRand()

	result, err := ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           "",
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err != nil {
		t.Fatalf("ensureWorkspaceSetup: %v", err)
	}

	origGetwd := setupGetwd
	setupGetwd = func() (string, error) { return repoRoot, nil }
	t.Cleanup(func() { setupGetwd = origGetwd })

	flags := newDaemonFlags("", "promptlockd", "", "", false)

	if flags.Config != result.ConfigPath {
		t.Fatalf("config path = %q, want %q", flags.Config, result.ConfigPath)
	}
	if flags.PIDFile != filepath.Join(result.InstanceDir, defaultDaemonPIDFileName) {
		t.Fatalf("pid file = %q, want %q", flags.PIDFile, filepath.Join(result.InstanceDir, defaultDaemonPIDFileName))
	}
	if flags.LogFile != filepath.Join(result.InstanceDir, defaultDaemonLogFileName) {
		t.Fatalf("log file = %q, want %q", flags.LogFile, filepath.Join(result.InstanceDir, defaultDaemonLogFileName))
	}
}

func TestResolveBrokerSelectionUsesWorkspaceSetupSocketsWhenEnvUnset(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	t.Setenv("XDG_STATE_HOME", stateDir)
	restoreRand := stubSetupRandomBytes(t)
	defer restoreRand()

	result, err := ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           "",
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err != nil {
		t.Fatalf("ensureWorkspaceSetup: %v", err)
	}

	origGetwd := setupGetwd
	setupGetwd = func() (string, error) { return repoRoot, nil }
	t.Cleanup(func() { setupGetwd = origGetwd })

	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_URL", "")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")

	operatorListener, err := net.Listen("unix", result.OperatorSocketPath)
	if err != nil {
		t.Fatalf("listen operator socket: %v", err)
	}
	t.Cleanup(func() {
		_ = operatorListener.Close()
		_ = os.Remove(result.OperatorSocketPath)
	})

	agentListener, err := net.Listen("unix", result.AgentSocketPath)
	if err != nil {
		t.Fatalf("listen agent socket: %v", err)
	}
	t.Cleanup(func() {
		_ = agentListener.Close()
		_ = os.Remove(result.AgentSocketPath)
	})

	operatorSelection, err := resolveBrokerSelection(brokerRoleOperator, brokerSelectionInput{})
	if err != nil {
		t.Fatalf("resolve operator broker selection: %v", err)
	}
	if operatorSelection.UnixSocket != result.OperatorSocketPath {
		t.Fatalf("operator unix socket = %q, want %q", operatorSelection.UnixSocket, result.OperatorSocketPath)
	}

	agentSelection, err := resolveBrokerSelection(brokerRoleAgent, brokerSelectionInput{})
	if err != nil {
		t.Fatalf("resolve agent broker selection: %v", err)
	}
	if agentSelection.UnixSocket != result.AgentSocketPath {
		t.Fatalf("agent unix socket = %q, want %q", agentSelection.UnixSocket, result.AgentSocketPath)
	}
}

func TestDaemonEnvLoadsWorkspaceSetupSecretsAndKeys(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	t.Setenv("XDG_STATE_HOME", stateDir)
	restoreRand := stubSetupRandomBytes(t)
	defer restoreRand()

	result, err := ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           "",
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err != nil {
		t.Fatalf("ensureWorkspaceSetup: %v", err)
	}

	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "")
	t.Setenv("PROMPTLOCK_SECRET_GITHUB_TOKEN", "")
	t.Setenv("PROMPTLOCK_OPERATOR_TOKEN", "")

	env := daemonEnv(result.ConfigPath)
	joined := strings.Join(env, "\n")

	for _, want := range []string{
		"PROMPTLOCK_CONFIG=" + result.ConfigPath,
		"PROMPTLOCK_OPERATOR_TOKEN=op_",
		"PROMPTLOCK_AUTH_STORE_KEY=",
		"PROMPTLOCK_SECRET_GITHUB_TOKEN=demo_github_token_value",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("daemon env missing %q:\n%s", want, joined)
		}
	}
}

func TestDefaultOperatorTokenUsesWorkspaceSetupConfigWhenEnvUnset(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	t.Setenv("XDG_STATE_HOME", stateDir)
	restoreRand := stubSetupRandomBytes(t)
	defer restoreRand()

	result, err := ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           "",
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err != nil {
		t.Fatalf("ensureWorkspaceSetup: %v", err)
	}

	cfg, err := config.Load(result.ConfigPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	origGetwd := setupGetwd
	setupGetwd = func() (string, error) { return repoRoot, nil }
	t.Cleanup(func() { setupGetwd = origGetwd })

	t.Setenv("PROMPTLOCK_OPERATOR_TOKEN", "")

	if got := defaultOperatorToken(); got != cfg.Auth.OperatorToken {
		t.Fatalf("defaultOperatorToken = %q, want %q", got, cfg.Auth.OperatorToken)
	}
}
