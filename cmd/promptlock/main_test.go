package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/audit"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func TestPostJSONAuthSendsBearerAndDecodes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/bootstrap/create" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("missing/invalid auth header: %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"bootstrap_token": "boot_x", "expires_at": "2026-01-01T00:00:00Z"})
	}))
	defer ts.Close()

	var out map[string]any
	if err := postJSONAuth(ts.URL, "", "/v1/auth/bootstrap/create", "tok", map[string]string{"agent_id": "a", "container_id": "c"}, &out); err != nil {
		t.Fatalf("postJSONAuth: %v", err)
	}
	if out["bootstrap_token"] != "boot_x" {
		t.Fatalf("unexpected response: %+v", out)
	}
}

func TestPostJSONAuthTimesOutOnStalledTCPPeer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer ln.Close()
	stallAcceptedConnections(t, ln)

	oldTimeout := brokerClientTimeout
	brokerClientTimeout = 100 * time.Millisecond
	t.Cleanup(func() { brokerClientTimeout = oldTimeout })

	var out map[string]any
	err = postJSONAuth("http://"+ln.Addr().String(), "", "/v1/auth/bootstrap/create", "tok", map[string]string{"agent_id": "a"}, &out)
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestPostJSONAuthTimesOutOnStalledUnixSocketPeer(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-stalled-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer ln.Close()
	stallAcceptedConnections(t, ln)

	oldTimeout := brokerClientTimeout
	brokerClientTimeout = 100 * time.Millisecond
	t.Cleanup(func() { brokerClientTimeout = oldTimeout })

	var out map[string]any
	err = postJSONAuth("http://promptlock", socketPath, "/v1/auth/bootstrap/create", "tok", map[string]string{"agent_id": "a"}, &out)
	if err == nil || !strings.Contains(err.Error(), "timed out after") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func stallAcceptedConnections(t *testing.T, ln net.Listener) {
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
	t.Cleanup(func() { close(stop) })
}

func captureCommandStdout(t *testing.T, fn func()) string {
	t.Helper()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = origStdout
	})
	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return string(out)
}

func TestAuthLoginOrchestratesBootstrapPairMint(t *testing.T) {
	const (
		operatorToken = "op_tok"
		agentID       = "agent_1"
		containerID   = "ctr_1"
		bootstrapTok  = "boot_1"
		grantID       = "grant_1"
		sessionToken  = "sess_1"
		expiresAtRaw  = "2026-01-01T00:00:00Z"
	)

	type call struct {
		path string
		auth string
		body map[string]string
	}
	var calls []call

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		calls = append(calls, call{
			path: r.URL.Path,
			auth: r.Header.Get("Authorization"),
			body: body,
		})

		switch r.URL.Path {
		case "/v1/auth/bootstrap/create":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"bootstrap_token": bootstrapTok,
				"expires_at":      expiresAtRaw,
			})
		case "/v1/auth/pair/complete":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"grant_id":            grantID,
				"idle_expires_at":     expiresAtRaw,
				"absolute_expires_at": expiresAtRaw,
			})
		case "/v1/auth/session/mint":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_token": sessionToken,
				"expires_at":    expiresAtRaw,
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	out, err := authLogin(ts.URL, "", operatorToken, agentID, containerID)
	if err != nil {
		t.Fatalf("authLogin: %v", err)
	}
	if out.GrantID != grantID {
		t.Fatalf("grant id = %q, want %q", out.GrantID, grantID)
	}
	if out.SessionToken != sessionToken {
		t.Fatalf("session token = %q, want %q", out.SessionToken, sessionToken)
	}
	if out.ExpiresAt.IsZero() {
		t.Fatalf("expected non-zero expires_at")
	}
	if len(calls) != 3 {
		t.Fatalf("call count = %d, want 3", len(calls))
	}
	if calls[0].path != "/v1/auth/bootstrap/create" || calls[1].path != "/v1/auth/pair/complete" || calls[2].path != "/v1/auth/session/mint" {
		t.Fatalf("unexpected call order: %+v", calls)
	}
	if calls[0].auth != "Bearer "+operatorToken {
		t.Fatalf("bootstrap auth header = %q", calls[0].auth)
	}
	if calls[1].auth != "" || calls[2].auth != "" {
		t.Fatalf("expected no auth header on pair/mint: %+v", calls)
	}
	if calls[0].body["agent_id"] != agentID || calls[0].body["container_id"] != containerID {
		t.Fatalf("bootstrap payload mismatch: %+v", calls[0].body)
	}
	if calls[1].body["token"] != bootstrapTok || calls[1].body["container_id"] != containerID {
		t.Fatalf("pair payload mismatch: %+v", calls[1].body)
	}
	if calls[2].body["grant_id"] != grantID {
		t.Fatalf("mint payload mismatch: %+v", calls[2].body)
	}
}

