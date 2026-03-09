package main

import (
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestValidateSecurityProfile(t *testing.T) {
	mk := func(profile string, auth bool) config.Config {
		return config.Config{SecurityProfile: profile, Auth: config.AuthConfig{EnableAuth: auth}}
	}

	if err := validateSecurityProfile(mk("dev", false), ""); err != nil {
		t.Fatalf("dev profile should be allowed: %v", err)
	}
	if err := validateSecurityProfile(mk("hardened", false), ""); err == nil {
		t.Fatalf("hardened profile without auth should fail")
	}
	if err := validateSecurityProfile(mk("hardened", true), ""); err != nil {
		t.Fatalf("hardened with auth should pass: %v", err)
	}
	if err := validateSecurityProfile(mk("insecure", true), ""); err == nil {
		t.Fatalf("insecure profile without explicit opt-in should fail")
	}
	if err := validateSecurityProfile(mk("insecure", true), "1"); err != nil {
		t.Fatalf("insecure profile with explicit opt-in should pass: %v", err)
	}
}

func TestIsInsecureDevMode(t *testing.T) {
	cfg := config.Default()
	if !isInsecureDevMode(cfg) {
		t.Fatalf("expected default config to be flagged as insecure dev mode")
	}

	cfg.Auth.EnableAuth = true
	if isInsecureDevMode(cfg) {
		t.Fatalf("expected auth-enabled config not to be insecure dev mode")
	}

	cfg.Auth.EnableAuth = false
	cfg.Auth.AllowPlaintextSecretReturn = false
	if isInsecureDevMode(cfg) {
		t.Fatalf("expected plaintext-disabled config not to be insecure dev mode")
	}
}

func TestValidateDeploymentMode(t *testing.T) {
	devCfg := config.Config{SecurityProfile: "dev", Auth: config.AuthConfig{EnableAuth: false}}
	if err := validateDeploymentMode(devCfg, ""); err == nil {
		t.Fatalf("expected dev mode without explicit opt-in to fail")
	}
	if err := validateDeploymentMode(devCfg, "1"); err != nil {
		t.Fatalf("expected explicit dev-mode opt-in to pass: %v", err)
	}

	hardenedCfg := config.Config{
		SecurityProfile: "hardened",
		StateStoreFile:  "/tmp/state-store.json",
		SecretSource: config.SecretSourceConfig{
			Type: "env",
		},
		Auth: config.AuthConfig{
			EnableAuth:            true,
			StoreFile:             "/tmp/auth-store.json",
			StoreEncryptionKeyEnv: "PROMPTLOCK_AUTH_STORE_KEY",
		},
	}
	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "")
	if err := validateDeploymentMode(hardenedCfg, ""); err == nil {
		t.Fatalf("expected hardened auth store without encryption key to fail")
	}
	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "0123456789abcdef")
	if err := validateDeploymentMode(hardenedCfg, ""); err != nil {
		t.Fatalf("expected hardened auth store with encryption key to pass: %v", err)
	}

	hardenedCfg.StateStoreFile = ""
	if err := validateDeploymentMode(hardenedCfg, ""); err == nil {
		t.Fatalf("expected missing state_store_file to fail")
	}
	hardenedCfg.StateStoreFile = "/tmp/state-store.json"
	hardenedCfg.SecretSource.Type = "in_memory"
	if err := validateDeploymentMode(hardenedCfg, ""); err == nil {
		t.Fatalf("expected in_memory secret source in non-dev to fail")
	}
}

func TestResolveAuthStoreEncryptionKey(t *testing.T) {
	cfg := config.Config{Auth: config.AuthConfig{StoreEncryptionKeyEnv: "PROMPTLOCK_AUTH_STORE_KEY"}}
	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "")
	key, err := resolveAuthStoreEncryptionKey(cfg)
	if err != nil {
		t.Fatalf("unexpected empty-key error: %v", err)
	}
	if len(key) != 0 {
		t.Fatalf("expected nil/empty key when env var is unset")
	}

	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "short")
	if _, err := resolveAuthStoreEncryptionKey(cfg); err == nil {
		t.Fatalf("expected short key to fail")
	}

	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "0123456789abcdef")
	key, err = resolveAuthStoreEncryptionKey(cfg)
	if err != nil {
		t.Fatalf("expected valid key to pass: %v", err)
	}
	if len(key) == 0 {
		t.Fatalf("expected non-empty key bytes")
	}
}
