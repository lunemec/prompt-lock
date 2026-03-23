package main

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func exchangeLine(t *testing.T, line string) map[string]any {
	t.Helper()
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = "."
	return exchangeLineWithCommand(t, cmd, line)
}

func exchangeLineWithCommand(t *testing.T, cmd *exec.Cmd, line string) map[string]any {
	t.Helper()
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
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize result must be object: %+v", msg)
	}
	if _, ok := res["protocolVersion"].(string); !ok {
		t.Fatalf("initialize.protocolVersion must be string: %+v", res)
	}
	caps, ok := res["capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("initialize.capabilities must be object: %+v", res)
	}
	if _, ok := caps["tools"].(map[string]any); !ok {
		t.Fatalf("initialize.capabilities.tools must be object: %+v", caps)
	}
	if _, ok := caps["resources"]; ok {
		t.Fatalf("initialize.capabilities.resources must be omitted until resources namespace is implemented: %+v", caps)
	}
	if _, ok := caps["prompts"]; ok {
		t.Fatalf("initialize.capabilities.prompts must be omitted until prompts namespace is implemented: %+v", caps)
	}
	info, ok := res["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("initialize.serverInfo must be object: %+v", res)
	}
	if _, ok := info["name"].(string); !ok {
		t.Fatalf("initialize.serverInfo.name must be string: %+v", info)
	}
	if got, ok := info["version"].(string); !ok || got == "" {
		t.Fatalf("initialize.serverInfo.version must be non-empty string: %+v", info)
	}
}

func TestInitializeResponseUsesInjectedBuildVersion(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "promptlock-mcp")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=v7.8.9", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build promptlock-mcp with injected version: %v\n%s", err, out)
	}
	msg := exchangeLineWithCommand(t, exec.Command(bin), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize result must be object: %+v", msg)
	}
	info, ok := res["serverInfo"].(map[string]any)
	if !ok {
		t.Fatalf("initialize.serverInfo must be object: %+v", res)
	}
	if got, ok := info["version"].(string); !ok || got != "7.8.9" {
		t.Fatalf("initialize.serverInfo.version = %#v, want 7.8.9", info["version"])
	}
}

func TestPingResponseSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":99,"method":"ping","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	if _, ok := msg["result"].(map[string]any); !ok {
		t.Fatalf("ping result must be object: %+v", msg)
	}
}

func TestShutdownResponseSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":100,"method":"shutdown","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	if _, ok := msg["result"].(map[string]any); !ok {
		t.Fatalf("shutdown result must be object: %+v", msg)
	}
}

func TestInitializedResponseSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":101,"method":"initialized","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	if _, ok := msg["result"].(map[string]any); !ok {
		t.Fatalf("initialized result must be object: %+v", msg)
	}
}

func TestCancelledRejectsBooleanRequestID(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":true,"method":"notifications/cancelled","params":{"requestId":1}}`)
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if int(errObj["code"].(float64)) != -32600 {
		t.Fatalf("expected -32600 for invalid cancellation request id, got %+v", errObj)
	}
	if id, ok := msg["id"]; !ok || id != nil {
		t.Fatalf("expected id=null for invalid cancellation request id, got %+v", msg)
	}
}

func TestInvalidRequestIDErrorUsesNullID(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":[1,2,3],"method":"ping","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	if id, ok := msg["id"]; !ok || id != nil {
		t.Fatalf("expected invalid request id response to use null id, got %+v", msg)
	}
	errObj, ok := msg["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %+v", msg)
	}
	if code, ok := errObj["code"].(float64); !ok || int(code) != -32600 {
		t.Fatalf("expected invalid request error code, got %+v", errObj)
	}
}

func TestNotificationsCancelledResponseSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":102,"method":"notifications/cancelled","params":{"requestId":1}}`)
	assertBaseJSONRPCShape(t, msg, true)
	if _, ok := msg["result"].(map[string]any); !ok {
		t.Fatalf("notifications/cancelled result must be object: %+v", msg)
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

func TestResourcesListResponseSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":200,"method":"resources/list","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("resources/list result must be object: %+v", msg)
	}
	if _, ok := res["resources"].([]any); !ok {
		t.Fatalf("resources/list result.resources must be array: %+v", msg)
	}
}

func TestPromptsListResponseSchema(t *testing.T) {
	msg := exchangeLine(t, `{"jsonrpc":"2.0","id":201,"method":"prompts/list","params":{}}`)
	assertBaseJSONRPCShape(t, msg, true)
	res, ok := msg["result"].(map[string]any)
	if !ok {
		t.Fatalf("prompts/list result must be object: %+v", msg)
	}
	if _, ok := res["prompts"].([]any); !ok {
		t.Fatalf("prompts/list result.prompts must be array: %+v", msg)
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