func TestRunAuthLoginDoesNotPrintBearerMaterialByDefault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/bootstrap/create":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"bootstrap_token": "boot_1",
				"expires_at":      "2026-01-01T00:00:00Z",
			})
		case "/v1/auth/pair/complete":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"grant_id":            "grant_1",
				"idle_expires_at":     "2026-01-01T00:00:00Z",
				"absolute_expires_at": "2026-01-01T00:00:00Z",
			})
		case "/v1/auth/session/mint":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_token": "sess_1",
				"expires_at":    "2026-01-01T00:00:00Z",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	out := captureCommandStdout(t, func() {
		runAuthLogin([]string{
			"--broker", ts.URL,
			"--operator-token", "op_tok",
			"--agent", "agent_1",
			"--container", "ctr_1",
		})
	})
	if strings.Contains(out, "session_token") || strings.Contains(out, "grant_id") {
		t.Fatalf("expected auth login default output to omit bearer material, got %q", out)
	}
	if !strings.Contains(out, "expires_at") {
		t.Fatalf("expected safe auth login output to retain expiry context, got %q", out)
	}
}

func TestRunAuthDockerRunDoesNotExposeSessionTokenOnDockerCommandLine(t *testing.T) {
	argsPath := filepath.Join(t.TempDir(), "docker-args.txt")
	dockerDir := t.TempDir()
	fakeDocker := filepath.Join(dockerDir, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_DOCKER_ARGS_PATH\"\n"
	if err := os.WriteFile(fakeDocker, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dockerDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_DOCKER_ARGS_PATH", argsPath)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/bootstrap/create":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"bootstrap_token": "boot_1",
				"expires_at":      "2026-01-01T00:00:00Z",
			})
		case "/v1/auth/pair/complete":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"grant_id":            "grant_1",
				"idle_expires_at":     "2026-01-01T00:00:00Z",
				"absolute_expires_at": "2026-01-01T00:00:00Z",
			})
		case "/v1/auth/session/mint":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_token": "sess_123",
				"expires_at":    "2026-01-01T00:00:00Z",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	runAuthDockerRun([]string{
		"--broker", ts.URL,
		"--operator-token", "op_tok",
		"--agent", "agent_1",
		"--container", "ctr_1",
		"--image", "promptlock-agent-lab",
	})

	argsBytes, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatalf("read fake docker args: %v", err)
	}
	if strings.Contains(string(argsBytes), "PROMPTLOCK_SESSION_TOKEN=sess_123") {
		t.Fatalf("expected docker argv to omit raw session token, got %q", string(argsBytes))
	}
}

