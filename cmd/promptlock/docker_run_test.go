package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDockerRunArgsWithUnixSocket(t *testing.T) {
	args, err := buildDockerRunArgs(dockerRunConfig{
		Image:                 "codex-promptlock",
		ContainerName:         "codex-1",
		WrapperAgentID:        "codex-agent",
		WrapperTaskID:         "codex-1",
		SessionToken:          "sess_123",
		BrokerUnixSocket:      "/Users/test/.promptlock/run/promptlock.sock",
		ContainerBrokerSocket: "/run/promptlock/promptlock.sock",
		User:                  "1000:1000",
		Entrypoint:            "/bin/sh",
		Workdir:               "/workspace",
		AdditionalMounts:      []string{"type=bind,src=/host/workspace,dst=/workspace"},
		AdditionalEnv:         []string{"CODEX_HOME=/workspace/.codex"},
		Command:               []string{"-lc", "echo ok"},
	})
	if err != nil {
		t.Fatalf("buildDockerRunArgs returned error: %v", err)
	}

	joined := strings.Join(args, "\n")
	checks := []string{
		"run",
		"--rm",
		"--name",
		"codex-1",
		"--user",
		"1000:1000",
		"--read-only",
		"--cap-drop",
		"ALL",
		"--security-opt",
		"no-new-privileges",
		"--pids-limit",
		"256",
		"--tmpfs",
		"/tmp:rw,noexec,nosuid,nodev,size=64m",
		"--mount",
		"type=bind,src=/Users/test/.promptlock/run/promptlock.sock,dst=/run/promptlock/promptlock.sock",
		"--entrypoint",
		"/bin/sh",
		"--workdir",
		"/workspace",
		"PROMPTLOCK_SESSION_TOKEN",
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN",
		"PROMPTLOCK_WRAPPER_AGENT_ID",
		"PROMPTLOCK_WRAPPER_TASK_ID",
		"PROMPTLOCK_AGENT_UNIX_SOCKET=/run/promptlock/promptlock.sock",
		"PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET",
		"CODEX_HOME=/workspace/.codex",
		"type=bind,src=/host/workspace,dst=/workspace",
		"codex-promptlock",
		"-lc",
		"echo ok",
	}
	for _, want := range checks {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected docker args to contain %q, got %q", want, joined)
		}
	}
	if strings.Contains(joined, "PROMPTLOCK_SESSION_TOKEN=sess_123") {
		t.Fatalf("expected session token value to stay out of docker argv, got %q", joined)
	}
}

func TestBuildDockerRunArgsWithBrokerURL(t *testing.T) {
	args, err := buildDockerRunArgs(dockerRunConfig{
		Image:          "codex-promptlock",
		ContainerName:  "codex-1",
		WrapperAgentID: "codex-agent",
		WrapperTaskID:  "codex-1",
		SessionToken:   "sess_123",
		BrokerURL:      "https://promptlock.example.internal:8765",
		User:           "1000:1000",
	})
	if err != nil {
		t.Fatalf("buildDockerRunArgs returned error: %v", err)
	}

	joined := strings.Join(args, "\n")
	checks := []string{
		"PROMPTLOCK_SESSION_TOKEN",
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN",
		"PROMPTLOCK_WRAPPER_AGENT_ID",
		"PROMPTLOCK_WRAPPER_TASK_ID",
		"PROMPTLOCK_BROKER_URL",
		"PROMPTLOCK_WRAPPER_BROKER_URL",
	}
	for _, want := range checks {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected docker args to contain %q, got %q", want, joined)
		}
	}
	if strings.Contains(joined, "PROMPTLOCK_SESSION_TOKEN=sess_123") {
		t.Fatalf("expected session token value to stay out of docker argv, got %q", joined)
	}
	if strings.Contains(joined, "PROMPTLOCK_BROKER_URL=https://promptlock.example.internal:8765") {
		t.Fatalf("expected broker URL value to stay out of docker argv, got %q", joined)
	}
	for _, removed := range []string{
		"PROMPTLOCK_BROKER_TLS_CA_FILE=",
		"PROMPTLOCK_BROKER_TLS_CLIENT_CERT_FILE=",
		"PROMPTLOCK_BROKER_TLS_CLIENT_KEY_FILE=",
		"PROMPTLOCK_BROKER_TLS_SERVER_NAME=",
	} {
		if strings.Contains(joined, removed) {
			t.Fatalf("expected docker args to omit removed broker TLS setting %q, got %q", removed, joined)
		}
	}
}

