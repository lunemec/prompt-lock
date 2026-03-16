package main

import (
	"strings"
	"testing"
)

func TestBuildDockerRunArgsWithUnixSocket(t *testing.T) {
	args, err := buildDockerRunArgs(dockerRunConfig{
		Image:                 "codex-promptlock",
		ContainerName:         "codex-1",
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
		"PROMPTLOCK_AGENT_UNIX_SOCKET=/run/promptlock/promptlock.sock",
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
		Image:         "codex-promptlock",
		ContainerName: "codex-1",
		SessionToken:  "sess_123",
		BrokerURL:     "https://promptlock.example.internal:8765",
		User:          "1000:1000",
	})
	if err != nil {
		t.Fatalf("buildDockerRunArgs returned error: %v", err)
	}

	joined := strings.Join(args, "\n")
	checks := []string{
		"PROMPTLOCK_SESSION_TOKEN",
		"PROMPTLOCK_BROKER_URL",
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