func TestRunAuditVerifyAcceptsAppendAfterCheckpoint(t *testing.T) {
	auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
	checkpointPath := filepath.Join(t.TempDir(), "audit.checkpoint")

	sink, err := audit.NewFileSink(auditPath)
	if err != nil {
		t.Fatalf("new file sink: %v", err)
	}
	if err := sink.Write(ports.AuditEvent{Event: "e1", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("write first audit event: %v", err)
	}
	firstHash, _, err := audit.VerifyFile(auditPath)
	if err != nil {
		t.Fatalf("verify initial audit file: %v", err)
	}
	if err := audit.WriteCheckpoint(checkpointPath, firstHash); err != nil {
		t.Fatalf("write checkpoint: %v", err)
	}
	if err := sink.Write(ports.AuditEvent{Event: "e2", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("write second audit event: %v", err)
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("close audit sink: %v", err)
	}

	out := captureCommandStdout(t, func() {
		runAuditVerify([]string{"--file", auditPath, "--checkpoint", checkpointPath})
	})
	if !strings.Contains(out, "audit verify ok: records=2") {
		t.Fatalf("expected audit verify success after append, got %q", out)
	}
}

func TestAuthLoginPropagatesStepFailures(t *testing.T) {
	tests := []struct {
		name          string
		failPath      string
		expectedErr   string
		expectedCalls int
	}{
		{
			name:          "bootstrap",
			failPath:      "/v1/auth/bootstrap/create",
			expectedErr:   "bootstrap step failed",
			expectedCalls: 1,
		},
		{
			name:          "pair",
			failPath:      "/v1/auth/pair/complete",
			expectedErr:   "pair step failed",
			expectedCalls: 2,
		},
		{
			name:          "mint",
			failPath:      "/v1/auth/session/mint",
			expectedErr:   "mint step failed",
			expectedCalls: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				if r.URL.Path == tt.failPath {
					http.Error(w, "boom", http.StatusForbidden)
					return
				}
				switch r.URL.Path {
				case "/v1/auth/bootstrap/create":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"bootstrap_token": "boot_1",
						"expires_at":      "2026-01-01T00:00:00Z",
					})
				case "/v1/auth/pair/complete":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"grant_id":            "grant_1",
						"idle_expires_at":     "2026-01-01T00:00:00Z",
						"absolute_expires_at": "2026-01-01T00:00:00Z",
					})
				case "/v1/auth/session/mint":
					_ = json.NewEncoder(w).Encode(map[string]any{
						"session_token": "sess_1",
						"expires_at":    "2026-01-01T00:00:00Z",
					})
				default:
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			}))
			defer ts.Close()

			_, err := authLogin(ts.URL, "", "op_tok", "agent_1", "ctr_1")
			if err == nil {
				t.Fatalf("expected authLogin error")
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Fatalf("error = %q, want %q", err, tt.expectedErr)
			}
			if callCount != tt.expectedCalls {
				t.Fatalf("call count = %d, want %d", callCount, tt.expectedCalls)
			}
		})
	}
}

func TestRunAuthLoginDoesNotPrintGrantIDByDefault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/bootstrap/create":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"bootstrap_token": "boot_1",
				"expires_at":      "2026-01-01T00:00:00Z",
			})
		case "/v1/auth/pair/complete":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"grant_id":            "grant_1",
				"idle_expires_at":     "2026-01-01T00:00:00Z",
				"absolute_expires_at": "2026-01-01T00:00:00Z",
			})
		case "/v1/auth/session/mint":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"session_token": "sess_1",
				"expires_at":    "2026-01-01T00:00:00Z",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	out := captureCommandStdout(t, func() {
		runAuthLogin([]string{
			"--broker", ts.URL,
			"--operator-token", "op_tok",
			"--agent", "agent_1",
			"--container", "ctr_1",
		})
	})
	if strings.Contains(out, "grant_id") {
		t.Fatalf("expected auth login output to omit grant_id by default, got %q", out)
	}
	if strings.Contains(out, "session_token") {
		t.Fatalf("expected auth login output to omit session token by default, got %q", out)
	}
}

func TestBuildURL(t *testing.T) {
	if got := buildURL("http://x", "", "/v1/a"); got != "http://x/v1/a" {
		t.Fatalf("unexpected url %s", got)
	}
	if got := buildURL("http://x/", "", "/v1/a"); got != "http://x/v1/a" {
		t.Fatalf("unexpected url %s", got)
	}
	if got := buildURL("http://[::1", "/tmp/promptlock.sock", "/v1/a"); got != unixSocketRequestBaseURL+"/v1/a" {
		t.Fatalf("unexpected unix-socket url %s", got)
	}
}

func TestPostJSONAuthIncludesErrorBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret backend unavailable", http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	var out map[string]any
	err := postJSONAuth(ts.URL, "", "/v1/leases/access", "tok", map[string]string{"k": "v"}, &out)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "secret backend unavailable") {
		t.Fatalf("expected backend message in error, got %q", err)
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected status code in error, got %q", err)
	}
}

func TestRequestStatusIncludesErrorBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "request_id required", http.StatusBadRequest)
	}))
	defer ts.Close()

	_, err := requestStatus(ts.URL, "", "tok", "")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "request_id required") {
		t.Fatalf("expected request_id guidance in error, got %q", err)
	}
}

func TestWaitForApprovalDenied(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "denied"})
	}))
	defer ts.Close()

	_, err := waitForApproval(ts.URL, "", "tok", "req_1", 200*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected denied error")
	}
	if !strings.Contains(err.Error(), "request denied") {
		t.Fatalf("expected denied message, got %q", err)
	}
}

func TestValidateExecCapabilityPreconditionsMissingSession(t *testing.T) {
	err := validateExecCapabilityPreconditions(capabilities{AuthEnabled: true, AllowPlaintextSecretReturn: true}, "", true)
	if err == nil {
		t.Fatalf("expected missing session error")
	}
	if !strings.Contains(err.Error(), "broker requires session token") {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestValidateExecCapabilityPreconditionsPlaintextPolicy(t *testing.T) {
	err := validateExecCapabilityPreconditions(capabilities{AuthEnabled: true, AllowPlaintextSecretReturn: false}, "sess", false)
	if err == nil {
		t.Fatalf("expected plaintext policy error")
	}
	if !strings.Contains(err.Error(), "--broker-exec") {
		t.Fatalf("unexpected error: %q", err)
	}
}

func TestRegisterBrokerFlagsRejectsRemovedTLSFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	registerBrokerFlags(fs)
	err := fs.Parse([]string{"--broker-tls-ca-file", "/tmp/ca.pem"})
	if err == nil {
		t.Fatalf("expected removed broker TLS flag to be rejected")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("unexpected parse error: %v", err)
	}
}

func TestResolveBrokerSelectionPrefersRoleSpecificSocketDefaults(t *testing.T) {
	socketsDir := t.TempDir()
	operatorSocket := filepath.Join(socketsDir, "promptlock-operator.sock")
	agentSocket := filepath.Join(socketsDir, "promptlock-agent.sock")
	if err := os.WriteFile(operatorSocket, []byte(""), 0o600); err != nil {
		t.Fatalf("write operator socket placeholder: %v", err)
	}
	if err := os.WriteFile(agentSocket, []byte(""), 0o600); err != nil {
		t.Fatalf("write agent socket placeholder: %v", err)
	}

	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", operatorSocket)
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", agentSocket)
	t.Setenv("PROMPTLOCK_BROKER_URL", "")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")

	operatorSelection, err := resolveBrokerSelection(brokerRoleOperator, brokerSelectionInput{})
	if err != nil {
		t.Fatalf("resolve operator broker selection: %v", err)
	}
	if operatorSelection.UnixSocket != operatorSocket {
		t.Fatalf("operator unix socket = %q, want %q", operatorSelection.UnixSocket, operatorSocket)
	}
	if operatorSelection.BaseURL != defaultBrokerURL {
		t.Fatalf("operator base url = %q, want default %q", operatorSelection.BaseURL, defaultBrokerURL)
	}

	agentSelection, err := resolveBrokerSelection(brokerRoleAgent, brokerSelectionInput{})
	if err != nil {
		t.Fatalf("resolve agent broker selection: %v", err)
	}
	if agentSelection.UnixSocket != agentSocket {
		t.Fatalf("agent unix socket = %q, want %q", agentSelection.UnixSocket, agentSocket)
	}
	if agentSelection.BaseURL != defaultBrokerURL {
		t.Fatalf("agent base url = %q, want default %q", agentSelection.BaseURL, defaultBrokerURL)
	}
}

func TestResolveIntentIgnoresMalformedAmbientBrokerURLWhenExplicitUnixSocketSelected(t *testing.T) {
	socketPath := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/intents/resolve" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sess_1" {
			t.Fatalf("authorization = %q, want bearer session", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"secrets": []string{"github_token"}})
	}))

	t.Setenv("PROMPTLOCK_BROKER_URL", "http://[::1")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")

	selection, err := resolveBrokerSelection(brokerRoleAgent, brokerSelectionInput{UnixSocket: socketPath})
	if err != nil {
		t.Fatalf("resolve broker selection: %v", err)
	}
	secrets, err := resolveIntent(selection.BaseURL, selection.UnixSocket, "sess_1", "run_tests")
	if err != nil {
		t.Fatalf("resolve intent over explicit unix socket: %v", err)
	}
	if len(secrets) != 1 || secrets[0] != "github_token" {
		t.Fatalf("unexpected secrets: %#v", secrets)
	}
}