func TestBuildDockerRunEnvIncludesWrapperIdentity(t *testing.T) {
	env := buildDockerRunEnv([]string{"BASE=1"}, dockerRunConfig{
		WrapperAgentID:        "codex-agent",
		WrapperTaskID:         "codex-session",
		SessionToken:          "sess_123",
		BrokerUnixSocket:      "/Users/test/.promptlock/run/promptlock.sock",
		ContainerBrokerSocket: "/run/promptlock/promptlock.sock",
	})
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"BASE=1",
		"PROMPTLOCK_SESSION_TOKEN=sess_123",
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN=sess_123",
		"PROMPTLOCK_WRAPPER_AGENT_ID=codex-agent",
		"PROMPTLOCK_WRAPPER_TASK_ID=codex-session",
		"PROMPTLOCK_AGENT_UNIX_SOCKET=/run/promptlock/promptlock.sock",
		"PROMPTLOCK_WRAPPER_AGENT_UNIX_SOCKET=/run/promptlock/promptlock.sock",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected docker env to contain %q, got %q", want, joined)
		}
	}
}

func TestBuildDockerRunArgsRequiresImageAndBroker(t *testing.T) {
	if _, err := buildDockerRunArgs(dockerRunConfig{
		ContainerName: "codex-1",
		SessionToken:  "sess_123",
	}); err == nil {
		t.Fatalf("expected missing image/broker configuration to fail")
	}
}

func TestBuildDockerRunArgsRejectsReservedEnvOverrides(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:            "codex-promptlock",
		ContainerName:    "codex-1",
		SessionToken:     "sess_123",
		BrokerUnixSocket: "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalEnv:    []string{"PROMPTLOCK_SESSION_TOKEN=override"},
	})
	if err == nil {
		t.Fatalf("expected reserved env override to fail")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected reserved env error, got %v", err)
	}
	if !strings.Contains(err.Error(), "different variable name") {
		t.Fatalf("expected actionable env override guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsRawDockerEnvFlags(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"--env=PROMPTLOCK_SESSION_TOKEN=override"},
	})
	if err == nil {
		t.Fatalf("expected raw docker env flag to fail")
	}
	if !strings.Contains(err.Error(), "docker-arg") {
		t.Fatalf("expected docker-arg guidance, got %v", err)
	}
	if !strings.Contains(err.Error(), "--env KEY=VALUE") {
		t.Fatalf("expected safe alternative guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsSplitDockerEnvFlags(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"--env", "PROMPTLOCK_SESSION_TOKEN=override"},
	})
	if err == nil {
		t.Fatalf("expected split docker env flag to fail")
	}
	if !strings.Contains(err.Error(), "environment variables") {
		t.Fatalf("expected env guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsSplitDockerMountFlags(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                 "codex-promptlock",
		ContainerName:         "codex-1",
		SessionToken:          "sess_123",
		BrokerUnixSocket:      "/Users/test/.promptlock/run/promptlock.sock",
		ContainerBrokerSocket: "/run/promptlock/promptlock.sock",
		AdditionalDockerArgs:  []string{"--mount", "type=bind,src=/tmp/other.sock,dst=/run/promptlock/promptlock.sock"},
	})
	if err == nil {
		t.Fatalf("expected split docker mount flag to fail")
	}
	if !strings.Contains(err.Error(), "add mounts") {
		t.Fatalf("expected mount guidance, got %v", err)
	}
	if !strings.Contains(err.Error(), "protected container broker socket") {
		t.Fatalf("expected protected socket guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsShortVolumeFlag(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"-v", "/tmp/host:/run/promptlock/promptlock-agent.sock"},
	})
	if err == nil {
		t.Fatalf("expected short volume flag to fail")
	}
	if !strings.Contains(err.Error(), "use --mount") {
		t.Fatalf("expected mount replacement guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsShortWorkdirFlag(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"-w", "/workspace"},
	})
	if err == nil {
		t.Fatalf("expected short workdir flag to fail")
	}
	if !strings.Contains(err.Error(), "wrapper's --workdir flag") {
		t.Fatalf("expected workdir replacement guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsShortUserFlag(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"-u", "0:0"},
	})
	if err == nil {
		t.Fatalf("expected short user flag to fail")
	}
	if !strings.Contains(err.Error(), "PromptLock manages the container user") {
		t.Fatalf("expected managed user guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsAllowsAllowlistedSplitDockerFlags(t *testing.T) {
	args, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"--pull", "always", "--init", "--label", "com.example.role=test"},
	})
	if err != nil {
		t.Fatalf("expected allowlisted docker args to pass, got %v", err)
	}
	joined := strings.Join(args, "\n")
	for _, want := range []string{"--pull", "always", "--init", "--label", "com.example.role=test"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected docker args to contain %q, got %q", want, joined)
		}
	}
}

