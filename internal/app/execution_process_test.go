package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lunemec/promptlock/internal/config"
)

func TestValidateExecuteCommandRequiresExactExecutableIdentity(t *testing.T) {
	policy := DefaultControlPlanePolicy{
		Exec: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"go", "git", "make"},
			DenylistSubstrings:    []string{"&&"},
		},
	}

	for _, cmd := range [][]string{
		{"go", "version"},
		{"/usr/local/bin/go", "version"},
		{"/usr/bin/git", "status"},
		{"make", "test"},
	} {
		if err := policy.ValidateExecuteCommand(cmd); err != nil {
			t.Fatalf("expected command %q to be allowed: %v", strings.Join(cmd, " "), err)
		}
	}

	for _, cmd := range [][]string{
		{"goevil", "version"},
		{"git-backdoor", "status"},
		{"/tmp/goevil", "version"},
	} {
		if err := policy.ValidateExecuteCommand(cmd); err == nil {
			t.Fatalf("expected command %q to be rejected", strings.Join(cmd, " "))
		}
	}
}

func TestResolveExecuteCommandRejectsPathOutsideControlledSearchPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script executable helper is unix-specific")
	}

	trustedDir := t.TempDir()
	untrustedDir := t.TempDir()
	writeTestExecutable(t, filepath.Join(untrustedDir, "go"), "echo untrusted")

	policy := DefaultControlPlanePolicy{
		Exec: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"go"},
			CommandSearchPaths:    []string{trustedDir},
		},
	}

	if _, err := policy.ResolveExecuteCommand([]string{filepath.Join(untrustedDir, "go"), "version"}); err == nil {
		t.Fatalf("expected path outside controlled search path to be rejected")
	}
}

func TestBuildExecutionEnvironmentStripsAmbientSecrets(t *testing.T) {
	env := BuildExecutionEnvironment([]string{
		"PATH=/usr/local/bin:/usr/bin",
		"HOME=/tmp/promptlock-home",
		"TMPDIR=/tmp/promptlock-tmp",
		"AWS_SECRET_ACCESS_KEY=ambient-secret",
		"GIT_ASKPASS=/tmp/askpass",
		"LANG=en_US.UTF-8",
	}, map[string]string{
		"github_token": "leased-github-token",
		"npm_token":    "leased-npm-token",
	})

	assertEnvContains(t, env, "PATH=/usr/local/bin:/usr/bin")
	assertEnvContains(t, env, "HOME=/tmp/promptlock-home")
	assertEnvContains(t, env, "TMPDIR=/tmp/promptlock-tmp")
	assertEnvContains(t, env, "GITHUB_TOKEN=leased-github-token")
	assertEnvContains(t, env, "NPM_TOKEN=leased-npm-token")
	assertEnvOmitsPrefix(t, env, "AWS_SECRET_ACCESS_KEY=")
	assertEnvOmitsPrefix(t, env, "GIT_ASKPASS=")
	assertEnvOmitsPrefix(t, env, "LANG=")
}

func assertEnvContains(t *testing.T, env []string, want string) {
	t.Helper()
	for _, entry := range env {
		if entry == want {
			return
		}
	}
	t.Fatalf("expected env %q in %#v", want, env)
}

func assertEnvOmitsPrefix(t *testing.T, env []string, prefix string) {
	t.Helper()
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			t.Fatalf("did not expect env prefix %q in %#v", prefix, env)
		}
	}
}

func writeTestExecutable(t *testing.T, path string, body string) {
	t.Helper()
	script := "#!/bin/sh\nset -eu\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}
