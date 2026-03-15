package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndValidateExecArgs(t *testing.T) {
	ok, err := parseAndValidateExecArgs(map[string]interface{}{
		"intent":      "run_tests",
		"command":     []interface{}{"bash", "-lc", "echo ok"},
		"ttl_minutes": float64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok.Intent != "run_tests" || len(ok.Cmd) != 3 || ok.TTL != 5 {
		t.Fatalf("unexpected parsed args: %#v", ok)
	}

	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "", "command": []interface{}{"bash"}}); err == nil {
		t.Fatalf("expected invalid intent")
	}
	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "run", "command": []interface{}{}, "ttl_minutes": float64(5)}); err == nil {
		t.Fatalf("expected invalid command")
	}
	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "run", "command": []interface{}{"bash", float64(1)}, "ttl_minutes": float64(5)}); err == nil {
		t.Fatalf("expected invalid non-string command argument")
	}
	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "run", "command": []interface{}{"bash"}, "ttl_minutes": float64(999)}); err == nil {
		t.Fatalf("expected invalid ttl")
	}
	if _, err := parseAndValidateExecArgs(map[string]interface{}{"intent": "run", "command": []interface{}{"bash"}, "ttl_minutes": float64(1.5)}); err == nil {
		t.Fatalf("expected invalid non-integer ttl")
	}
}

func TestResolveBrokerTransportPrefersAgentUnixSocket(t *testing.T) {
	transport, err := resolveBrokerTransport(lookupEnvMap(map[string]string{
		"PROMPTLOCK_AGENT_UNIX_SOCKET": "/tmp/promptlock-agent.sock",
	}), func(path string) error {
		if path == "/tmp/promptlock-agent.sock" {
			return nil
		}
		return errors.New("unexpected path")
	})
	if err != nil {
		t.Fatalf("resolveBrokerTransport returned error: %v", err)
	}
	if transport.UnixSocket != "/tmp/promptlock-agent.sock" {
		t.Fatalf("unix socket = %q, want agent socket", transport.UnixSocket)
	}
	if transport.BaseURL != "" {
		t.Fatalf("expected unix-socket transport to avoid implicit TCP base URL, got %q", transport.BaseURL)
	}
}

func TestResolveBrokerTransportFailsClosedWithoutSocketOrExplicitBroker(t *testing.T) {
	_, err := resolveBrokerTransport(lookupEnvMap(nil), func(string) error {
		return errors.New("missing")
	})
	if err == nil {
		t.Fatalf("expected missing broker transport to fail closed")
	}
}

func TestExecuteWithIntentFailsClosedWithoutExplicitBrokerTransport(t *testing.T) {
	t.Setenv("PROMPTLOCK_SESSION_TOKEN", "sess_1")
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", filepath.Join(t.TempDir(), "missing-agent.sock"))
	t.Setenv("PROMPTLOCK_BROKER_URL", "")

	_, err := executeWithIntent(context.Background(), map[string]interface{}{
		"intent":  "run_tests",
		"command": []interface{}{"go", "version"},
	})
	if err == nil {
		t.Fatalf("expected missing broker transport to fail closed")
	}
	if !strings.Contains(err.Error(), "broker unix socket not found") {
		t.Fatalf("expected fail-closed missing socket error, got %v", err)
	}
}