func TestResolveBrokerSelectionUsesExplicitBrokerURLWhenDefaultSocketMissing(t *testing.T) {
	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_URL", "https://broker.example.internal")

	selection, err := resolveBrokerSelection(brokerRoleOperator, brokerSelectionInput{})
	if err != nil {
		t.Fatalf("resolve operator broker selection: %v", err)
	}
	if selection.UnixSocket != "" {
		t.Fatalf("expected missing default unix socket to fall back to broker URL, got %q", selection.UnixSocket)
	}
	if selection.BaseURL != "https://broker.example.internal" {
		t.Fatalf("base url = %q, want https broker fallback", selection.BaseURL)
	}
}

func TestResolveBrokerSelectionExplicitCompatUnixSocketWins(t *testing.T) {
	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "/tmp/compat.sock")
	t.Setenv("PROMPTLOCK_BROKER_URL", "https://broker.example.internal")

	selection, err := resolveBrokerSelection(brokerRoleOperator, brokerSelectionInput{})
	if err != nil {
		t.Fatalf("resolve operator broker selection: %v", err)
	}
	if selection.UnixSocket != "/tmp/compat.sock" {
		t.Fatalf("compat unix socket = %q, want /tmp/compat.sock", selection.UnixSocket)
	}
}

func TestResolveBrokerSelectionFailsClosedWhenRoleSocketMissingAndNoExplicitBroker(t *testing.T) {
	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", filepath.Join(t.TempDir(), "missing.sock"))
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_URL", "")

	if _, err := resolveBrokerSelection(brokerRoleOperator, brokerSelectionInput{}); err == nil {
		t.Fatalf("expected missing role socket to fail closed")
	}
}

func TestResolveBrokerSelectionExplicitBrokerURLWinsOverLocalSocketDefaults(t *testing.T) {
	socketsDir := t.TempDir()
	operatorSocket := filepath.Join(socketsDir, "promptlock-operator.sock")
	if err := os.WriteFile(operatorSocket, []byte(""), 0o600); err != nil {
		t.Fatalf("write operator socket placeholder: %v", err)
	}

	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", operatorSocket)
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_URL", "")

	selection, err := resolveBrokerSelection(brokerRoleOperator, brokerSelectionInput{BaseURL: "https://broker.example.internal"})
	if err != nil {
		t.Fatalf("resolve operator broker selection: %v", err)
	}
	if selection.UnixSocket != "" {
		t.Fatalf("expected explicit broker URL to skip local unix socket selection, got %q", selection.UnixSocket)
	}
	if selection.BaseURL != "https://broker.example.internal" {
		t.Fatalf("base url = %q, want explicit broker URL", selection.BaseURL)
	}
}

