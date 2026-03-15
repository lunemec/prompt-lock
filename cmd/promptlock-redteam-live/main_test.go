package main

import (
	"path/filepath"
	"testing"
)

func TestConfigureHarnessTransportDevUsesLocalHTTP(t *testing.T) {
	cfg := map[string]any{
		"tls": map[string]any{"enable": true},
	}

	got, err := configureHarnessTransport("dev", t.TempDir(), 9123, cfg)
	if err != nil {
		t.Fatalf("configure transport: %v", err)
	}
	if got.operatorBase != "http://127.0.0.1:9123" {
		t.Fatalf("unexpected operator base: %q", got.operatorBase)
	}
	if got.agentBase != "http://127.0.0.1:9123" {
		t.Fatalf("unexpected agent base: %q", got.agentBase)
	}
	if got.operatorClient == nil || got.agentClient == nil {
		t.Fatalf("expected non-nil HTTP clients")
	}
	if _, exists := cfg["tls"]; exists {
		t.Fatalf("expected removed tls config to be stripped")
	}
}

func TestConfigureHarnessTransportHardenedUsesDualSockets(t *testing.T) {
	tempDir := t.TempDir()
	cfg := map[string]any{
		"tls": map[string]any{"enable": true},
	}

	got, err := configureHarnessTransport("hardened", tempDir, 9123, cfg)
	if err != nil {
		t.Fatalf("configure transport: %v", err)
	}
	if got.operatorBase != "http://unix" {
		t.Fatalf("unexpected operator base: %q", got.operatorBase)
	}
	if got.agentBase != "http://unix" {
		t.Fatalf("unexpected agent base: %q", got.agentBase)
	}
	if got.operatorClient == nil || got.agentClient == nil {
		t.Fatalf("expected unix-socket clients")
	}
	if gotSocket, ok := cfg["agent_unix_socket"].(string); !ok || gotSocket != filepath.Join(tempDir, "promptlock-agent.sock") {
		t.Fatalf("unexpected agent socket config: %#v", cfg["agent_unix_socket"])
	}
	if gotSocket, ok := cfg["operator_unix_socket"].(string); !ok || gotSocket != filepath.Join(tempDir, "promptlock-operator.sock") {
		t.Fatalf("unexpected operator socket config: %#v", cfg["operator_unix_socket"])
	}
	if _, exists := cfg["tls"]; exists {
		t.Fatalf("expected removed tls config to be stripped")
	}
}
