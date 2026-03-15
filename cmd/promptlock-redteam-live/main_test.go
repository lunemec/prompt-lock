package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigureHarnessTransportDevUsesLocalHTTP(t *testing.T) {
	cfg := map[string]any{
		"tls": map[string]any{"enable": true},
	}

	got, err := configureHarnessTransport("dev", t.TempDir(), 9123, cfg)
	if err != nil {
		t.Fatalf("configure transport: %v", err)
	}
	if got.operatorBase != "http://127.0.0.1:9123" {
		t.Fatalf("unexpected operator base: %q", got.operatorBase)
	}
	if got.agentBase != "http://127.0.0.1:9123" {
		t.Fatalf("unexpected agent base: %q", got.agentBase)
	}
	if got.operatorClient == nil || got.agentClient == nil {
		t.Fatalf("expected non-nil HTTP clients")
	}
	if _, exists := cfg["tls"]; exists {
		t.Fatalf("expected removed tls config to be stripped")
	}
}

func TestFinalizeAggregatesFailure(t *testing.T) {
	t.Parallel()

	rep := finalize([]result{
		{Name: "ok", OK: true},
		{Name: "failed", OK: false},
	})
	if rep.OK {
		t.Fatalf("expected aggregate report failure")
	}
	if len(rep.Results) != 2 {
		t.Fatalf("expected both results to be preserved, got %d", len(rep.Results))
	}
}

func TestEmitReportFailsClearlyWhenWriteFails(t *testing.T) {
	t.Parallel()

	outPath := filepath.Join(t.TempDir(), "missing", "report.json")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := emitReport(report{OK: true, Results: []result{{Name: "smoke", OK: true}}}, 0, outPath, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected report write failure to force exit code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("expected rendered report on stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "write report file:") {
		t.Fatalf("expected write failure on stderr, got %q", stderr.String())
	}
}

func TestWaitForUpReturnsFalseWhenServerNeverBecomesReady(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusInternalServerError)
	}))
	defer srv.Close()

	if waitForUp(srv.Client(), srv.URL, 50*time.Millisecond) {
		t.Fatalf("expected waitForUp to time out")
	}
}

func TestHTTPJSONHandlesRequestAndResponseFailures(t *testing.T) {
	t.Parallel()

	t.Run("request construction failure", func(t *testing.T) {
		status, body, raw := httpJSON(http.DefaultClient, http.MethodGet, "http://[::1", nil, nil)
		if status != 0 || body != nil || raw != "" {
			t.Fatalf("unexpected result for request construction failure: status=%d body=%v raw=%q", status, body, raw)
		}
	})

	t.Run("client failure", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		})}
		status, body, raw := httpJSON(client, http.MethodGet, "http://example.com", nil, nil)
		if status != 0 || body != nil || raw != "" {
			t.Fatalf("unexpected result for client failure: status=%d body=%v raw=%q", status, body, raw)
		}
	})

	t.Run("response body read failure", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       errReadCloser{err: errors.New("read failed")},
				Header:     make(http.Header),
			}, nil
		})}
		status, body, raw := httpJSON(client, http.MethodGet, "http://example.com", nil, nil)
		if status != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", status, http.StatusBadGateway)
		}
		if len(body) != 0 {
			t.Fatalf("expected empty parsed body on read failure, got %v", body)
		}
		if raw != "" {
			t.Fatalf("expected empty raw body on read failure, got %q", raw)
		}
	})
}

func TestTailLog(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		if got := tailLog(filepath.Join(t.TempDir(), "missing.log"), 5); got != "" {
			t.Fatalf("expected empty tail for missing file, got %q", got)
		}
	})

	t.Run("returns requested tail", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broker.log")
		if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour"), 0o644); err != nil {
			t.Fatalf("write log: %v", err)
		}
		if got := tailLog(path, 2); got != "three\nfour" {
			t.Fatalf("tailLog = %q, want %q", got, "three\nfour")
		}
	})
}

func TestRunChecksStopsOnAuthFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		handler        http.HandlerFunc
		wantResults    []string
		wantFailedName string
		wantFailedCode int
	}{
		{
			name: "bootstrap failure",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/leases/approve":
					http.Error(w, "missing auth", http.StatusUnauthorized)
				case "/v1/auth/bootstrap/create":
					http.Error(w, "bootstrap failed", http.StatusInternalServerError)
				default:
					http.NotFound(w, r)
				}
			},
			wantResults:    []string{"auth_bypass_operator_endpoint", "bootstrap_create"},
			wantFailedName: "bootstrap_create",
			wantFailedCode: http.StatusInternalServerError,
		},
		{
			name: "pair failure",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/leases/approve":
					http.Error(w, "missing auth", http.StatusUnauthorized)
				case "/v1/auth/bootstrap/create":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"bootstrap_token":"boot"}`))
				case "/v1/auth/pair/complete":
					http.Error(w, "pair failed", http.StatusInternalServerError)
				default:
					http.NotFound(w, r)
				}
			},
			wantResults:    []string{"auth_bypass_operator_endpoint", "bootstrap_create", "pair_complete"},
			wantFailedName: "pair_complete",
			wantFailedCode: http.StatusInternalServerError,
		},
		{
			name: "session mint failure",
			handler: func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/leases/approve":
					http.Error(w, "missing auth", http.StatusUnauthorized)
				case "/v1/auth/bootstrap/create":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"bootstrap_token":"boot"}`))
				case "/v1/auth/pair/complete":
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"grant_id":"grant"}`))
				case "/v1/auth/session/mint":
					http.Error(w, "mint failed", http.StatusInternalServerError)
				default:
					http.NotFound(w, r)
				}
			},
			wantResults:    []string{"auth_bypass_operator_endpoint", "bootstrap_create", "pair_complete", "bootstrap_replay_denied", "session_mint"},
			wantFailedName: "session_mint",
			wantFailedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tt.handler)
			defer srv.Close()

			transport := harnessTransport{
				operatorClient: srv.Client(),
				operatorBase:   srv.URL,
				agentClient:    srv.Client(),
				agentBase:      srv.URL,
			}

			rep := runChecks(transport, "operator-token")
			if rep.OK {
				t.Fatalf("expected failed report")
			}
			if len(rep.Results) != len(tt.wantResults) {
				t.Fatalf("result count = %d, want %d", len(rep.Results), len(tt.wantResults))
			}
			for i, wantName := range tt.wantResults {
				if rep.Results[i].Name != wantName {
					t.Fatalf("result[%d].Name = %q, want %q", i, rep.Results[i].Name, wantName)
				}
			}
			last := rep.Results[len(rep.Results)-1]
			if last.Name != tt.wantFailedName {
				t.Fatalf("failed result = %q, want %q", last.Name, tt.wantFailedName)
			}
			if last.Status != tt.wantFailedCode {
				t.Fatalf("failed status = %d, want %d", last.Status, tt.wantFailedCode)
			}
			if last.OK {
				t.Fatalf("expected final result to fail")
			}
		})
	}
}

func TestRunChecksUsesApprovedLeaseForEgressDeny(t *testing.T) {
	t.Parallel()

	const (
		operatorToken = "operator-token"
		requestID     = "req_1"
		leaseToken    = "lease_1"
	)

	var (
		pairCalls      int
		sawLeaseCreate bool
		sawApprove     bool
		sawExecute     bool
		executeLease   string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/leases/approve":
			authz := r.Header.Get("Authorization")
			switch authz {
			case "":
				http.Error(w, "missing auth", http.StatusUnauthorized)
			case "Bearer " + operatorToken:
				if !sawLeaseCreate {
					http.Error(w, "request missing", http.StatusConflict)
					return
				}
				if got := r.URL.Query().Get("request_id"); got != requestID {
					http.Error(w, "wrong request id", http.StatusBadRequest)
					return
				}
				sawApprove = true
				_, _ = w.Write([]byte(`{"lease_token":"` + leaseToken + `"}`))
			default:
				http.Error(w, "wrong role", http.StatusUnauthorized)
			}
		case "/v1/auth/bootstrap/create":
			_, _ = w.Write([]byte(`{"bootstrap_token":"boot_1"}`))
		case "/v1/auth/pair/complete":
			if pairCalls > 0 {
				http.Error(w, "bootstrap replay", http.StatusForbidden)
				return
			}
			pairCalls++
			_, _ = w.Write([]byte(`{"grant_id":"grant_1"}`))
		case "/v1/auth/session/mint":
			_, _ = w.Write([]byte(`{"session_token":"sess_1"}`))
		case "/v1/leases/request":
			if got := r.Header.Get("Authorization"); got != "Bearer sess_1" {
				http.Error(w, "missing session", http.StatusUnauthorized)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode lease request: %v", err)
			}
			if got, _ := body["intent"].(string); got != "run_tests" {
				t.Fatalf("intent = %q, want run_tests", got)
			}
			sawLeaseCreate = true
			_, _ = w.Write([]byte(`{"request_id":"` + requestID + `"}`))
		case "/v1/leases/execute":
			if !sawApprove {
				http.Error(w, "approve missing", http.StatusConflict)
				return
			}
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode execute body: %v", err)
			}
			executeLease, _ = body["lease_token"].(string)
			if executeLease != leaseToken {
				http.Error(w, "wrong lease token", http.StatusBadRequest)
				return
			}
			sawExecute = true
			http.Error(w, "network egress denied by substring \"169.254.169.254\"", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	transport := harnessTransport{
		operatorClient: srv.Client(),
		operatorBase:   srv.URL,
		agentClient:    srv.Client(),
		agentBase:      srv.URL,
	}

	rep := runChecks(transport, operatorToken)
	if !rep.OK {
		t.Fatalf("expected report success, got %+v", rep)
	}
	if !sawLeaseCreate || !sawApprove || !sawExecute {
		t.Fatalf("expected real request/approve/execute sequence, got create=%v approve=%v execute=%v", sawLeaseCreate, sawApprove, sawExecute)
	}
	if executeLease != leaseToken {
		t.Fatalf("execute lease token = %q, want %q", executeLease, leaseToken)
	}
	last := rep.Results[len(rep.Results)-1]
	if last.Name != "egress_bypass_denied" {
		t.Fatalf("last result = %q, want egress_bypass_denied", last.Name)
	}
	if !last.OK || last.Status != http.StatusForbidden {
		t.Fatalf("unexpected final result: %+v", last)
	}
	if !strings.Contains(last.Detail, "network egress denied by substring") {
		t.Fatalf("expected egress-specific deny detail, got %+v", last)
	}
}

func TestRunChecksRejectsGenericForbiddenAsEgressProof(t *testing.T) {
	t.Parallel()

	transport := harnessTransportForEgressResult(t, http.StatusForbidden, "forbidden")

	rep := runChecks(transport, "operator-token")
	if rep.OK {
		t.Fatalf("expected report failure for generic forbidden result")
	}
	last := rep.Results[len(rep.Results)-1]
	if last.Name != "egress_bypass_denied" {
		t.Fatalf("last result = %q, want egress_bypass_denied", last.Name)
	}
	if last.OK {
		t.Fatalf("expected generic forbidden to fail egress proof: %+v", last)
	}
	if !strings.Contains(last.Detail, "network egress-specific deny evidence") {
		t.Fatalf("expected egress-proof guidance in detail, got %+v", last)
	}
}

func TestRunChecksRejectsExecutionPolicyForbiddenAsEgressProof(t *testing.T) {
	t.Parallel()

	transport := harnessTransportForEgressResult(t, http.StatusForbidden, "command \"curl\" not allowed by execution policy")

	rep := runChecks(transport, "operator-token")
	if rep.OK {
		t.Fatalf("expected report failure for execution-policy deny")
	}
	last := rep.Results[len(rep.Results)-1]
	if last.OK {
		t.Fatalf("expected execution-policy deny to fail egress proof: %+v", last)
	}
	if !strings.Contains(last.Detail, "execution policy") {
		t.Fatalf("expected execution-policy detail to be preserved, got %+v", last)
	}
}

func TestRunChecksRejectsCommandResolutionForbiddenAsEgressProof(t *testing.T) {
	t.Parallel()

	transport := harnessTransportForEgressResult(t, http.StatusForbidden, "command \"curl\" not found in execution_policy.command_search_paths")

	rep := runChecks(transport, "operator-token")
	if rep.OK {
		t.Fatalf("expected report failure for command-resolution deny")
	}
	last := rep.Results[len(rep.Results)-1]
	if last.OK {
		t.Fatalf("expected command-resolution deny to fail egress proof: %+v", last)
	}
	if !strings.Contains(last.Detail, "execution_policy.command_search_paths") {
		t.Fatalf("expected command-resolution detail to be preserved, got %+v", last)
	}
}

func harnessTransportForEgressResult(t *testing.T, executeStatus int, executeBody string) harnessTransport {
	t.Helper()

	const (
		operatorToken = "operator-token"
		requestID     = "req_1"
		leaseToken    = "lease_1"
	)

	var pairCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/leases/approve":
			authz := r.Header.Get("Authorization")
			switch authz {
			case "":
				http.Error(w, "missing auth", http.StatusUnauthorized)
			case "Bearer " + operatorToken:
				if got := r.URL.Query().Get("request_id"); got != requestID {
					http.Error(w, "wrong request id", http.StatusBadRequest)
					return
				}
				_, _ = w.Write([]byte(`{"lease_token":"` + leaseToken + `"}`))
			default:
				http.Error(w, "wrong role", http.StatusUnauthorized)
			}
		case "/v1/auth/bootstrap/create":
			_, _ = w.Write([]byte(`{"bootstrap_token":"boot_1"}`))
		case "/v1/auth/pair/complete":
			if pairCalls > 0 {
				http.Error(w, "bootstrap replay", http.StatusForbidden)
				return
			}
			pairCalls++
			_, _ = w.Write([]byte(`{"grant_id":"grant_1"}`))
		case "/v1/auth/session/mint":
			_, _ = w.Write([]byte(`{"session_token":"sess_1"}`))
		case "/v1/leases/request":
			_, _ = w.Write([]byte(`{"request_id":"` + requestID + `"}`))
		case "/v1/leases/execute":
			http.Error(w, executeBody, executeStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return harnessTransport{
		operatorClient: srv.Client(),
		operatorBase:   srv.URL,
		agentClient:    srv.Client(),
		agentBase:      srv.URL,
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read([]byte) (int, error) {
	return 0, r.err
}

func (r errReadCloser) Close() error {
	return nil
}

func TestConfigureHarnessTransportHardenedUsesDualSockets(t *testing.T) {
	tempDir := t.TempDir()
	cfg := map[string]any{
		"tls": map[string]any{"enable": true},
	}

	got, err := configureHarnessTransport("hardened", tempDir, 9123, cfg)
	if err != nil {
		t.Fatalf("configure transport: %v", err)
	}
	if got.operatorBase != "http://unix" {
		t.Fatalf("unexpected operator base: %q", got.operatorBase)
	}
	if got.agentBase != "http://unix" {
		t.Fatalf("unexpected agent base: %q", got.agentBase)
	}
	if got.operatorClient == nil || got.agentClient == nil {
		t.Fatalf("expected unix-socket clients")
	}
	if gotSocket, ok := cfg["agent_unix_socket"].(string); !ok || gotSocket != filepath.Join(tempDir, "promptlock-agent.sock") {
		t.Fatalf("unexpected agent socket config: %#v", cfg["agent_unix_socket"])
	}
	if gotSocket, ok := cfg["operator_unix_socket"].(string); !ok || gotSocket != filepath.Join(tempDir, "promptlock-operator.sock") {
		t.Fatalf("unexpected operator socket config: %#v", cfg["operator_unix_socket"])
	}
	if _, exists := cfg["tls"]; exists {
		t.Fatalf("expected removed tls config to be stripped")
	}
}
