package main

import (
	"fmt"
	"testing"

	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/sopsenv"
)

func TestLoadStartupSOPSEnvNoPathNoop(t *testing.T) {
	t.Setenv(sopsenv.DefaultEnvFileEnv, "")
	orig := loadPromptlockSOPSEnvFile
	t.Cleanup(func() { loadPromptlockSOPSEnvFile = orig })
	called := false
	loadPromptlockSOPSEnvFile = func(_ string, _ []string) error {
		called = true
		return nil
	}

	if err := loadStartupSOPSEnv(config.Default()); err != nil {
		t.Fatalf("load startup sops env: %v", err)
	}
	if called {
		t.Fatalf("expected loader not to be called when path is unset")
	}
}

func TestLoadStartupSOPSEnvNonDevRequiresAuthStoreKey(t *testing.T) {
	t.Setenv(sopsenv.DefaultEnvFileEnv, "/etc/promptlock/runtime.sops.env")
	orig := loadPromptlockSOPSEnvFile
	t.Cleanup(func() { loadPromptlockSOPSEnvFile = orig })

	cfg := config.Default()
	cfg.SecurityProfile = "hardened"
	cfg.Auth.EnableAuth = true
	cfg.Auth.StoreFile = "/var/lib/promptlock/auth-store.json"
	cfg.Auth.StoreEncryptionKeyEnv = "PROMPTLOCK_AUTH_STORE_KEY"

	loadPromptlockSOPSEnvFile = func(path string, required []string) error {
		if path != "/etc/promptlock/runtime.sops.env" {
			t.Fatalf("unexpected sops path: %q", path)
		}
		if len(required) != 1 || required[0] != "PROMPTLOCK_AUTH_STORE_KEY" {
			t.Fatalf("unexpected required keys: %#v", required)
		}
		return nil
	}

	if err := loadStartupSOPSEnv(cfg); err != nil {
		t.Fatalf("load startup sops env: %v", err)
	}
}

func TestLoadStartupSOPSEnvUsesDefaultAuthStoreKeyEnvName(t *testing.T) {
	t.Setenv(sopsenv.DefaultEnvFileEnv, "/etc/promptlock/runtime.sops.env")
	orig := loadPromptlockSOPSEnvFile
	t.Cleanup(func() { loadPromptlockSOPSEnvFile = orig })

	cfg := config.Default()
	cfg.SecurityProfile = "hardened"
	cfg.Auth.EnableAuth = true
	cfg.Auth.StoreFile = "/var/lib/promptlock/auth-store.json"
	cfg.Auth.StoreEncryptionKeyEnv = ""

	loadPromptlockSOPSEnvFile = func(_ string, required []string) error {
		if len(required) != 1 || required[0] != "PROMPTLOCK_AUTH_STORE_KEY" {
			t.Fatalf("unexpected required keys: %#v", required)
		}
		return nil
	}

	if err := loadStartupSOPSEnv(cfg); err != nil {
		t.Fatalf("load startup sops env: %v", err)
	}
}

func TestLoadStartupSOPSEnvPropagatesLoaderError(t *testing.T) {
	t.Setenv(sopsenv.DefaultEnvFileEnv, "/etc/promptlock/runtime.sops.env")
	orig := loadPromptlockSOPSEnvFile
	t.Cleanup(func() { loadPromptlockSOPSEnvFile = orig })

	cfg := config.Default()
	loadPromptlockSOPSEnvFile = func(_ string, _ []string) error {
		return fmt.Errorf("decrypt failed")
	}

	if err := loadStartupSOPSEnv(cfg); err == nil {
		t.Fatalf("expected loader error")
	}
}
