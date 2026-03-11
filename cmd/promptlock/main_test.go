package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestBuildURL(t *testing.T) {
	if got := buildURL("http://x", "/v1/a"); got != "http://x/v1/a" {
		t.Fatalf("unexpected url %s", got)
	}
	if got := buildURL("http://x/", "/v1/a"); got != "http://x/v1/a" {
		t.Fatalf("unexpected url %s", got)
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

func TestHTTPClientRejectsTLSOptionsWithUnixSocket(t *testing.T) {
	orig := activeBrokerTLSOptions
	t.Cleanup(func() { activeBrokerTLSOptions = orig })
	activeBrokerTLSOptions = brokerTLSOptions{CAFile: "/tmp/ca.pem"}
	_, err := httpClient("https://broker.example", "/tmp/promptlock.sock")
	if err == nil {
		t.Fatalf("expected unix-socket + tls options error")
	}
	if !strings.Contains(err.Error(), "not supported with --broker-unix-socket") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHTTPClientRejectsTLSOptionsWithHTTPBrokerURL(t *testing.T) {
	orig := activeBrokerTLSOptions
	t.Cleanup(func() { activeBrokerTLSOptions = orig })
	activeBrokerTLSOptions = brokerTLSOptions{CAFile: "/tmp/ca.pem"}
	_, err := httpClient("http://127.0.0.1:8765", "")
	if err == nil {
		t.Fatalf("expected http broker + tls options error")
	}
	if !strings.Contains(err.Error(), "require an https broker URL") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuildBrokerTLSConfigLoadsCAAndClientCertificate(t *testing.T) {
	certPEM, keyPEM := generateSelfSignedCertPair(t)
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	certPath := filepath.Join(t.TempDir(), "client.crt")
	keyPath := filepath.Join(t.TempDir(), "client.key")
	if err := os.WriteFile(caPath, certPEM, 0o600); err != nil {
		t.Fatalf("write ca file: %v", err)
	}
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		t.Fatalf("write client cert file: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write client key file: %v", err)
	}

	cfg, err := buildBrokerTLSConfig(brokerTLSOptions{
		CAFile:         caPath,
		ClientCertFile: certPath,
		ClientKeyFile:  keyPath,
		ServerName:     "broker.example",
	})
	if err != nil {
		t.Fatalf("build tls config: %v", err)
	}
	if cfg.RootCAs == nil {
		t.Fatalf("expected root CAs to be configured")
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("expected 1 client certificate, got %d", len(cfg.Certificates))
	}
	if cfg.ServerName != "broker.example" {
		t.Fatalf("expected server name broker.example, got %q", cfg.ServerName)
	}
}

func TestBuildBrokerTLSConfigRequiresClientKeyWhenCertProvided(t *testing.T) {
	_, err := buildBrokerTLSConfig(brokerTLSOptions{ClientCertFile: "/tmp/client.crt"})
	if err == nil {
		t.Fatalf("expected missing client key error")
	}
	if !strings.Contains(err.Error(), "client cert requires") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func generateSelfSignedCertPair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	now := time.Now().UTC()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "promptlock-test",
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return certPEM, keyPEM
}
