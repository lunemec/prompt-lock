package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizedLaunchEnvPrefersWrapperValues(t *testing.T) {
	env, err := normalizedLaunchEnv([]string{
		"PROMPTLOCK_SESSION_TOKEN=stale-session",
		"PROMPTLOCK_BROKER_URL=http://stale.example.internal",
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN=fresh-session",
		"PROMPTLOCK_WRAPPER_BROKER_URL=http://fresh.example.internal",
		"PROMPTLOCK_WRAPPER_AGENT_ID=codex-agent",
		"PROMPTLOCK_WRAPPER_TASK_ID=codex-task",
	})
	if err != nil {
		t.Fatalf("normalizedLaunchEnv returned error: %v", err)
	}
	values := envToMap(env)
	if got := values["PROMPTLOCK_SESSION_TOKEN"]; got != "fresh-session" {
		t.Fatalf("PROMPTLOCK_SESSION_TOKEN = %q, want fresh wrapper value", got)
	}
	if got := values["PROMPTLOCK_BROKER_URL"]; got != "http://fresh.example.internal" {
		t.Fatalf("PROMPTLOCK_BROKER_URL = %q, want fresh wrapper value", got)
	}
	if got := values["PROMPTLOCK_AGENT_ID"]; got != "codex-agent" {
		t.Fatalf("PROMPTLOCK_AGENT_ID = %q, want wrapper agent id", got)
	}
	if got := values["PROMPTLOCK_TASK_ID"]; got != "codex-task" {
		t.Fatalf("PROMPTLOCK_TASK_ID = %q, want wrapper task id", got)
	}
}

func TestNormalizedLaunchEnvRequiresSession(t *testing.T) {
	_, err := normalizedLaunchEnv([]string{"PROMPTLOCK_BROKER_URL=http://broker.example.internal"})
	if err == nil {
		t.Fatalf("expected missing session token to fail")
	}
}

func TestNormalizedLaunchEnvFallsBackToWrapperEnvFile(t *testing.T) {
	t.Setenv("PROMPTLOCK_MCP_ENV_FILE", "")
	path := t.TempDir() + "/promptlock-mcp.env"
	if err := os.WriteFile(path, []byte(strings.Join([]string{
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN=file-session",
		"PROMPTLOCK_WRAPPER_BROKER_URL=http://file.example.internal:8765",
		"PROMPTLOCK_WRAPPER_AGENT_ID=file-agent",
		"PROMPTLOCK_WRAPPER_TASK_ID=file-task",
	}, "\n")+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	env, err := normalizedLaunchEnv([]string{"PROMPTLOCK_MCP_ENV_FILE=" + path})
	if err != nil {
		t.Fatalf("normalizedLaunchEnv returned error: %v", err)
	}
	values := envToMap(env)
	if got := values["PROMPTLOCK_SESSION_TOKEN"]; got != "file-session" {
		t.Fatalf("PROMPTLOCK_SESSION_TOKEN = %q, want file fallback", got)
	}
	if got := values["PROMPTLOCK_BROKER_URL"]; got != "http://file.example.internal:8765" {
		t.Fatalf("PROMPTLOCK_BROKER_URL = %q, want file fallback", got)
	}
	if got := values["PROMPTLOCK_AGENT_ID"]; got != "file-agent" {
		t.Fatalf("PROMPTLOCK_AGENT_ID = %q, want wrapper file agent id", got)
	}
	if got := values["PROMPTLOCK_TASK_ID"]; got != "file-task" {
		t.Fatalf("PROMPTLOCK_TASK_ID = %q, want wrapper file task id", got)
	}
}

func TestResolvePromptlockMCPBinaryPrefersSibling(t *testing.T) {
	target, err := resolvePromptlockMCPBinary(
		func() (string, error) { return "/tmp/bin/promptlock-mcp-launch", nil },
		func(string) (string, error) {
			t.Fatalf("lookPath should not be called when sibling exists")
			return "", nil
		},
		func(path string) bool { return path == "/tmp/bin/promptlock-mcp" },
	)
	if err != nil {
		t.Fatalf("resolvePromptlockMCPBinary returned error: %v", err)
	}
	if target != "/tmp/bin/promptlock-mcp" {
		t.Fatalf("target = %q, want sibling promptlock-mcp", target)
	}
}

func TestResolvePromptlockMCPBinaryFallsBackToPath(t *testing.T) {
	target, err := resolvePromptlockMCPBinary(
		func() (string, error) { return "/tmp/bin/promptlock-mcp-launch", nil },
		func(name string) (string, error) {
			if name != "promptlock-mcp" {
				t.Fatalf("lookPath name = %q, want promptlock-mcp", name)
			}
			return "/opt/homebrew/bin/promptlock-mcp", nil
		},
		func(string) bool { return false },
	)
	if err != nil {
		t.Fatalf("resolvePromptlockMCPBinary returned error: %v", err)
	}
	if target != "/opt/homebrew/bin/promptlock-mcp" {
		t.Fatalf("target = %q, want PATH fallback", target)
	}
}

func TestEnvListRoundTrip(t *testing.T) {
	values, order := envMapWithOrder([]string{"A=1", "B=2", "A=3"})
	got := envListFromOrder(values, order)
	want := []string{"A=3", "B=2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("envListFromOrder = %#v, want %#v", got, want)
	}
}

func TestWrapperEnvFileParsesCommentsAndRejectsMalformedLines(t *testing.T) {
	parsed, err := parseWrapperEnvFile([]byte("# comment\nPROMPTLOCK_SESSION_TOKEN=sess\n\nPROMPTLOCK_BROKER_URL=http://broker\n"))
	if err != nil {
		t.Fatalf("parseWrapperEnvFile returned error: %v", err)
	}
	if got := parsed["PROMPTLOCK_SESSION_TOKEN"]; got != "sess" {
		t.Fatalf("PROMPTLOCK_SESSION_TOKEN = %q, want sess", got)
	}
	if _, err := parseWrapperEnvFile([]byte("not-an-env-line\n")); err == nil {
		t.Fatalf("expected malformed env file line to fail")
	}
}

func TestDefaultWrapperEnvFilePathIsStable(t *testing.T) {
	if got, want := defaultWrapperMCPEnvFilePath(), "/run/promptlock/promptlock-mcp.env"; got != want {
		t.Fatalf("defaultWrapperMCPEnvFilePath = %q, want %q", got, want)
	}
}

func envToMap(items []string) map[string]string {
	values := map[string]string{}
	for _, entry := range items {
		name, value, ok := cutEnv(entry)
		if !ok {
			continue
		}
		values[name] = value
	}
	return values
}

func cutEnv(entry string) (string, string, bool) {
	for i := 0; i < len(entry); i++ {
		if entry[i] == '=' {
			return entry[:i], entry[i+1:], entry[:i] != ""
		}
	}
	return "", "", false
}
