package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type rpcMsg map[string]any

func launchMCP(t *testing.T, extraEnv map[string]string) (*exec.Cmd, func(string), func() rpcMsg) {
	t.Helper()
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	cmd.Env = os.Environ()
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(stdout)
	writeLine := func(s string) { _, _ = stdin.Write([]byte(s + "\n")) }
	readJSON := func() rpcMsg {
		done := make(chan rpcMsg, 1)
		errCh := make(chan error, 1)
		go func() {
			line, err := r.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
			line = strings.TrimSpace(line)
			var m rpcMsg
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				errCh <- err
				return
			}
			done <- m
		}()
		select {
		case m := <-done:
			return m
		case err := <-errCh:
			t.Fatal(err)
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for MCP response")
		}
		return nil
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return cmd, writeLine, readJSON
}

func TestMCPStdioInitializeAndToolsList(t *testing.T) {
	_, writeLine, readJSON := launchMCP(t, map[string]string{})

	writeLine(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	msg1 := readJSON()
	if msg1["error"] != nil {
		t.Fatalf("initialize returned error: %+v", msg1)
	}
	if msg1["result"] == nil {
		t.Fatalf("initialize missing result: %+v", msg1)
	}

	writeLine(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	msg2 := readJSON()
	if msg2["error"] != nil {
		t.Fatalf("tools/list returned error: %+v", msg2)
	}
	res, ok := msg2["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list result shape invalid: %+v", msg2)
	}
	tools, ok := res["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools/list no tools: %+v", msg2)
	}
}

func TestMCPToolsCallExecuteWithIntentRoundtrip(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case r.URL.Path == "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r1"})
		case r.URL.Path == "/v1/requests/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved"})
		case r.URL.Path == "/v1/leases/by-request":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease_token": "l1"})
		case r.URL.Path == "/v1/leases/execute":
			_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": 0, "stdout_stderr": "ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":    broker.URL,
		"PROMPTLOCK_SESSION_TOKEN": "s1",
	})

	writeLine(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] != nil {
		t.Fatalf("tools/call returned error: %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("invalid result: %+v", msg)
	}
	content, ok := res["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("missing content: %+v", msg)
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid content shape: %+v", msg)
	}
	text, _ := item["text"].(string)
	if !strings.Contains(text, "exit=0") {
		t.Fatalf("expected exit=0 in text, got: %s", text)
	}
}

