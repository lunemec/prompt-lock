package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
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
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tools/list tool shape invalid: %+v", msg2)
	}
	schema, ok := tool["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list inputSchema missing: %+v", msg2)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list properties missing: %+v", msg2)
	}
	envPath, ok := props["env_path"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list missing env_path property: %+v", msg2)
	}
	if got, _ := envPath["type"].(string); got != "string" {
		t.Fatalf("expected env_path type=string, got %+v", envPath)
	}
}

func TestMCPToolsListIncludesIntentUsageGuidance(t *testing.T) {
	_, writeLine, readJSON := launchMCP(t, map[string]string{})

	writeLine(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	initResp := readJSON()
	if initResp["error"] != nil {
		t.Fatalf("initialize returned error: %+v", initResp)
	}

	writeLine(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	msg := readJSON()
	if msg["error"] != nil {
		t.Fatalf("tools/list returned error: %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list result shape invalid: %+v", msg)
	}
	tools, ok := res["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("tools/list no tools: %+v", msg)
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("tools/list tool shape invalid: %+v", tools[0])
	}
	toolDescription, _ := tool["description"].(string)
	if !strings.Contains(toolDescription, "configured intent id") {
		t.Fatalf("expected tool description to explain configured intent ids, got %q", toolDescription)
	}
	if !strings.Contains(toolDescription, "run_tests") {
		t.Fatalf("expected tool description to include run_tests quickstart hint, got %q", toolDescription)
	}
	schema, ok := tool["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list inputSchema missing: %+v", tool)
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list properties missing: %+v", schema)
	}
	intentProp, ok := props["intent"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list missing intent property: %+v", props)
	}
	intentDescription, _ := intentProp["description"].(string)
	if !strings.Contains(intentDescription, "must exactly match") {
		t.Fatalf("expected intent description to require exact match ids, got %q", intentDescription)
	}
}

func TestMCPInitializeNotificationAndToolsCallSequence(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case r.URL.Path == "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-seq"})
		case r.URL.Path == "/v1/requests/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved"})
		case r.URL.Path == "/v1/leases/by-request":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease_token": "l-seq"})
		case r.URL.Path == "/v1/leases/execute":
			_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": 0, "stdout_stderr": "sequence-ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":    broker.URL,
		"PROMPTLOCK_SESSION_TOKEN": "s1",
	})

	writeLine(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	initResp := readJSON()
	if initResp["error"] != nil {
		t.Fatalf("initialize returned error: %+v", initResp)
	}

	// notification must not emit a response that would get ahead of the next request response.
	writeLine(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`)

	writeLine(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	listResp := readJSON()
	if got, ok := listResp["id"].(float64); !ok || int(got) != 2 {
		t.Fatalf("expected tools/list response id=2 (notification should be silent), got %+v", listResp)
	}
	if listResp["error"] != nil {
		t.Fatalf("tools/list returned error: %+v", listResp)
	}

	writeLine(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	callResp := readJSON()
	if got, ok := callResp["id"].(float64); !ok || int(got) != 3 {
		t.Fatalf("expected tools/call response id=3, got %+v", callResp)
	}
	if callResp["error"] != nil {
		t.Fatalf("tools/call returned error: %+v", callResp)
	}
	res, ok := callResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("invalid tools/call result: %+v", callResp)
	}
	content, ok := res["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("missing tools/call content: %+v", callResp)
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("invalid tools/call content shape: %+v", callResp)
	}
	text, _ := item["text"].(string)
	if !strings.Contains(text, "exit=0") || !strings.Contains(text, "sequence-ok") {
		t.Fatalf("unexpected tools/call text: %q", text)
	}
}

func TestMCPToolsCallExecuteWithIntentRoundtrip(t *testing.T) {
	var leaseRequest struct {
		EnvPath string `json:"env_path"`
	}
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case r.URL.Path == "/v1/leases/request":
			if err := json.NewDecoder(r.Body).Decode(&leaseRequest); err != nil {
				t.Fatalf("decode lease request: %v", err)
			}
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

	writeLine(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5,"env_path":"demo-envs/github.env"}}}`)
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
	if leaseRequest.EnvPath != "demo-envs/github.env" {
		t.Fatalf("expected lease request env_path to roundtrip, got %#v", leaseRequest)
	}
}

