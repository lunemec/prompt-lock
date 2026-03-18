package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForWatchBrokerReadyAcceptsDelayedOperatorSocket(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-watch-ready-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", socketPath)
	t.Setenv("PROMPTLOCK_BROKER_URL", "")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")

	started := make(chan error, 1)
	var ln net.Listener
	var server *http.Server
	time.AfterFunc(150*time.Millisecond, func() {
		var err error
		ln, err = net.Listen("unix", socketPath)
		if err != nil {
			started <- err
			return
		}
		server = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "operator auth required", http.StatusUnauthorized)
		})}
		started <- nil
		_ = server.Serve(ln)
	})
	t.Cleanup(func() {
		if server != nil {
			_ = server.Close()
		}
		if ln != nil {
			_ = ln.Close()
		}
		_ = os.Remove(socketPath)
	})

	conn := brokerFlags{Broker: ptrString(""), BrokerUnix: ptrString("")}
	if err := waitForWatchBrokerReady(conn, "", 2*time.Second); err != nil {
		select {
		case startErr := <-started:
			if startErr != nil {
				t.Fatalf("delayed operator socket failed to start: %v", startErr)
			}
		default:
		}
		t.Fatalf("waitForWatchBrokerReady: %v", err)
	}
}

func TestWaitForWatchBrokerReadyTimesOutWhenSocketNeverAppears(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-watch-missing-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", socketPath)
	t.Setenv("PROMPTLOCK_BROKER_URL", "")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")

	conn := brokerFlags{Broker: ptrString(""), BrokerUnix: ptrString("")}
	err := waitForWatchBrokerReady(conn, "", 200*time.Millisecond)
	if err == nil {
		t.Fatalf("expected readiness timeout")
	}
	if !errors.Is(err, errWatchBrokerReadyTimeout) {
		t.Fatalf("expected readiness timeout error, got %v", err)
	}
}
