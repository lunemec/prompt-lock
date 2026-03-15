package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func runMCPAndExchange(t *testing.T, line string) map[string]any {
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
		var msg map[string]any
		if err := json.Unmarshal([]byte(respLine), &msg); err != nil {
			errCh <- err
			return
		}
		ch <- msg
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

func TestBatchRequestRejected(t *testing.T) {
	msg := runMCPAndExchange(t, `[{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}]`)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32600 {
		t.Fatalf("expected -32600 for batch rejection, got %+v", errObj)
	}
	id, hasID := msg["id"]
	if !hasID || id != nil {
		t.Fatalf("expected id=null for batch rejection, got %+v", msg)
	}
}

func TestInvalidRequestRejected(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"1.0","id":1,"method":"","params":{}}`)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32600 {
		t.Fatalf("expected -32600 for invalid request, got %+v", errObj)
	}
}

func TestNotificationNoResponse(t *testing.T) {
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

	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"initialize","params":{}}` + "\n"))
	r := bufio.NewReader(stdout)
	ch := make(chan string, 1)
	go func() {
		line, _ := r.ReadString('\n')
		ch <- line
	}()
	select {
	case got := <-ch:
		if strings.TrimSpace(got) != "" {
			t.Fatalf("expected no notification response, got %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		// expected: no response emitted
	}
}

func TestParseErrorReturnsNullID(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":1,"method"`)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32700 {
		t.Fatalf("expected -32700 for parse error, got %+v", errObj)
	}
	id, hasID := msg["id"]
	if !hasID || id != nil {
		t.Fatalf("expected id=null for parse error, got %+v", msg)
	}
}

func TestInitializeEchoesStringID(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":"client-1","method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if got, ok := msg["id"].(string); !ok || got != "client-1" {
		t.Fatalf("expected string id echo, got %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected initialize result object, got %+v", msg)
	}
	if pv, ok := res["protocolVersion"].(string); !ok || pv == "" {
		t.Fatalf("expected initialize.protocolVersion string, got %+v", res)
	}
	caps, ok := res["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("expected initialize.capabilities object, got %+v", res)
	}
	if _, ok := caps["tools"].(map[string]any); !ok {
		t.Fatalf("expected initialize.capabilities.tools object, got %+v", caps)
	}
	if _, ok := caps["resources"]; ok {
		t.Fatalf("did not expect initialize.capabilities.resources advertisement, got %+v", caps)
	}
	if _, ok := caps["prompts"]; ok {
		t.Fatalf("did not expect initialize.capabilities.prompts advertisement, got %+v", caps)
	}
}

func TestPingSupported(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":7,"method":"ping","params":{}}`)
	if got, ok := msg["id"].(float64); !ok || int(got) != 7 {
		t.Fatalf("expected id=7 echo, got %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected ping result object, got %+v", msg)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty ping result object, got %+v", res)
	}
}

func TestShutdownSupported(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":8,"method":"shutdown","params":{}}`)
	if got, ok := msg["id"].(float64); !ok || int(got) != 8 {
		t.Fatalf("expected id=8 echo, got %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected shutdown result object, got %+v", msg)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty shutdown result object, got %+v", res)
	}
}

func TestExitNotificationTerminatesProcessWithoutResponse(t *testing.T) {
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

	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"exit","params":{}}` + "\n"))
	r := bufio.NewReader(stdout)
	lineCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		line, readErr := r.ReadString('\n')
		if readErr != nil {
			errCh <- readErr
			return
		}
		lineCh <- line
	}()

	select {
	case line := <-lineCh:
		if strings.TrimSpace(line) != "" {
			t.Fatalf("expected no response for exit notification, got %q", line)
		}
	case readErr := <-errCh:
		if readErr != io.EOF {
			t.Fatalf("expected EOF for exit notification, got %v", readErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for exit notification handling")
	}

	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	select {
	case waitErr := <-waitCh:
		if waitErr != nil {
			t.Fatalf("expected clean process exit, got %v", waitErr)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timeout waiting for process exit after exit notification")
	}
}

func TestToolsListAcceptsNullParams(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":22,"method":"tools/list","params":null}`)
	if _, ok := msg["result"].(map[string]any); !ok {
		t.Fatalf("expected tools/list result object, got %+v", msg)
	}
}