func TestMCPToolsCallUsesAgentUnixSocketByDefault(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-mcp-tools-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-unix"})
		case "/v1/requests/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved"})
		case "/v1/leases/by-request":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease_token": "l-unix"})
		case "/v1/leases/execute":
			_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": 0, "stdout_stderr": "unix-ok"})
		default:
			http.NotFound(w, r)
		}
	})}
	defer srv.Close()
	go func() { _ = srv.Serve(ln) }()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":        "",
		"PROMPTLOCK_AGENT_UNIX_SOCKET": socketPath,
		"PROMPTLOCK_SESSION_TOKEN":     "s1",
	})

	writeLine(`{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] != nil {
		t.Fatalf("tools/call over unix socket returned error: %+v", msg)
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
	if !strings.Contains(text, "unix-ok") {
		t.Fatalf("expected unix socket roundtrip output, got %q", text)
	}
}

func TestMCPToolsCallRejectsObjectRequestID(t *testing.T) {
	_, writeLine, readJSON := launchMCP(t, map[string]string{"PROMPTLOCK_SESSION_TOKEN": "s1"})

	writeLine(`{"jsonrpc":"2.0","id":{"bad":1},"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32600 {
		t.Fatalf("expected -32600 for invalid tools/call request id, got %+v", errObj)
	}
	if id, ok := msg["id"]; !ok || id != nil {
		t.Fatalf("expected id=null for invalid tools/call request id, got %+v", msg)
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
			"exact_match_executables": []string{"bash"},
			"denylist_substrings":     []string{"printenv"},
			"max_output_bytes":        65536,
			"default_timeout_sec":     30,
			"max_timeout_sec":         60,
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

func TestMCPExecuteWithIntentUsesAgentUnixSocketByDefault(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-mcp-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer listener.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-unix"})
		case "/v1/requests/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved"})
		case "/v1/leases/by-request":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease_token": "l-unix"})
		case "/v1/leases/execute":
			_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": 0, "stdout_stderr": "unix-ok"})
		default:
			http.NotFound(w, r)
		}
	})}
	defer srv.Close()
	go func() { _ = srv.Serve(listener) }()

	t.Setenv("PROMPTLOCK_BROKER_URL", "")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", socketPath)
	t.Setenv("PROMPTLOCK_SESSION_TOKEN", "s1")

	out, err := executeWithIntent(context.Background(), map[string]interface{}{
		"intent":      "run_tests",
		"command":     []interface{}{"echo", "ok"},
		"ttl_minutes": float64(5),
	})
	if err != nil {
		t.Fatalf("executeWithIntent: %v", err)
	}
	if !strings.Contains(out, "unix-ok") {
		t.Fatalf("expected unix-socket transport output, got %q", out)
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

func TestMCPToolsCallCancelledByNotification(t *testing.T) {
	var cancelCalls atomic.Int64
	var statusCalls atomic.Int64
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-cancel"})
		case "/v1/requests/status":
			statusCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
		case "/v1/leases/cancel":
			cancelCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-cancel", "status": "denied"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":           broker.URL,
		"PROMPTLOCK_SESSION_TOKEN":        "s1",
		"PROMPTLOCK_APPROVAL_TIMEOUT_SEC": "30",
	})

	writeLine(`{"jsonrpc":"2.0","id":31,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	waitAtomicAtLeast(t, &statusCalls, 1, 5*time.Second, "approval polling status call before cancellation")
	writeLine(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":31}}`)

	msg := readJSON()
	if got, ok := msg["id"].(float64); !ok || int(got) != 31 {
		t.Fatalf("expected cancelled response id=31, got %+v", msg)
	}
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected cancellation error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32000 {
		t.Fatalf("expected -32000 cancellation error code, got %+v", errObj)
	}
	msgText, _ := errObj["message"].(string)
	if !strings.Contains(strings.ToLower(msgText), "cancel") {
		t.Fatalf("expected cancellation message, got %q", msgText)
	}
	if cancelCalls.Load() < 1 {
		t.Fatalf("expected broker cancel endpoint call, got %d", cancelCalls.Load())
	}
}

func TestMCPToolsCallCancelledByNotificationStringID(t *testing.T) {
	var cancelCalls atomic.Int64
	var statusCalls atomic.Int64
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-cancel-string"})
		case "/v1/requests/status":
			statusCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
		case "/v1/leases/cancel":
			cancelCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-cancel-string", "status": "denied"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":           broker.URL,
		"PROMPTLOCK_SESSION_TOKEN":        "s1",
		"PROMPTLOCK_APPROVAL_TIMEOUT_SEC": "30",
	})

	writeLine(`{"jsonrpc":"2.0","id":"req-31","method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	waitAtomicAtLeast(t, &statusCalls, 1, 5*time.Second, "approval polling status call before cancellation")
	writeLine(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"req-31"}}`)

	msg := readJSON()
	if got, ok := msg["id"].(string); !ok || got != "req-31" {
		t.Fatalf("expected cancelled response id=req-31, got %+v", msg)
	}
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected cancellation error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32000 {
		t.Fatalf("expected -32000 cancellation error code, got %+v", errObj)
	}
	msgText, _ := errObj["message"].(string)
	if !strings.Contains(strings.ToLower(msgText), "cancel") {
		t.Fatalf("expected cancellation message, got %q", msgText)
	}
	if cancelCalls.Load() < 1 {
		t.Fatalf("expected broker cancel endpoint call, got %d", cancelCalls.Load())
	}
}