func TestListPendingIgnoresMalformedAmbientBrokerURLWhenRoleSocketSelected(t *testing.T) {
	socketPath := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/requests/pending" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer op_tok" {
			t.Fatalf("authorization = %q, want operator token", got)
		}
		_ = json.NewEncoder(w).Encode(pendingResponse{
			Pending: []pendingItem{{ID: "req_1", AgentID: "agent-1", TaskID: "task-1", TTLMinutes: 5, Secrets: []string{"github_token"}}},
		})
	}))

	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", socketPath)
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", filepath.Join(t.TempDir(), "missing-agent.sock"))
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_URL", "http://[::1")

	selection, err := resolveBrokerSelection(brokerRoleOperator, brokerSelectionInput{})
	if err != nil {
		t.Fatalf("resolve operator broker selection: %v", err)
	}
	if selection.UnixSocket != socketPath {
		t.Fatalf("operator unix socket = %q, want %q", selection.UnixSocket, socketPath)
	}
	items, err := listPending(selection.BaseURL, selection.UnixSocket, "op_tok")
	if err != nil {
		t.Fatalf("listPending over operator socket: %v", err)
	}
	if len(items) != 1 || items[0].ID != "req_1" {
		t.Fatalf("unexpected pending items: %#v", items)
	}
}

func TestAuthLoginUsesOperatorSocketForBootstrapAndAgentSocketForPairMint(t *testing.T) {
	operatorSocket := filepath.Join(t.TempDir(), "operator.sock")
	agentSocket := filepath.Join(t.TempDir(), "agent.sock")
	if err := os.WriteFile(operatorSocket, []byte(""), 0o600); err != nil {
		t.Fatalf("write operator socket placeholder: %v", err)
	}
	if err := os.WriteFile(agentSocket, []byte(""), 0o600); err != nil {
		t.Fatalf("write agent socket placeholder: %v", err)
	}
	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", operatorSocket)
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", agentSocket)
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_URL", "")

	type authCall struct {
		baseURL    string
		unixSocket string
		path       string
	}

	var calls []authCall
	origDoPostJSONAuth := doPostJSONAuth
	t.Cleanup(func() { doPostJSONAuth = origDoPostJSONAuth })
	doPostJSONAuth = func(baseURL, unixSocket, path, bearer string, in any, out any) error {
		calls = append(calls, authCall{
			baseURL:    baseURL,
			unixSocket: unixSocket,
			path:       path,
		})
		switch typed := out.(type) {
		case *authBootstrapResult:
			typed.BootstrapToken = "boot_1"
			typed.ExpiresAt = time.Unix(1, 0)
		case *authPairResult:
			typed.GrantID = "grant_1"
			typed.IdleExpiresAt = time.Unix(1, 0)
			typed.AbsoluteExpiresAt = time.Unix(2, 0)
		case *authMintResult:
			typed.SessionToken = "sess_1"
			typed.ExpiresAt = time.Unix(3, 0)
		default:
			t.Fatalf("unexpected output type %T", out)
		}
		return nil
	}

	out, err := authLogin("", "", "op_tok", "agent_1", "ctr_1")
	if err != nil {
		t.Fatalf("authLogin: %v", err)
	}
	if out.SessionToken != "sess_1" {
		t.Fatalf("unexpected session token %q", out.SessionToken)
	}
	if len(calls) != 3 {
		t.Fatalf("call count = %d, want 3", len(calls))
	}
	if calls[0].path != "/v1/auth/bootstrap/create" || calls[0].unixSocket != operatorSocket {
		t.Fatalf("bootstrap call = %+v, want operator socket %q", calls[0], operatorSocket)
	}
	if calls[1].path != "/v1/auth/pair/complete" || calls[1].unixSocket != agentSocket {
		t.Fatalf("pair call = %+v, want agent socket %q", calls[1], agentSocket)
	}
	if calls[2].path != "/v1/auth/session/mint" || calls[2].unixSocket != agentSocket {
		t.Fatalf("mint call = %+v, want agent socket %q", calls[2], agentSocket)
	}
}

