package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestPostAuthTimesOutOnStalledTCPPeer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()
	stallAcceptedBrokerConnections(t, ln)

	oldTimeout := brokerClientTimeout
	brokerClientTimeout = 100 * time.Millisecond
	t.Cleanup(func() { brokerClientTimeout = oldTimeout })

	var out map[string]any
	err = postAuth(context.Background(), brokerTransport{BaseURL: "http://" + ln.Addr().String()}, "/v1/intents/resolve", "tok", map[string]any{"intent": "run_tests"}, &out)
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestPostAuthTimesOutOnStalledUnixSocketPeer(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-mcp-stalled-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()
	stallAcceptedBrokerConnections(t, ln)

	oldTimeout := brokerClientTimeout
	brokerClientTimeout = 100 * time.Millisecond
	t.Cleanup(func() { brokerClientTimeout = oldTimeout })

	var out map[string]any
	err = postAuth(context.Background(), brokerTransport{UnixSocket: socketPath}, "/v1/intents/resolve", "tok", map[string]any{"intent": "run_tests"}, &out)
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestPostAuthReturnsRequestConstructionErrorForMalformedBrokerURL(t *testing.T) {
	var out map[string]any
	err := postAuth(context.Background(), brokerTransport{BaseURL: "http://[::1"}, "/v1/intents/resolve", "tok", map[string]any{"intent": "run_tests"}, &out)
	if err == nil {
		t.Fatalf("expected malformed broker URL error")
	}
	if strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected raw request-construction error, got %v", err)
	}
}

func TestGetAuthReturnsRequestConstructionErrorForMalformedBrokerURL(t *testing.T) {
	var out map[string]any
	err := getAuth(context.Background(), brokerTransport{BaseURL: "http://[::1"}, "/v1/meta/capabilities", "tok", &out)
	if err == nil {
		t.Fatalf("expected malformed broker URL error")
	}
	if strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected raw request-construction error, got %v", err)
	}
}

func TestGetAuthTimesOutOnStalledTCPPeer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()
	stallAcceptedBrokerConnections(t, ln)

	oldTimeout := brokerClientTimeout
	brokerClientTimeout = 100 * time.Millisecond
	t.Cleanup(func() { brokerClientTimeout = oldTimeout })

	var out map[string]any
	err = getAuth(context.Background(), brokerTransport{BaseURL: "http://" + ln.Addr().String()}, "/v1/meta/capabilities", "tok", &out)
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestGetAuthTimesOutOnStalledUnixSocketPeer(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-mcp-stalled-get-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()
	stallAcceptedBrokerConnections(t, ln)

	oldTimeout := brokerClientTimeout
	brokerClientTimeout = 100 * time.Millisecond
	t.Cleanup(func() { brokerClientTimeout = oldTimeout })

	var out map[string]any
	err = getAuth(context.Background(), brokerTransport{UnixSocket: socketPath}, "/v1/meta/capabilities", "tok", &out)
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func stallAcceptedBrokerConnections(t *testing.T, ln net.Listener) {
	t.Helper()
	stop := make(chan struct{})
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-stop:
				default:
				}
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				<-stop
			}(conn)
		}
	}()
	t.Cleanup(func() {
		close(stop)
		time.Sleep(10 * time.Millisecond)
	})
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

func TestResolveBrokerTransportPrefersAgentUnixSocketOverAmbientBrokerURL(t *testing.T) {
	transport, err := resolveBrokerTransport(lookupEnvMap(map[string]string{
		"PROMPTLOCK_AGENT_UNIX_SOCKET": "/tmp/promptlock-agent.sock",
		"PROMPTLOCK_BROKER_URL":        "http://broker.example.internal",
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
		t.Fatalf("expected agent unix socket to beat ambient broker URL, got %q", transport.BaseURL)
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