func TestMCPToolsCallCancelledCleanupFailureWarns(t *testing.T) {
	var cancelCalls atomic.Int64
	var statusCalls atomic.Int64
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-cancel-warn"})
		case "/v1/requests/status":
			statusCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "pending"})
		case "/v1/leases/cancel":
			cancelCalls.Add(1)
			http.Error(w, "state backend unavailable", http.StatusServiceUnavailable)
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	cmd.Env = append(os.Environ(),
		"PROMPTLOCK_BROKER_URL="+broker.URL,
		"PROMPTLOCK_SESSION_TOKEN=s1",
		"PROMPTLOCK_APPROVAL_TIMEOUT_SEC=30",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	stderrLines := make(chan string, 32)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrLines <- scanner.Text()
		}
		close(stderrLines)
	}()

	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","id":41,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}` + "\n"))
	waitAtomicAtLeast(t, &statusCalls, 1, 5*time.Second, "approval polling status call before cancellation")
	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":41}}` + "\n"))

	respReader := bufio.NewReader(stdout)
	readDone := make(chan rpcMsg, 1)
	readErr := make(chan error, 1)
	go func() {
		line, err := respReader.ReadString('\n')
		if err != nil {
			readErr <- err
			return
		}
		var msg rpcMsg
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &msg); err != nil {
			readErr <- err
			return
		}
		readDone <- msg
	}()

	var msg rpcMsg
	select {
	case msg = <-readDone:
	case err := <-readErr:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for cancellation response")
	}
	if got, ok := msg["id"].(float64); !ok || int(got) != 41 {
		t.Fatalf("expected cancellation response id=41, got %+v", msg)
	}
	if msg["error"] == nil {
		t.Fatalf("expected cancellation error payload, got %+v", msg)
	}
	if cancelCalls.Load() < 1 {
		t.Fatalf("expected broker cancel endpoint call, got %d", cancelCalls.Load())
	}

	warnFound := false
	timeout := time.After(2 * time.Second)
	for !warnFound {
		select {
		case line, ok := <-stderrLines:
			if !ok {
				if !warnFound {
					t.Fatal("expected cleanup warning on stderr, got channel closed")
				}
				return
			}
			if strings.Contains(line, "failed to cancel pending request") && strings.Contains(line, "r-cancel-warn") {
				warnFound = true
			}
		case <-timeout:
			t.Fatal("timeout waiting for cleanup warning on stderr")
		}
	}
}

func TestMCPToolsCallNotificationIgnoredNoSideEffects(t *testing.T) {
	var backendCalls atomic.Int64
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalls.Add(1)
		http.NotFound(w, r)
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":    broker.URL,
		"PROMPTLOCK_SESSION_TOKEN": "s1",
	})

	writeLine(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	writeLine(`{"jsonrpc":"2.0","id":77,"method":"ping","params":{}}`)

	msg := readJSON()
	if got, ok := msg["id"].(float64); !ok || int(got) != 77 {
		t.Fatalf("expected ping response id=77 after tools/call notification, got %+v", msg)
	}
	if backendCalls.Load() != 0 {
		t.Fatalf("expected tools/call notification to be ignored, backend calls=%d", backendCalls.Load())
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

func TestMCPToolsCallPrefersWrapperSessionAndTransportOverStaleSavedEnv(t *testing.T) {
	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer fresh-session" {
			http.Error(w, "invalid session", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/intents/resolve":
			_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
		case "/v1/leases/request":
			_ = json.NewEncoder(w).Encode(map[string]any{"request_id": "r-wrapper"})
		case "/v1/requests/status":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "approved"})
		case "/v1/leases/by-request":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease_token": "l-wrapper"})
		case "/v1/leases/execute":
			_ = json.NewEncoder(w).Encode(map[string]any{"exit_code": 0, "stdout_stderr": "wrapper-env-ok"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer broker.Close()

	_, writeLine, readJSON := launchMCP(t, map[string]string{
		"PROMPTLOCK_BROKER_URL":            "http://127.0.0.1:1",
		"PROMPTLOCK_SESSION_TOKEN":         "stale-session",
		"PROMPTLOCK_WRAPPER_BROKER_URL":    broker.URL,
		"PROMPTLOCK_WRAPPER_SESSION_TOKEN": "fresh-session",
	})
	writeLine(`{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["bash","-lc","echo ok"],"ttl_minutes":5}}}`)
	msg := readJSON()
	if msg["error"] != nil {
		t.Fatalf("expected wrapper env to beat stale saved env, got error %+v", msg)
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
	if !strings.Contains(text, "wrapper-env-ok") {
		t.Fatalf("expected wrapper-backed execution output, got %q", text)
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
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured rpc error, got %+v", msg)
	}
	message, _ := errObj["message"].(string)
	if !strings.Contains(message, "command denied by policy") {
		t.Fatalf("expected policy body in MCP error message, got %q", message)
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

func waitAtomicAtLeast(t *testing.T, counter *atomic.Int64, min int64, timeout time.Duration, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if counter.Load() >= min {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}
