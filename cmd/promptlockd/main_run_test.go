package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunFailsWhenEnvPathRootIsInvalid(t *testing.T) {
	t.Setenv("PROMPTLOCK_ALLOW_DEV_PROFILE", "1")
	t.Setenv("PROMPTLOCK_CONFIG", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", filepath.Join(t.TempDir(), "does-not-exist"))

	err := run()
	if err == nil {
		t.Fatalf("expected startup to fail when env-path root is invalid")
	}
	if !strings.Contains(err.Error(), "env-path secret source root") {
		t.Fatalf("expected env-path startup error, got %v", err)
	}
}
