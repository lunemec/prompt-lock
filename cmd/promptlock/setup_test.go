package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildWorkspaceSetupLayoutUsesWorkspaceRootAndHostStateDir(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	subdir := filepath.Join(repoRoot, "nested", "workspace")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatalf("mkdir subdir: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	layout, err := buildWorkspaceSetupLayout(subdir, stateDir)
	if err != nil {
		t.Fatalf("buildWorkspaceSetupLayout: %v", err)
	}
	if layout.WorkspaceRoot != repoRoot {
		t.Fatalf("workspace root = %q, want %q", layout.WorkspaceRoot, repoRoot)
	}
	wantPrefix := filepath.Join(stateDir, "promptlock", "workspaces") + string(os.PathSeparator)
	if !strings.HasPrefix(layout.InstanceDir, wantPrefix) {
		t.Fatalf("instance dir = %q, want prefix %q", layout.InstanceDir, wantPrefix)
	}
	if strings.HasPrefix(layout.InstanceDir, repoRoot+string(os.PathSeparator)) {
		t.Fatalf("instance dir must stay outside workspace, got %q under %q", layout.InstanceDir, repoRoot)
	}
	for _, path := range []string{
		layout.ConfigPath,
		layout.EnvPath,
		layout.AuditPath,
		layout.StateStorePath,
		layout.AuthStorePath,
	} {
		if !strings.HasPrefix(path, layout.InstanceDir+string(os.PathSeparator)) {
			t.Fatalf("generated path %q must stay under instance dir %q", path, layout.InstanceDir)
		}
	}
	if layout.SocketDir == "" {
		t.Fatalf("expected non-empty socket dir")
	}
	if strings.HasPrefix(layout.SocketDir, layout.InstanceDir+string(os.PathSeparator)) {
		t.Fatalf("socket dir must stay separate from instance dir, got %q under %q", layout.SocketDir, layout.InstanceDir)
	}
	for _, path := range []string{layout.AgentSocketPath, layout.OperatorSocketPath} {
		if !strings.HasPrefix(path, layout.SocketDir+string(os.PathSeparator)) {
			t.Fatalf("socket path %q must stay under socket dir %q", path, layout.SocketDir)
		}
		if len(path) >= 100 {
			t.Fatalf("socket path %q is too long for portable unix-socket use", path)
		}
	}
}

func TestEnsureWorkspaceSetupWritesSecureInstanceFiles(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	restoreRand := stubSetupRandomBytes(t)
	defer restoreRand()

	result, err := ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           stateDir,
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err != nil {
		t.Fatalf("ensureWorkspaceSetup: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected new setup to report created=true")
	}

	configBytes, err := os.ReadFile(result.ConfigPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(configBytes, &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if got := cfg["security_profile"]; got != "hardened" {
		t.Fatalf("security_profile = %#v, want hardened", got)
	}
	if got := cfg["agent_unix_socket"]; got != result.AgentSocketPath {
		t.Fatalf("agent_unix_socket = %#v, want %q", got, result.AgentSocketPath)
	}
	if got := cfg["operator_unix_socket"]; got != result.OperatorSocketPath {
		t.Fatalf("operator_unix_socket = %#v, want %q", got, result.OperatorSocketPath)
	}
	if got := cfg["agent_bridge_address"]; got != "127.0.0.1:0" {
		t.Fatalf("agent_bridge_address = %#v, want 127.0.0.1:0", got)
	}
	if got := cfg["audit_path"]; got != result.AuditPath {
		t.Fatalf("audit_path = %#v, want %q", got, result.AuditPath)
	}
	if got := cfg["state_store_file"]; got != result.StateStorePath {
		t.Fatalf("state_store_file = %#v, want %q", got, result.StateStorePath)
	}

	authCfg, ok := cfg["auth"].(map[string]any)
	if !ok {
		t.Fatalf("expected auth object in config, got %#v", cfg["auth"])
	}
	if got := authCfg["enable_auth"]; got != true {
		t.Fatalf("auth.enable_auth = %#v, want true", got)
	}
	if got := authCfg["allow_plaintext_secret_return"]; got != false {
		t.Fatalf("auth.allow_plaintext_secret_return = %#v, want false", got)
	}
	if got, _ := authCfg["operator_token"].(string); strings.TrimSpace(got) == "" {
		t.Fatalf("expected non-empty generated operator token, got %#v", authCfg["operator_token"])
	}
	if got := authCfg["store_file"]; got != result.AuthStorePath {
		t.Fatalf("auth.store_file = %#v, want %q", got, result.AuthStorePath)
	}
	if got := authCfg["store_encryption_key_env"]; got != "PROMPTLOCK_AUTH_STORE_KEY" {
		t.Fatalf("auth.store_encryption_key_env = %#v, want PROMPTLOCK_AUTH_STORE_KEY", got)
	}

	intents, ok := cfg["intents"].(map[string]any)
	if !ok {
		t.Fatalf("expected intents object, got %#v", cfg["intents"])
	}
	runTests, ok := intents["run_tests"].([]any)
	if !ok || len(runTests) != 1 || runTests[0] != "github_token" {
		t.Fatalf("intents.run_tests = %#v, want [github_token]", intents["run_tests"])
	}

	execCfg, ok := cfg["execution_policy"].(map[string]any)
	if !ok || execCfg["output_security_mode"] != "raw" {
		t.Fatalf("execution_policy.output_security_mode = %#v, want raw", cfg["execution_policy"])
	}

	envBytes, err := os.ReadFile(result.EnvPath)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	envFile := string(envBytes)
	for _, want := range []string{
		"export PROMPTLOCK_CONFIG=",
		"export PROMPTLOCK_SETUP_SOCKET_DIR=",
		"export PROMPTLOCK_AGENT_UNIX_SOCKET=",
		"export PROMPTLOCK_OPERATOR_UNIX_SOCKET=",
		"export PROMPTLOCK_AGENT_BRIDGE_ADDRESS=",
		`export PROMPTLOCK_DOCKER_HOST_ALIAS="${PROMPTLOCK_DOCKER_HOST_ALIAS:-host.docker.internal}"`,
		"export PROMPTLOCK_OPERATOR_TOKEN=",
		"export PROMPTLOCK_AUTH_STORE_KEY=",
		"export PROMPTLOCK_SECRET_GITHUB_TOKEN=",
		"demo_github_token_value",
		result.ConfigPath,
		result.SocketDir,
		result.AgentSocketPath,
		result.OperatorSocketPath,
	} {
		if !strings.Contains(envFile, want) {
			t.Fatalf("env file missing %q:\n%s", want, envFile)
		}
	}
	if strings.Contains(envFile, "export PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL=") {
		t.Fatalf("expected dynamic bridge quickstart env to omit baked-in PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL, got:\n%s", envFile)
	}

	if runtime.GOOS != "windows" {
		if info, err := os.Stat(result.InstanceDir); err != nil {
			t.Fatalf("stat instance dir: %v", err)
		} else if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("instance dir mode = %o, want 700", got)
		}
		if info, err := os.Stat(result.SocketDir); err != nil {
			t.Fatalf("stat socket dir: %v", err)
		} else if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("socket dir mode = %o, want 700", got)
		}
		for _, path := range []string{result.ConfigPath, result.EnvPath} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat %s: %v", path, err)
			}
			if got := info.Mode().Perm(); got != 0o600 {
				t.Fatalf("%s mode = %o, want 600", path, got)
			}
		}
	}
}

