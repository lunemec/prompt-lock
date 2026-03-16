package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestNewEnvPathSecretStoreUsesCwdFallbackOnlyInDev(t *testing.T) {
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", "")
	root := t.TempDir()
	store, resolvedRoot, err := newEnvPathSecretStore(config.Config{SecurityProfile: "dev"}, func() (string, error) {
		return root, nil
	})
	if err != nil {
		t.Fatalf("newEnvPathSecretStore returned error: %v", err)
	}
	if store == nil {
		t.Fatalf("expected env-path store in dev profile")
	}
	if resolvedRoot != root {
		t.Fatalf("resolved root = %q, want %q", resolvedRoot, root)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("github_token=test\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	canonical, err := store.Canonicalize(".env")
	if err != nil {
		t.Fatalf("expected dev fallback store to canonicalize test env file: %v", err)
	}
	if !strings.HasSuffix(canonical, ".env") {
		t.Fatalf("canonical path = %q, want suffix .env", canonical)
	}
}

func TestNewEnvPathSecretStoreDisablesFallbackOutsideDev(t *testing.T) {
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", "")
	store, resolvedRoot, err := newEnvPathSecretStore(config.Config{SecurityProfile: "hardened"}, func() (string, error) {
		return t.TempDir(), nil
	})
	if err != nil {
		t.Fatalf("newEnvPathSecretStore returned error: %v", err)
	}
	if resolvedRoot != "" {
		t.Fatalf("resolved root = %q, want empty", resolvedRoot)
	}
	if _, err := store.Canonicalize(".env"); err == nil || !strings.Contains(err.Error(), "PROMPTLOCK_ENV_PATH_ROOT") {
		t.Fatalf("expected explicit root requirement, got %v", err)
	}
}
