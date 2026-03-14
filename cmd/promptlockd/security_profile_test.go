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

	externalCfg := config.Config{
		SecurityProfile: "hardened",
		StateStore: config.StateStoreConfig{
			Type:                 "external",
			ExternalURL:          "https://state.example.internal",
			ExternalAuthTokenEnv: "PROMPTLOCK_EXTERNAL_STATE_TOKEN",
		},
		SecretSource: config.SecretSourceConfig{
			Type: "env",
		},
		Auth: config.AuthConfig{
			EnableAuth:            true,
			StoreFile:             "/tmp/auth-store.json",
			StoreEncryptionKeyEnv: "PROMPTLOCK_AUTH_STORE_KEY",
		},
	}
	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "0123456789abcdef")
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")
	if err := validateDeploymentMode(externalCfg, ""); err != nil {
		t.Fatalf("expected hardened external state backend to pass: %v", err)
	}

	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "")
	if err := validateDeploymentMode(externalCfg, ""); err == nil {
		t.Fatalf("expected external state backend without token env value to fail")
	}
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")

	externalCfg.StateStore.ExternalURL = "http://state.example.internal"
	if err := validateDeploymentMode(externalCfg, ""); err == nil {
		t.Fatalf("expected non-https external state backend in non-dev profile to fail")
	}

	externalCfg.StateStore.ExternalURL = "https://state.example.internal"
	externalCfg.StateStore.ExternalAuthTokenEnv = ""
	if err := validateDeploymentMode(externalCfg, ""); err == nil {
		t.Fatalf("expected missing external auth token env name to fail")
	}

	externalSecretCfg := config.Config{
		SecurityProfile: "hardened",
		StateStoreFile:  "/tmp/state-store.json",
		SecretSource: config.SecretSourceConfig{
			Type:                 "external",
			ExternalURL:          "https://secrets.example.internal",
			ExternalAuthTokenEnv: "PROMPTLOCK_EXTERNAL_SECRET_TOKEN",
		},
		Auth: config.AuthConfig{
			EnableAuth:            true,
			StoreFile:             "/tmp/auth-store.json",
			StoreEncryptionKeyEnv: "PROMPTLOCK_AUTH_STORE_KEY",
		},
	}
	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "0123456789abcdef")
	t.Setenv("PROMPTLOCK_EXTERNAL_SECRET_TOKEN", "secret-token")
	if err := validateDeploymentMode(externalSecretCfg, ""); err != nil {
		t.Fatalf("expected hardened external secret backend to pass: %v", err)
	}

	t.Setenv("PROMPTLOCK_EXTERNAL_SECRET_TOKEN", "")
	if err := validateDeploymentMode(externalSecretCfg, ""); err == nil {
		t.Fatalf("expected external secret backend without token env value to fail")
	}
	t.Setenv("PROMPTLOCK_EXTERNAL_SECRET_TOKEN", "secret-token")

	externalSecretCfg.SecretSource.ExternalURL = "http://secrets.example.internal"
	if err := validateDeploymentMode(externalSecretCfg, ""); err == nil {
		t.Fatalf("expected non-https external secret backend in non-dev profile to fail")
	}

	externalSecretCfg.SecretSource.ExternalURL = "https://secrets.example.internal"
	externalSecretCfg.SecretSource.ExternalAuthTokenEnv = ""
	if err := validateDeploymentMode(externalSecretCfg, ""); err == nil {
		t.Fatalf("expected missing external secret auth token env name to fail")
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

func TestValidateStateStoreSafety(t *testing.T) {
	cfg := config.Config{}
	if err := validateStateStoreSafety(cfg); err != nil {
		t.Fatalf("default state store safety should pass: %v", err)
	}

	cfg.StateStore.Type = "external"
	if err := validateStateStoreSafety(cfg); err == nil {
		t.Fatalf("expected missing external_url to fail")
	}

	cfg.StateStore.ExternalURL = "https://state.example.internal"
	if err := validateStateStoreSafety(cfg); err == nil {
		t.Fatalf("expected missing external_auth_token_env to fail")
	}

	cfg.StateStore.ExternalAuthTokenEnv = "PROMPTLOCK_EXTERNAL_STATE_TOKEN"
	if err := validateStateStoreSafety(cfg); err != nil {
		t.Fatalf("expected valid external state store config to pass: %v", err)
	}

	cfg.StateStore.ExternalURL = "://bad-url"
	if err := validateStateStoreSafety(cfg); err == nil {
		t.Fatalf("expected invalid URL to fail")
	}

	cfg.StateStore.Type = "postgres"
	if err := validateStateStoreSafety(cfg); err == nil {
		t.Fatalf("expected unsupported type to fail")
	}
}
