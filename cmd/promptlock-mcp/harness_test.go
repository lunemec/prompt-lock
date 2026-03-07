package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
