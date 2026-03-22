package showcase

import (
	"os"
	"strings"
	"testing"
)

func TestPromptLockEnvShowcaseToken(t *testing.T) {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		if !demoRequireEnv() {
			t.Skip("GITHUB_TOKEN not set; run via make demo-run-env-showcase-tests or PromptLock MCP demo flow")
		}
		t.Fatalf("GITHUB_TOKEN is required for demo showcase")
	}
	if strings.ContainsAny(token, "\r\n") {
		t.Fatalf("GITHUB_TOKEN must be single-line")
	}
	if expected := strings.TrimSpace(os.Getenv("PROMPTLOCK_DEMO_EXPECTED_TOKEN")); expected != "" && token != expected {
		t.Fatalf("GITHUB_TOKEN mismatch: got %q, want %q", token, expected)
	}
	t.Logf("PromptLock showcase: leased GITHUB_TOKEN is present (len=%d)", len(token))
}

func TestPromptLockEnvShowcaseMetadata(t *testing.T) {
	mode := strings.TrimSpace(os.Getenv("PROMPTLOCK_DEMO_MODE"))
	actor := strings.TrimSpace(os.Getenv("PROMPTLOCK_DEMO_ACTOR"))
	if mode == "" || actor == "" {
		if !demoRequireEnv() {
			t.Skip("PROMPTLOCK_DEMO_MODE / PROMPTLOCK_DEMO_ACTOR not set; set by make demo-run-env-showcase-tests")
		}
		t.Fatalf("PROMPTLOCK_DEMO_MODE and PROMPTLOCK_DEMO_ACTOR are required for demo showcase")
	}
	if mode != "env-showcase" {
		t.Fatalf("PROMPTLOCK_DEMO_MODE=%q, want env-showcase", mode)
	}
	t.Logf("PromptLock showcase: metadata mode=%s actor=%s", mode, actor)
}

func demoRequireEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROMPTLOCK_DEMO_REQUIRE"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
