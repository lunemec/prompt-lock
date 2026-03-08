package main

import (
	"bufio"
	"encoding/json"
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
