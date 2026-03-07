package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type rpcMsg map[string]any

func TestMCPStdioInitializeAndToolsList(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
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
	defer func() { _ = cmd.Process.Kill() }()

	r := bufio.NewReader(stdout)

	writeLine := func(s string) {
		_, _ = stdin.Write([]byte(s + "\n"))
	}
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
