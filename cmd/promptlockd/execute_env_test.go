package main

import (
	"os"
	"testing"
)

func TestBuildBrokerExecutionEnvStripsAmbientSecrets(t *testing.T) {
	t.Setenv("PATH", "/usr/local/bin:/usr/bin")
	t.Setenv("HOME", "/tmp/broker-home")
	t.Setenv("TMPDIR", "/tmp/broker-tmp")
	t.Setenv("PROMPTLOCK_AUTH_STORE_KEY", "ambient-key")
	t.Setenv("OPENAI_API_KEY", "ambient-openai-key")

	env := buildBrokerExecutionEnv(os.Environ(), map[string]string{
		"github_token": "leased-github-token",
	}, "/trusted/bin:/usr/bin")

	assertBrokerEnvEntry(t, env, "PATH=/trusted/bin:/usr/bin")
	assertBrokerEnvEntry(t, env, "HOME=/tmp/broker-home")
	assertBrokerEnvEntry(t, env, "TMPDIR=/tmp/broker-tmp")
	assertBrokerEnvEntry(t, env, "GITHUB_TOKEN=leased-github-token")
	assertBrokerNoEnvKey(t, env, "PROMPTLOCK_AUTH_STORE_KEY")
	assertBrokerNoEnvKey(t, env, "OPENAI_API_KEY")
}

func assertBrokerEnvEntry(t *testing.T, env []string, want string) {
	t.Helper()
	for _, entry := range env {
		if entry == want {
			return
		}
	}
	t.Fatalf("expected env %q in %#v", want, env)
}

func assertBrokerNoEnvKey(t *testing.T, env []string, key string) {
	t.Helper()
	prefix := key + "="
	for _, entry := range env {
		if len(entry) >= len(prefix) && entry[:len(prefix)] == prefix {
			t.Fatalf("did not expect env key %q in %#v", key, env)
		}
	}
}