func TestMCPToolsCallRealBrokerE2E(t *testing.T) {
	addr := freeAddr(t)
	cfgPath := filepath.Join(t.TempDir(), "promptlock-config.json")
	cfg := map[string]any{
		"address":    addr,
		"audit_path": filepath.Join(t.TempDir(), "audit.jsonl"),
		"auth": map[string]any{
			"enable_auth": false,
		},
		"execution_policy": map[string]any{
			"allowlist_prefixes":  []string{"bash"},
			"denylist_substrings": []string{"printenv"},
			"max_output_bytes":    65536,
			"default_timeout_sec": 30,
			"max_timeout_sec":     60,
		},
		"secrets": []map[string]string{{"name": "github_token", "value": "E2E_OK"}},
		"intents": map[string][]string{"run_tests": {"github_token"}},
	}
	b, _ := json.Marshal(cfg)
	if err := os.WriteFile(cfgPath, b, 0o600); err != nil {
		t.Fatal(err)
	}

	brokerCmd := exec.Command("go", "run", "./cmd/promptlockd")
	brokerCmd.Dir = "../.."
	brokerCmd.Env = append(os.Environ(), "PROMPTLOCK_CONFIG="+cfgPath, "PROMPTLOCK_ALLOW_DEV_PROFILE=1")
	if err := brokerCmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = brokerCmd.Process.Kill() })
	waitBroker(t, "http://"+addr)

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":    "http://" + addr,
		"PROMPTLOCK_SESSION_TOKEN": "dummy",
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			resp, err := http.Get("http://" + addr + "/v1/requests/pending")
			if err == nil {
				var p struct {
					Pending []struct {
						ID string `json:"ID"`
					} `json:"pending"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&p)
				_ = resp.Body.Close()
				if len(p.Pending) > 0 {
					_, _ = http.Post("http://"+addr+"/v1/leases/approve?request_id="+p.Pending[0].ID, "application/json", bytes.NewBufferString(`{"ttl_minutes":5}`))
					return
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	writeLine(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo -n $GITHUB_TOKEN"],"ttl_minutes":5}}}`)
	msg := readJSON()
	<-done
	if msg["error"] != nil {
		t.Fatalf("tools/call E2E returned error: %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("invalid result: %+v", msg)
	}
	content, ok := res["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("missing content: %+v", msg)
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid content shape: %+v", msg)
	}
	text, _ := item["text"].(string)
	if !strings.Contains(text, "exit=0") || !strings.Contains(text, "E2E_OK") {
		t.Fatalf("unexpected text: %s", text)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func TestMCPToolsCallDeniedPath(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-deny"})
		case "/v1/requests/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "denied"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{"PROMPTLOCK_BROKER_URL": broker.URL, "PROMPTLOCK_SESSION_TOKEN": "s1"})
	writeLine(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] == nil {
		t.Fatalf("expected denied error, got %+v", msg)
	}
}

func TestMCPToolsCallTimeoutPath(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-wait"})
		case "/v1/requests/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":           broker.URL,
		"PROMPTLOCK_SESSION_TOKEN":        "s1",
		"PROMPTLOCK_APPROVAL_TIMEOUT_SEC": "1",
	})
	writeLine(`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] == nil {
		t.Fatalf("expected timeout error, got %+v", msg)
	}
}

func TestMCPToolsCallMissingSessionToken(t *testing.T) {
	_, writeLine, readJSON := launchMCP(t, map[string]string{})
	writeLine(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] == nil {
		t.Fatalf("expected missing session token error")
	}
}

func TestMCPMalformedJSONReturnsParseError(t *testing.T) {
	_, writeLine, readJSON := launchMCP(t, map[string]string{})
	writeLine(`{"jsonrpc":"2.0","id":1,"method"`)
	msg := readJSON()
	if msg["error"] == nil {
		t.Fatalf("expected parse error")
	}
}

func TestMCPUnknownToolError(t *testing.T) {
	_, writeLine, readJSON := launchMCP(t, map[string]string{"PROMPTLOCK_SESSION_TOKEN": "s1"})
	writeLine(`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"unknown_tool","arguments":{}}}`)
	msg := readJSON()
	if msg["error"] == nil {
		t.Fatalf("expected unknown tool error")
	}
}

func TestMCPInvalidArgsValidationError(t *testing.T) {
	_, writeLine, readJSON := launchMCP(t, map[string]string{"PROMPTLOCK_SESSION_TOKEN": "s1"})
	writeLine(`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"bad intent !","command":["bash"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] == nil {
		t.Fatalf("expected validation error")
	}
}

func TestMCPToolsCallInvalidSessionToken(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid session", http.StatusUnauthorized)
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":    broker.URL,
		"PROMPTLOCK_SESSION_TOKEN": "bad-token",
	})
	writeLine(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] == nil {
		t.Fatalf("expected invalid session token error")
	}
}

func TestMCPToolsCallPolicyDeniedPath(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-policy"})
		case "/v1/requests/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved"})
		case "/v1/leases/by-request":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease_token": "l-policy"})
		case "/v1/leases/execute":
			http.Error(w, "command denied by policy", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":    broker.URL,
		"PROMPTLOCK_SESSION_TOKEN": "s1",
	})
	writeLine(`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] == nil {
		t.Fatalf("expected policy denied error")
	}
}

func waitBroker(t *testing.T, base string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/v1/meta/capabilities")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("broker did not become ready: %s", base)
}