func TestEnsureWorkspaceSetupIsIdempotent(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	restoreRand := stubSetupRandomBytes(t)
	defer restoreRand()

	first, err := ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           stateDir,
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err != nil {
		t.Fatalf("first ensureWorkspaceSetup: %v", err)
	}
	configBefore, err := os.ReadFile(first.ConfigPath)
	if err != nil {
		t.Fatalf("read config before reuse: %v", err)
	}
	envBefore, err := os.ReadFile(first.EnvPath)
	if err != nil {
		t.Fatalf("read env before reuse: %v", err)
	}

	second, err := ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           stateDir,
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err != nil {
		t.Fatalf("second ensureWorkspaceSetup: %v", err)
	}
	if second.Created {
		t.Fatalf("expected reused setup to report created=false")
	}
	configAfter, err := os.ReadFile(second.ConfigPath)
	if err != nil {
		t.Fatalf("read config after reuse: %v", err)
	}
	envAfter, err := os.ReadFile(second.EnvPath)
	if err != nil {
		t.Fatalf("read env after reuse: %v", err)
	}
	if !bytes.Equal(configBefore, configAfter) {
		t.Fatalf("config changed on reuse")
	}
	if !bytes.Equal(envBefore, envAfter) {
		t.Fatalf("env file changed on reuse")
	}
}

