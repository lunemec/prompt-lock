package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
