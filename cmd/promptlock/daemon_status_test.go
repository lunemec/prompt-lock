package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestDaemonStatusReportFromFlagsIncludesBridgeDiagnostics(t *testing.T) {
	bridgeAddr := startLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta/capabilities" {
			t.Fatalf("unexpected bridge path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	socketPath := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/meta/capabilities" {
			t.Fatalf("unexpected broker path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth_enabled":                  true,
			"allow_plaintext_secret_return": false,
			"agent_bridge_address":          bridgeAddr,
		})
	}))
	pidFile := filepath.Join(t.TempDir(), "promptlockd.pid")
	if err := os.WriteFile(pidFile, []byte(strings.TrimSpace(strconv.Itoa(os.Getpid()))+"\n"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	configPath := writeDaemonStatusConfig(t, map[string]any{
		"agent_unix_socket":    socketPath,
		"agent_bridge_address": bridgeAddr,
	})

	report, err := daemonStatusReportFromFlags(daemonFlags{
		PIDFile: pidFile,
		Config:  configPath,
	})
	if err != nil {
		t.Fatalf("daemonStatusReportFromFlags: %v", err)
	}
	if report.Status != "running" {
		t.Fatalf("status = %q, want running", report.Status)
	}
	if !report.AgentTransportReachable {
		t.Fatalf("expected agent transport to be reachable, got %+v", report)
	}
	if report.AgentTransport != "unix_socket" || report.AgentTransportTarget != socketPath {
		t.Fatalf("unexpected agent transport: %+v", report)
	}
	if !report.AgentBridgeAdvertised {
		t.Fatalf("expected bridge to be advertised, got %+v", report)
	}
	if !report.AgentBridgeReachable {
		t.Fatalf("expected bridge to be reachable, got %+v", report)
	}
	if report.AgentBridgeHostURL != "http://"+bridgeAddr {
		t.Fatalf("agent bridge host url = %q, want %q", report.AgentBridgeHostURL, "http://"+bridgeAddr)
	}
	if report.AgentBridgeContainerURL != dockerBridgeURLForHostAlias(bridgeAddr, "host.docker.internal") {
		t.Fatalf("agent bridge container url = %q", report.AgentBridgeContainerURL)
	}
}

func TestDaemonStatusJSONOutputIncludesBridgeDiagnostics(t *testing.T) {
	bridgeAddr := startLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	socketPath := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth_enabled":                  true,
			"allow_plaintext_secret_return": false,
			"agent_bridge_address":          bridgeAddr,
		})
	}))
	pidFile := filepath.Join(t.TempDir(), "promptlockd.pid")
	if err := os.WriteFile(pidFile, []byte(strings.TrimSpace(strconv.Itoa(os.Getpid()))+"\n"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	configPath := writeDaemonStatusConfig(t, map[string]any{
		"agent_unix_socket":    socketPath,
		"agent_bridge_address": bridgeAddr,
	})

	out := captureCommandStdout(t, func() {
		if err := daemonStatus(daemonFlags{
			PIDFile:    pidFile,
			Config:     configPath,
			JSONOutput: true,
		}); err != nil {
			t.Fatalf("daemonStatus: %v", err)
		}
	})

	var report daemonStatusReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("unmarshal daemon status json: %v (out=%q)", err, out)
	}
	if report.Status != "running" || !report.AgentBridgeReachable {
		t.Fatalf("unexpected daemon status report: %+v", report)
	}
	if report.AgentBridgeContainerURL == "" {
		t.Fatalf("expected container bridge url in report: %+v", report)
	}
}

func TestDaemonStatusTextOutputIncludesBridgeDiagnostics(t *testing.T) {
	bridgeAddr := startLoopbackHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	socketPath := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"auth_enabled":                  true,
			"allow_plaintext_secret_return": false,
			"agent_bridge_address":          bridgeAddr,
		})
	}))
	pidFile := filepath.Join(t.TempDir(), "promptlockd.pid")
	if err := os.WriteFile(pidFile, []byte(strings.TrimSpace(strconv.Itoa(os.Getpid()))+"\n"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}
	configPath := writeDaemonStatusConfig(t, map[string]any{
		"agent_unix_socket":    socketPath,
		"agent_bridge_address": bridgeAddr,
	})

	out := captureCommandStdout(t, func() {
		if err := daemonStatus(daemonFlags{
			PIDFile: pidFile,
			Config:  configPath,
		}); err != nil {
			t.Fatalf("daemonStatus: %v", err)
		}
	})

	if !strings.Contains(out, "promptlockd status: running") {
		t.Fatalf("expected running status in output, got %q", out)
	}
	if !strings.Contains(out, "agent bridge: reachable on host") {
		t.Fatalf("expected bridge reachability in output, got %q", out)
	}
	if !strings.Contains(out, "container bridge url: http://host.docker.internal:") {
		t.Fatalf("expected container bridge url in output, got %q", out)
	}
}

func TestDaemonStatusReportTreatsZombiePIDAsStale(t *testing.T) {
	origProcessState := daemonReadProcessState
	daemonReadProcessState = func(pid int) (string, error) { return "Z", nil }
	t.Cleanup(func() { daemonReadProcessState = origProcessState })

	pidFile := filepath.Join(t.TempDir(), "promptlockd.pid")
	if err := os.WriteFile(pidFile, []byte(strings.TrimSpace(strconv.Itoa(os.Getpid()))+"\n"), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	report, err := daemonStatusReportFromFlags(daemonFlags{PIDFile: pidFile})
	if err != nil {
		t.Fatalf("daemonStatusReportFromFlags: %v", err)
	}
	if report.Status != "stale" {
		t.Fatalf("status = %q, want stale", report.Status)
	}
}

func startLoopbackHTTPServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen loopback tcp: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		_ = ln.Close()
	})
	return ln.Addr().String()
}

func writeDaemonStatusConfig(t *testing.T, doc map[string]any) string {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
