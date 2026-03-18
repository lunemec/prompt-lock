package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestApplyEnvOverridesStateStoreExternalFields(t *testing.T) {
	cfg := config.Default()
	t.Setenv("PROMPTLOCK_AUDIT_PATH", "/tmp/audit.jsonl")
	t.Setenv("PROMPTLOCK_ADDR", "0.0.0.0:9999")
	t.Setenv("PROMPTLOCK_UNIX_SOCKET", "/tmp/promptlock.sock")
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", "/tmp/promptlock-agent.sock")
	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", "/tmp/promptlock-operator.sock")
	t.Setenv("PROMPTLOCK_AGENT_BRIDGE_ADDRESS", "127.0.0.1:8766")
	t.Setenv("PROMPTLOCK_STATE_STORE_FILE", "/tmp/state-store.json")
	t.Setenv("PROMPTLOCK_OPERATOR_TOKEN", "op-token")
	t.Setenv("PROMPTLOCK_STATE_STORE_TYPE", "external")
	t.Setenv("PROMPTLOCK_STATE_STORE_EXTERNAL_URL", "https://state.example.internal")
	t.Setenv("PROMPTLOCK_STATE_STORE_EXTERNAL_AUTH_TOKEN_ENV", "PROMPTLOCK_EXTERNAL_STATE_TOKEN")
	t.Setenv("PROMPTLOCK_STATE_STORE_EXTERNAL_TIMEOUT_SEC", "42")

	if err := applyEnvOverrides(&cfg); err != nil {
		t.Fatalf("apply env overrides: %v", err)
	}

	if cfg.AuditPath != "/tmp/audit.jsonl" {
		t.Fatalf("unexpected audit path: %q", cfg.AuditPath)
	}
	if cfg.Address != "0.0.0.0:9999" {
		t.Fatalf("unexpected address: %q", cfg.Address)
	}
	if cfg.UnixSocket != "/tmp/promptlock.sock" {
		t.Fatalf("unexpected unix socket: %q", cfg.UnixSocket)
	}
	if cfg.AgentUnixSocket != "/tmp/promptlock-agent.sock" {
		t.Fatalf("unexpected agent unix socket: %q", cfg.AgentUnixSocket)
	}
	if cfg.OperatorUnixSocket != "/tmp/promptlock-operator.sock" {
		t.Fatalf("unexpected operator unix socket: %q", cfg.OperatorUnixSocket)
	}
	if cfg.AgentBridgeAddress != "127.0.0.1:8766" {
		t.Fatalf("unexpected agent bridge address: %q", cfg.AgentBridgeAddress)
	}
	if cfg.StateStoreFile != "/tmp/state-store.json" {
		t.Fatalf("unexpected state_store_file: %q", cfg.StateStoreFile)
	}
	if cfg.Auth.OperatorToken != "op-token" {
		t.Fatalf("unexpected operator token: %q", cfg.Auth.OperatorToken)
	}
	if cfg.StateStore.Type != "external" {
		t.Fatalf("unexpected state_store.type: %q", cfg.StateStore.Type)
	}
	if cfg.StateStore.ExternalURL != "https://state.example.internal" {
		t.Fatalf("unexpected state_store.external_url: %q", cfg.StateStore.ExternalURL)
	}
	if cfg.StateStore.ExternalAuthTokenEnv != "PROMPTLOCK_EXTERNAL_STATE_TOKEN" {
		t.Fatalf("unexpected state_store.external_auth_token_env: %q", cfg.StateStore.ExternalAuthTokenEnv)
	}
	if cfg.StateStore.ExternalTimeoutSec != 42 {
		t.Fatalf("unexpected state_store.external_timeout_sec: %d", cfg.StateStore.ExternalTimeoutSec)
	}
}

func TestApplyEnvOverridesInvalidStateStoreTimeout(t *testing.T) {
	cfg := config.Default()
	t.Setenv("PROMPTLOCK_STATE_STORE_EXTERNAL_TIMEOUT_SEC", "not-a-number")

	if err := applyEnvOverrides(&cfg); err == nil {
		t.Fatalf("expected invalid timeout error")
	}
}

func TestApplyEnvOverridesNilConfig(t *testing.T) {
	if err := applyEnvOverrides(nil); err == nil {
		t.Fatalf("expected nil config error")
	}
}
