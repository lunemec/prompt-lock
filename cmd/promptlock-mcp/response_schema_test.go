package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func exchangeLine(t *testing.T, line string) map[string]any {
	t.Helper()
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
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

	_, _ = stdin.Write([]byte(line + "\n"))

	r := bufio.NewReader(stdout)
	ch := make(chan map[string]any, 1)
	errCh := make(chan error, 1)
	go func() {
		respLine, err := r.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		respLine = strings.TrimSpace(respLine)
		var m map[string]any
		if err := json.Unmarshal([]byte(respLine), &m); err != nil {
			errCh <- err
			return
		}
		ch <- m
	}()

	select {
	case m := <-ch:
		return m
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response")
	}
	return nil
}

func assertBaseJSONRPCShape(t *testing.T, msg map[string]any, expectID bool) {
	t.Helper()
	if v, ok := msg["jsonrpc"].(string); !ok || v != "2.0" {
		t.Fatalf("jsonrpc must be '2.0', got %#v", msg["jsonrpc"])
	}
	_, hasID := msg["id"]
	if expectID && !hasID {
		t.Fatalf("expected id in response: %+v", msg)
	}
	if !expectID && hasID {
		t.Fatalf("did not expect id in response: %+v", msg)
	}
	_, hasResult := msg["result"]
	_, hasError := msg["error"]
	if hasResult == hasError {
		t.Fatalf("response must have exactly one of result/error: %+v", msg)
	}
}

func TestInitializeResponseSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	if _, ok := msg["result"].(map[string]any); !ok {
		t.Fatalf("initialize result must be object: %+v", msg)
	}
}

func TestToolsListResponseSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list result must be object: %+v", msg)
	}
	if _, ok := res["tools"].([]any); !ok {
		t.Fatalf("tools/list result.tools must be array: %+v", msg)
	}
}

func TestUnknownMethodErrorSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":3,"method":"nope","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("error must be object: %+v", msg)
	}
	if _, ok := errObj["code"].(float64); !ok {
		t.Fatalf("error.code must be number: %+v", errObj)
	}
	if _, ok := errObj["message"].(string); !ok {
		t.Fatalf("error.message must be string: %+v", errObj)
	}
}
