package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
