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
