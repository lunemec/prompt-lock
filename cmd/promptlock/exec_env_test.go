package main

import "testing"

func TestBuildLocalExecutionEnvStripsAmbientSecrets(t *testing.T) {
	t.Setenv("PATH", "/usr/local/bin:/usr/bin")
	t.Setenv("HOME", "/tmp/local-home")
	t.Setenv("TMPDIR", "/tmp/local-tmp")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "ambient-secret")
	t.Setenv("GIT_ASKPASS", "/tmp/askpass")

	env := buildLocalExecutionEnv(map[string]string{
		"github_token": "leased-github-token",
	})

	assertEnvEntry(t, env, "PATH=/usr/local/bin:/usr/bin")
	assertEnvEntry(t, env, "HOME=/tmp/local-home")
	assertEnvEntry(t, env, "TMPDIR=/tmp/local-tmp")
	assertEnvEntry(t, env, "GITHUB_TOKEN=leased-github-token")
	assertNoEnvKey(t, env, "AWS_SECRET_ACCESS_KEY")
	assertNoEnvKey(t, env, "GIT_ASKPASS")
}

func assertEnvEntry(t *testing.T, env []string, want string) {
	t.Helper()
	for _, entry := range env {
		if entry == want {
			return
		}
	}
	t.Fatalf("expected env %q in %#v", want, env)
}

func assertNoEnvKey(t *testing.T, env []string, key string) {
	t.Helper()
	prefix := key + "="
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			t.Fatalf("did not expect env key %q in %#v", key, env)
		}
	}
}