func TestToolsListIncludesInputSchemaConstraints(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":28,"method":"tools/list","params":{}}`)
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected tools/list result object, got %+v", msg)
	}
	tools, ok := res["tools"].([]any)
	if !ok || len(tools) == 0 {
		t.Fatalf("expected non-empty tools array, got %+v", res)
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first tool object, got %+v", tools[0])
	}
	schema, ok := tool["inputSchema"].(map[string]any)
	if !ok {
		t.Fatalf("expected inputSchema object, got %+v", tool)
	}
	if v, ok := schema["additionalProperties"].(bool); !ok || v {
		t.Fatalf("expected additionalProperties=false, got %+v", schema["additionalProperties"])
	}
	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("expected required array, got %+v", schema)
	}
	if len(required) < 2 {
		t.Fatalf("expected required entries for intent and command, got %+v", required)
	}
}

func TestInitializeRejectsObjectRequestID(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":{"bad":1},"method":"initialize","params":{}}`)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32600 {
		t.Fatalf("expected -32600 for invalid object id, got %+v", errObj)
	}
	if id, ok := msg["id"]; !ok || id != nil {
		t.Fatalf("expected id=null for invalid object id, got %+v", msg)
	}
}

func TestToolsCallRejectsBooleanRequestID(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":true,"method":"tools/call","params":{"name":"execute_with_intent","arguments":{"intent":"run_tests","command":["go","version"]}}}`)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32600 {
		t.Fatalf("expected -32600 for invalid boolean id, got %+v", errObj)
	}
}

func TestCancelledRejectsArrayRequestID(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":[1],"method":"notifications/cancelled","params":{"requestId":1}}`)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32600 {
		t.Fatalf("expected -32600 for invalid array id, got %+v", errObj)
	}
}

func TestResourcesListSupported(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":25,"method":"resources/list","params":{}}`)
	if got, ok := msg["id"].(float64); !ok || int(got) != 25 {
		t.Fatalf("expected id=25 echo, got %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected resources/list result object, got %+v", msg)
	}
	if _, ok := res["resources"].([]any); !ok {
		t.Fatalf("expected resources/list result.resources array, got %+v", res)
	}
}

func TestPromptsListSupported(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":26,"method":"prompts/list","params":{}}`)
	if got, ok := msg["id"].(float64); !ok || int(got) != 26 {
		t.Fatalf("expected id=26 echo, got %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected prompts/list result object, got %+v", msg)
	}
	if _, ok := res["prompts"].([]any); !ok {
		t.Fatalf("expected prompts/list result.prompts array, got %+v", res)
	}
}

func TestToolsCallNullParamsInvalidParams(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":27,"method":"tools/call","params":null}`)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32602 {
		t.Fatalf("expected -32602 for invalid params, got %+v", errObj)
	}
}

func TestInitializedRequestSupported(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":23,"method":"initialized","params":{}}`)
	if got, ok := msg["id"].(float64); !ok || int(got) != 23 {
		t.Fatalf("expected id=23 echo, got %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected initialized result object, got %+v", msg)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty initialized result object, got %+v", res)
	}
}

func TestInitializedNotificationNoResponse(t *testing.T) {
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

	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"initialized","params":{}}` + "\n"))
	r := bufio.NewReader(stdout)
	ch := make(chan string, 1)
	go func() {
		line, _ := r.ReadString('\n')
		ch <- line
	}()
	select {
	case got := <-ch:
		if strings.TrimSpace(got) != "" {
			t.Fatalf("expected no initialized notification response, got %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		// expected: no response emitted
	}
}

func TestNotificationsInitializedNotificationNoResponse(t *testing.T) {
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

	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n"))
	r := bufio.NewReader(stdout)
	ch := make(chan string, 1)
	go func() {
		line, _ := r.ReadString('\n')
		ch <- line
	}()
	select {
	case got := <-ch:
		if strings.TrimSpace(got) != "" {
			t.Fatalf("expected no notifications/initialized response, got %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		// expected: no response emitted
	}
}

func TestNotificationsCancelledRequestSupported(t *testing.T) {
	msg := runMCPAndExchange(t, `{"jsonrpc":"2.0","id":24,"method":"notifications/cancelled","params":{"requestId":1}}`)
	if got, ok := msg["id"].(float64); !ok || int(got) != 24 {
		t.Fatalf("expected id=24 echo, got %+v", msg)
	}
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected notifications/cancelled result object, got %+v", msg)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty notifications/cancelled result object, got %+v", res)
	}
}

func TestNotificationsCancelledNotificationNoResponse(t *testing.T) {
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

	_, _ = stdin.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}` + "\n"))
	r := bufio.NewReader(stdout)
	ch := make(chan string, 1)
	go func() {
		line, _ := r.ReadString('\n')
		ch <- line
	}()
	select {
	case got := <-ch:
		if strings.TrimSpace(got) != "" {
			t.Fatalf("expected no notifications/cancelled response, got %q", got)
		}
	case <-time.After(500 * time.Millisecond):
		// expected: no response emitted
	}
}