func TestBuildDockerRunArgsAllowsInlineAllowlistedFlags(t *testing.T) {
	args, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"--pull=always", "--label=com.example.role=test"},
	})
	if err != nil {
		t.Fatalf("expected inline allowlisted docker args to pass, got %v", err)
	}
	joined := strings.Join(args, "\n")
	for _, want := range []string{"--pull=always", "--label=com.example.role=test"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected docker args to contain %q, got %q", want, joined)
		}
	}
}

func TestBuildDockerRunArgsRejectsNonAllowlistedDockerFlags(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"--network", "host"},
	})
	if err == nil {
		t.Fatalf("expected non-allowlisted docker flag to fail")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected allowlist guidance, got %v", err)
	}
	if !strings.Contains(err.Error(), "allowed docker-arg flags") {
		t.Fatalf("expected allowlist details, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsHostAliasRedirectFlagsForBrokerURL(t *testing.T) {
	for _, dockerArgs := range [][]string{
		{"--add-host", "host.docker.internal:10.0.0.2"},
		{"--dns", "10.0.0.2"},
		{"--dns-option", "ndots:1"},
		{"--dns-search", "corp.local"},
	} {
		_, err := buildDockerRunArgs(dockerRunConfig{
			Image:                "codex-promptlock",
			ContainerName:        "codex-1",
			SessionToken:         "sess_123",
			BrokerURL:            "http://host.docker.internal:58879",
			AdditionalDockerArgs: dockerArgs,
		})
		if err == nil {
			t.Fatalf("expected host-alias redirect flags %v to fail", dockerArgs)
		}
		if !strings.Contains(err.Error(), "host.docker.internal") {
			t.Fatalf("expected host-alias guidance, got %v", err)
		}
	}
}

func TestBuildDockerRunArgsRejectsAllowlistedFlagWithoutValue(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                "codex-promptlock",
		ContainerName:        "codex-1",
		SessionToken:         "sess_123",
		BrokerUnixSocket:     "/Users/test/.promptlock/run/promptlock.sock",
		AdditionalDockerArgs: []string{"--pull"},
	})
	if err == nil {
		t.Fatalf("expected missing docker arg value to fail")
	}
	if !strings.Contains(err.Error(), "missing value") {
		t.Fatalf("expected missing value guidance, got %v", err)
	}
	if !strings.Contains(err.Error(), "allowed docker-arg flags") {
		t.Fatalf("expected allowlist reminder, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsMountOverAgentSocket(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                 "codex-promptlock",
		ContainerName:         "codex-1",
		SessionToken:          "sess_123",
		BrokerUnixSocket:      "/Users/test/.promptlock/run/promptlock.sock",
		ContainerBrokerSocket: "/run/promptlock/promptlock.sock",
		AdditionalMounts:      []string{"type=bind,src=/tmp/other.sock,dst=/run/promptlock/promptlock.sock"},
	})
	if err == nil {
		t.Fatalf("expected mount over agent socket to fail")
	}
	if !strings.Contains(err.Error(), "container broker socket") {
		t.Fatalf("expected socket shadowing guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsIncludesWrapperMCPEnvFileMount(t *testing.T) {
	args, err := buildDockerRunArgs(dockerRunConfig{
		Image:               "promptlock-agent-lab",
		ContainerName:       "codex-1",
		SessionToken:        "sess_123",
		BrokerURL:           "http://host.docker.internal:58879",
		WrapperMCPEnvFile:   "/tmp/promptlock-mcp.env.host",
		ContainerMCPEnvFile: "/run/promptlock/promptlock-mcp.env",
	})
	if err != nil {
		t.Fatalf("buildDockerRunArgs returned error: %v", err)
	}
	joined := strings.Join(args, "\n")
	want := "type=bind,src=/tmp/promptlock-mcp.env.host,dst=/run/promptlock/promptlock-mcp.env,readonly"
	if !strings.Contains(joined, want) {
		t.Fatalf("expected docker args to contain wrapper MCP env file mount %q, got %q", want, joined)
	}
}

func TestPrepareDockerRunHiddenMountsMasksWorkspaceFile(t *testing.T) {
	hostRoot := t.TempDir()
	hostSecretDir := filepath.Join(hostRoot, "demo-envs")
	if err := os.MkdirAll(hostSecretDir, 0o755); err != nil {
		t.Fatalf("mkdir demo env dir: %v", err)
	}
	hostSecret := filepath.Join(hostSecretDir, "github.env")
	if err := os.WriteFile(hostSecret, []byte("github_token=FAKE_GITHUB_TOKEN\n"), 0o600); err != nil {
		t.Fatalf("write demo env file: %v", err)
	}

	cfg, cleanup, err := prepareDockerRunHiddenMounts(dockerRunConfig{
		Workdir:          "/workspace",
		AdditionalMounts: []string{"type=bind,src=" + hostRoot + ",dst=/workspace"},
		HiddenPaths:      []string{"demo-envs/github.env"},
	})
	if err != nil {
		t.Fatalf("prepareDockerRunHiddenMounts returned error: %v", err)
	}
	defer cleanup()

	if len(cfg.HiddenMounts) != 1 {
		t.Fatalf("expected 1 hidden mount, got %d", len(cfg.HiddenMounts))
	}
	masked := cfg.HiddenMounts[0]
	if masked.Destination != "/workspace/demo-envs/github.env" {
		t.Fatalf("destination = %q, want /workspace/demo-envs/github.env", masked.Destination)
	}
	if masked.Source == hostSecret {
		t.Fatalf("expected mask source to differ from host secret path %q", hostSecret)
	}
	if !masked.ReadOnly {
		t.Fatalf("expected hidden mount to be readonly")
	}
	info, err := os.Stat(masked.Source)
	if err != nil {
		t.Fatalf("stat hidden mount source: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("expected file placeholder, got directory %q", masked.Source)
	}
	body, err := os.ReadFile(masked.Source)
	if err != nil {
		t.Fatalf("read hidden mount source: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("expected empty placeholder file, got %q", string(body))
	}
}

func TestPrepareDockerRunHiddenMountsMasksWorkspaceDirectory(t *testing.T) {
	hostRoot := t.TempDir()
	hostSecretDir := filepath.Join(hostRoot, "secrets")
	if err := os.MkdirAll(filepath.Join(hostSecretDir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir secret dir: %v", err)
	}

	cfg, cleanup, err := prepareDockerRunHiddenMounts(dockerRunConfig{
		Workdir:          "/workspace",
		AdditionalMounts: []string{"type=bind,src=" + hostRoot + ",dst=/workspace"},
		HiddenPaths:      []string{"secrets"},
	})
	if err != nil {
		t.Fatalf("prepareDockerRunHiddenMounts returned error: %v", err)
	}
	defer cleanup()

	if len(cfg.HiddenMounts) != 1 {
		t.Fatalf("expected 1 hidden mount, got %d", len(cfg.HiddenMounts))
	}
	info, err := os.Stat(cfg.HiddenMounts[0].Source)
	if err != nil {
		t.Fatalf("stat hidden mount source: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory placeholder, got file %q", cfg.HiddenMounts[0].Source)
	}
}

func TestPrepareDockerRunHiddenMountsRejectsUncoveredPath(t *testing.T) {
	_, cleanup, err := prepareDockerRunHiddenMounts(dockerRunConfig{
		Workdir:          "/workspace",
		AdditionalMounts: []string{"type=bind,src=/host/workspace,dst=/workspace"},
		HiddenPaths:      []string{"/other/secret.env"},
	})
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatalf("expected uncovered hidden path to fail")
	}
	if !strings.Contains(err.Error(), "not covered by any bind mount") {
		t.Fatalf("expected bind mount guidance, got %v", err)
	}
}

func TestPrepareDockerRunHiddenMountsRejectsProtectedPromptLockPaths(t *testing.T) {
	_, cleanup, err := prepareDockerRunHiddenMounts(dockerRunConfig{
		Workdir:             "/workspace",
		ContainerMCPEnvFile: "/run/promptlock/promptlock-mcp.env",
		AdditionalMounts:    []string{"type=bind,src=/host/workspace,dst=/workspace"},
		HiddenPaths:         []string{"/run/promptlock"},
	})
	if cleanup != nil {
		defer cleanup()
	}
	if err == nil {
		t.Fatalf("expected protected PromptLock target to fail")
	}
	if !strings.Contains(err.Error(), "reserved PromptLock path") {
		t.Fatalf("expected reserved path guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsIncludesHiddenMountsAfterWorkspaceMount(t *testing.T) {
	args, err := buildDockerRunArgs(dockerRunConfig{
		Image:         "promptlock-agent-lab",
		ContainerName: "codex-1",
		SessionToken:  "sess_123",
		BrokerURL:     "http://host.docker.internal:58879",
		AdditionalMounts: []string{
			"type=bind,src=/host/workspace,dst=/workspace",
		},
		HiddenMounts: []dockerHiddenMount{
			{Source: "/tmp/promptlock-mask-1", Destination: "/workspace/demo-envs/github.env", ReadOnly: true},
		},
	})
	if err != nil {
		t.Fatalf("buildDockerRunArgs returned error: %v", err)
	}
	joined := strings.Join(args, "\n")
	workspaceMount := "type=bind,src=/host/workspace,dst=/workspace"
	hiddenMount := "type=bind,src=/tmp/promptlock-mask-1,dst=/workspace/demo-envs/github.env,readonly"
	if !strings.Contains(joined, workspaceMount) {
		t.Fatalf("expected docker args to contain workspace mount %q, got %q", workspaceMount, joined)
	}
	if !strings.Contains(joined, hiddenMount) {
		t.Fatalf("expected docker args to contain hidden mount %q, got %q", hiddenMount, joined)
	}
	if strings.Index(joined, workspaceMount) > strings.Index(joined, hiddenMount) {
		t.Fatalf("expected hidden mount to be appended after workspace mount, got %q", joined)
	}
}

func TestBuildDockerRunArgsRejectsMountOverWrapperMCPEnvFile(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:               "promptlock-agent-lab",
		ContainerName:       "codex-1",
		SessionToken:        "sess_123",
		BrokerURL:           "http://host.docker.internal:58879",
		ContainerMCPEnvFile: "/run/promptlock/promptlock-mcp.env",
		AdditionalMounts:    []string{"type=bind,src=/tmp/other.env,dst=/run/promptlock/promptlock-mcp.env"},
	})
	if err == nil {
		t.Fatalf("expected mount over wrapper MCP env file to fail")
	}
	if !strings.Contains(err.Error(), "mcp env file") {
		t.Fatalf("expected wrapper MCP env file guidance, got %v", err)
	}
}

func TestBuildDockerRunArgsRejectsMountOverPromptLockRuntimeDir(t *testing.T) {
	_, err := buildDockerRunArgs(dockerRunConfig{
		Image:                 "promptlock-agent-lab",
		ContainerName:         "codex-1",
		SessionToken:          "sess_123",
		BrokerUnixSocket:      "/Users/test/.promptlock/run/promptlock.sock",
		ContainerBrokerSocket: "/run/promptlock/promptlock-agent.sock",
		ContainerMCPEnvFile:   "/run/promptlock/promptlock-mcp.env",
		AdditionalMounts:      []string{"type=bind,src=/tmp/other,dst=/run/promptlock"},
	})
	if err == nil {
		t.Fatalf("expected mount over PromptLock runtime dir to fail")
	}
	if !strings.Contains(err.Error(), "reserved PromptLock path") {
		t.Fatalf("expected reserved PromptLock path guidance, got %v", err)
	}
}

func TestDockerRunPostLaunchGuidanceForInteractiveCodex(t *testing.T) {
	got := dockerRunPostLaunchGuidance(dockerRunConfig{
		Image:            "promptlock-agent-lab",
		AttachTTY:        true,
		AdditionalMounts: []string{"type=bind,src=/Users/test/.codex,dst=/home/promptlock/.codex"},
	})
	for _, want := range []string{
		"PromptLock wrapper note for Codex:",
		"promptlock mcp doctor",
		"codex mcp add promptlock -- promptlock-mcp-launch",
		"codex -C /workspace --no-alt-screen",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected guidance to contain %q, got %q", want, got)
		}
	}
}

func TestDockerRunPostLaunchGuidanceSkippedForNonInteractiveOrNonCodex(t *testing.T) {
	for _, cfg := range []dockerRunConfig{
		{
			Image:     "promptlock-agent-lab",
			AttachTTY: false,
			Command:   []string{"codex"},
		},
		{
			Image:     "promptlock-agent-lab",
			AttachTTY: true,
			Command:   []string{"go", "version"},
		},
	} {
		if got := dockerRunPostLaunchGuidance(cfg); got != "" {
			t.Fatalf("expected no guidance for %+v, got %q", cfg, got)
		}
	}
}