func TestAuthLoginIgnoresMalformedAmbientBrokerURLWhenRoleSocketsSelected(t *testing.T) {
	operatorSocket := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/bootstrap/create" {
			t.Fatalf("unexpected operator path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer op_tok" {
			t.Fatalf("authorization = %q, want operator token", got)
		}
		_ = json.NewEncoder(w).Encode(authBootstrapResult{
			BootstrapToken: "boot_1",
			ExpiresAt:      time.Unix(1, 0),
		})
	}))
	agentSocket := startUnixSocketHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/pair/complete":
			_ = json.NewEncoder(w).Encode(authPairResult{
				GrantID:           "grant_1",
				IdleExpiresAt:     time.Unix(2, 0),
				AbsoluteExpiresAt: time.Unix(3, 0),
			})
		case "/v1/auth/session/mint":
			_ = json.NewEncoder(w).Encode(authMintResult{
				SessionToken: "sess_1",
				ExpiresAt:    time.Unix(4, 0),
			})
		default:
			t.Fatalf("unexpected agent path %q", r.URL.Path)
		}
	}))

	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", operatorSocket)
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", agentSocket)
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_URL", "http://[::1")

	result, err := authLogin("", "", "op_tok", "agent_1", "ctr_1")
	if err != nil {
		t.Fatalf("authLogin with role sockets: %v", err)
	}
	if result.SessionToken != "sess_1" {
		t.Fatalf("session token = %q, want sess_1", result.SessionToken)
	}
}

func TestRunAuthLoginOmitsGrantIDFromStdout(t *testing.T) {
	operatorSocket := filepath.Join(t.TempDir(), "operator.sock")
	agentSocket := filepath.Join(t.TempDir(), "agent.sock")
	if err := os.WriteFile(operatorSocket, []byte(""), 0o600); err != nil {
		t.Fatalf("write operator socket placeholder: %v", err)
	}
	if err := os.WriteFile(agentSocket, []byte(""), 0o600); err != nil {
		t.Fatalf("write agent socket placeholder: %v", err)
	}
	t.Setenv("PROMPTLOCK_OPERATOR_UNIX_SOCKET", operatorSocket)
	t.Setenv("PROMPTLOCK_AGENT_UNIX_SOCKET", agentSocket)
	t.Setenv("PROMPTLOCK_BROKER_UNIX_SOCKET", "")
	t.Setenv("PROMPTLOCK_BROKER_URL", "")

	origDoPostJSONAuth := doPostJSONAuth
	t.Cleanup(func() { doPostJSONAuth = origDoPostJSONAuth })
	doPostJSONAuth = func(baseURL, unixSocket, path, bearer string, in any, out any) error {
		switch typed := out.(type) {
		case *authBootstrapResult:
			typed.BootstrapToken = "boot_1"
			typed.ExpiresAt = time.Unix(1, 0)
		case *authPairResult:
			typed.GrantID = "grant_1"
			typed.IdleExpiresAt = time.Unix(1, 0)
			typed.AbsoluteExpiresAt = time.Unix(2, 0)
		case *authMintResult:
			typed.SessionToken = "sess_1"
			typed.ExpiresAt = time.Unix(3, 0)
		default:
			t.Fatalf("unexpected output type %T", out)
		}
		return nil
	}

	out := captureCommandStdout(t, func() {
		runAuthLogin([]string{"--operator-token", "op_tok", "--agent", "agent_1", "--container", "ctr_1"})
	})
	if strings.Contains(out, "grant_id") {
		t.Fatalf("expected auth login stdout to omit grant_id, got %q", out)
	}
	if strings.Contains(out, "session_token") {
		t.Fatalf("expected auth login stdout to omit session_token by default, got %q", out)
	}
}

func startUnixSocketHTTPServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("promptlock-test-%d.sock", time.Now().UnixNano()))
	_ = os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		_ = server.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = server.Close()
		_ = ln.Close()
		_ = os.Remove(socketPath)
	})
	return socketPath
}
