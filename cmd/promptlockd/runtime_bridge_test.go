package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/config"
)

func TestServeConfiguredListenersStartsAgentBridgeInDualSocketMode(t *testing.T) {
	cfg := config.Default()
	cfg.AgentUnixSocket = tempUnixSocketPath(t, "agent")
	cfg.OperatorUnixSocket = tempUnixSocketPath(t, "operator")
	cfg.AgentBridgeAddress = reserveLocalTCPAddress(t)

	s := &server{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- serveConfiguredListeners(ctx, cfg, s)
	}()

	var resp *http.Response
	var err error
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://" + cfg.AgentBridgeAddress + "/v1/meta/capabilities")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("timed out waiting for agent bridge listener: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("bridge status = %d, want 200", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("serveConfiguredListeners returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for serveConfiguredListeners shutdown")
	}
}

func TestNewUnixSocketTargetsCreatesMissingParentDirs(t *testing.T) {
	root := filepath.Join(string(os.PathSeparator), "tmp", "promptlock-runtime-test", itoa(uint64(time.Now().UnixNano())))
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	socketPath := filepath.Join(root, "nested", "runtime", "agent.sock")
	targets, err := newUnixSocketTargets([]unixSocketTarget{{
		Path:    socketPath,
		Mode:    0o600,
		Handler: http.NewServeMux(),
		Label:   "agent unix socket",
	}})
	if err != nil {
		t.Fatalf("newUnixSocketTargets: %v", err)
	}
	for _, target := range targets {
		if target.Cleanup != nil {
			target.Cleanup()
		}
	}
	if _, err := os.Stat(filepath.Dir(socketPath)); err != nil {
		t.Fatalf("expected socket parent dir to exist: %v", err)
	}
}

func tempUnixSocketPath(t *testing.T, label string) string {
	t.Helper()
	return filepath.Join(os.TempDir(), "promptlock-"+label+"-"+itoa(uint64(time.Now().UnixNano()))+".sock")
}

func reserveLocalTCPAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local tcp address: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close reserved local tcp address: %v", err)
	}
	return addr
}