func TestEnsureWorkspaceSetupRefreshesLegacySocketPathsWithoutRotatingSecrets(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	layout, err := buildWorkspaceSetupLayout(repoRoot, stateDir)
	if err != nil {
		t.Fatalf("buildWorkspaceSetupLayout: %v", err)
	}
	if err := os.MkdirAll(layout.InstanceDir, 0o700); err != nil {
		t.Fatalf("mkdir instance dir: %v", err)
	}

	legacyAgentSocket := filepath.Join(layout.InstanceDir, "agent.sock")
	legacyOperatorSocket := filepath.Join(layout.InstanceDir, "operator.sock")
	configBody := []byte(fmt.Sprintf(`{
  "security_profile": "hardened",
  "agent_unix_socket": %q,
  "operator_unix_socket": %q,
  "agent_bridge_address": "127.0.0.1:8766",
  "auth": {"operator_token": "op_existing"},
  "state_store": {"type": "file"}
}
`, legacyAgentSocket, legacyOperatorSocket))
	if err := os.WriteFile(layout.ConfigPath, configBody, 0o600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	legacyEnv := strings.Join([]string{
		"export PROMPTLOCK_SETUP_WORKSPACE_ROOT=" + shellQuote(repoRoot),
		"export PROMPTLOCK_SETUP_INSTANCE_DIR=" + shellQuote(layout.InstanceDir),
		"export PROMPTLOCK_CONFIG=" + shellQuote(layout.ConfigPath),
		"export PROMPTLOCK_AGENT_UNIX_SOCKET=" + shellQuote(legacyAgentSocket),
		"export PROMPTLOCK_OPERATOR_UNIX_SOCKET=" + shellQuote(legacyOperatorSocket),
		"export PROMPTLOCK_OPERATOR_TOKEN='op_existing'",
		"export PROMPTLOCK_AUTH_STORE_KEY='auth_existing'",
		"export PROMPTLOCK_SECRET_GITHUB_TOKEN='secret_existing'",
		"",
	}, "\n")
	if err := os.WriteFile(layout.EnvPath, []byte(legacyEnv), 0o600); err != nil {
		t.Fatalf("write legacy env: %v", err)
	}

	result, err := ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           stateDir,
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err != nil {
		t.Fatalf("ensureWorkspaceSetup: %v", err)
	}
	if result.Created {
		t.Fatalf("expected existing setup to be refreshed, not recreated")
	}

	configBytes, err := os.ReadFile(layout.ConfigPath)
	if err != nil {
		t.Fatalf("read refreshed config: %v", err)
	}
	if strings.Contains(string(configBytes), legacyAgentSocket) || strings.Contains(string(configBytes), legacyOperatorSocket) {
		t.Fatalf("expected legacy socket paths to be removed from config: %s", string(configBytes))
	}
	if !strings.Contains(string(configBytes), layout.AgentSocketPath) || !strings.Contains(string(configBytes), layout.OperatorSocketPath) {
		t.Fatalf("expected refreshed config to contain new socket paths: %s", string(configBytes))
	}
	if !strings.Contains(string(configBytes), "op_existing") {
		t.Fatalf("expected existing operator token to be preserved: %s", string(configBytes))
	}

	envBytes, err := os.ReadFile(layout.EnvPath)
	if err != nil {
		t.Fatalf("read refreshed env: %v", err)
	}
	envFile := string(envBytes)
	if strings.Contains(envFile, legacyAgentSocket) || strings.Contains(envFile, legacyOperatorSocket) {
		t.Fatalf("expected legacy socket paths to be removed from env: %s", envFile)
	}
	for _, want := range []string{
		layout.SocketDir,
		layout.AgentSocketPath,
		layout.OperatorSocketPath,
		"op_existing",
		"auth_existing",
		"secret_existing",
	} {
		if !strings.Contains(envFile, want) {
			t.Fatalf("expected refreshed env to contain %q:\n%s", want, envFile)
		}
	}
	if strings.Contains(envFile, "export PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL=") {
		t.Fatalf("expected refreshed env to remove stale PROMPTLOCK_DOCKER_AGENT_BRIDGE_URL, got:\n%s", envFile)
	}
}

func TestEnsureWorkspaceSetupRejectsIncompleteExistingInstance(t *testing.T) {
	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	stateDir := filepath.Join(t.TempDir(), "state-home")
	layout, err := buildWorkspaceSetupLayout(repoRoot, stateDir)
	if err != nil {
		t.Fatalf("buildWorkspaceSetupLayout: %v", err)
	}
	if err := os.MkdirAll(layout.InstanceDir, 0o700); err != nil {
		t.Fatalf("mkdir instance dir: %v", err)
	}
	if err := os.WriteFile(layout.ConfigPath, []byte(`{"security_profile":"hardened"}`), 0o600); err != nil {
		t.Fatalf("write partial config: %v", err)
	}

	_, err = ensureWorkspaceSetup(repoRoot, workspaceSetupOptions{
		StateDir:           stateDir,
		IntentName:         "run_tests",
		SecretName:         "github_token",
		AllowDomain:        "api.github.com",
		DemoSecretValue:    "demo_github_token_value",
		OutputSecurityMode: "raw",
	})
	if err == nil {
		t.Fatalf("expected incomplete existing instance to fail")
	}
	if !strings.Contains(err.Error(), "incomplete existing workspace setup") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSetupPrintsContainerFirstCommands(t *testing.T) {
	origGOOS := setupRuntimeGOOS
	setupRuntimeGOOS = "darwin"
	t.Cleanup(func() { setupRuntimeGOOS = origGOOS })

	repoRoot := filepath.Join(t.TempDir(), "repo-root")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}

	restoreRand := stubSetupRandomBytes(t)
	defer restoreRand()
	origGetwd := setupGetwd
	setupGetwd = func() (string, error) { return repoRoot, nil }
	t.Cleanup(func() { setupGetwd = origGetwd })

	stateDir := filepath.Join(t.TempDir(), "state-home")
	out := captureCommandStdout(t, func() {
		runSetup([]string{"--state-dir", stateDir})
	})
	for _, want := range []string{
		"PromptLock local docker quickstart is ready",
		"You are ready for the first approval flow in this workspace.",
		"Next commands:",
		"Run the following commands exactly once in three terminals:",
		"Terminal A (broker host):",
		"Terminal B (operator watch UI):",
		"Terminal C (agent container launch):",
		"cd '" + repoRoot + "'",
		"go run ./cmd/promptlock daemon start",
		"go run ./cmd/promptlock watch",
		"go run ./cmd/promptlock auth docker-run",
		"docker build -t promptlock-agent-lab .",
		"instance.env",
		"Socket dir:",
		"Quickstart socket paths live under",
		"After daemon start, use `go run ./cmd/promptlock daemon status --json` to discover the active container bridge URL.",
		"agent-only loopback bridge for containerized MCP clients",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("setup output missing %q:\n%s", want, out)
		}
	}
}

func stubSetupRandomBytes(t *testing.T) func() {
	t.Helper()
	orig := setupRandomBytes
	call := 0
	setupRandomBytes = func(n int) ([]byte, error) {
		call++
		return bytes.Repeat([]byte{byte(call)}, n), nil
	}
	return func() {
		setupRandomBytes = orig
	}
}
