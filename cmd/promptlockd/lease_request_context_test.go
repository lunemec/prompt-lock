package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
)

func newRequestContextServer(now time.Time) *server {
	store := memory.NewStore()
	return &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-context-1" },
			NewLeaseTok:  func() string { return "lease-context-1" },
		},
		authEnabled: false,
		authCfg:     config.AuthConfig{EnableAuth: false},
		now:         func() time.Time { return now },
	}
}

func TestHandleRequestRejectsMissingCommandFingerprint(t *testing.T) {
	s := newRequestContextServer(time.Now().UTC())
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"a1","task_id":"t1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"workdir_fingerprint":"wd-1"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing command fingerprint, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "command_fingerprint required") {
		t.Fatalf("expected command fingerprint guidance, got %s", w.Body.String())
	}
}

func TestHandleRequestRejectsMissingWorkdirFingerprint(t *testing.T) {
	s := newRequestContextServer(time.Now().UTC())
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"a1","task_id":"t1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"cmd-1"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing workdir fingerprint, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "workdir_fingerprint required") {
		t.Fatalf("expected workdir fingerprint guidance, got %s", w.Body.String())
	}
}

func TestHandlePendingRequestsIncludesEnvPathContext(t *testing.T) {
	now := time.Now().UTC()
	s := newRequestContextServer(now)
	root := t.TempDir()
	t.Setenv("PROMPTLOCK_ENV_PATH_ROOT", root)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("github_token=test\n"), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"a1","task_id":"t1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"cmd-1","workdir_fingerprint":"wd-1","env_path":"./.env","env_path_canonical":"should-be-overridden"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("request failed: code=%d body=%s", w.Code, w.Body.String())
	}

	pendingW := httptest.NewRecorder()
	pendingReq := httptest.NewRequest(http.MethodGet, "/v1/requests/pending", nil)
	s.handlePendingRequests(pendingW, pendingReq)
	if pendingW.Code != http.StatusOK {
		t.Fatalf("pending failed: code=%d body=%s", pendingW.Code, pendingW.Body.String())
	}

	var out struct {
		Pending []map[string]any `json:"pending"`
	}
	if err := json.NewDecoder(pendingW.Body).Decode(&out); err != nil {
		t.Fatalf("decode pending response: %v", err)
	}
	if len(out.Pending) != 1 {
		t.Fatalf("expected one pending item, got %d", len(out.Pending))
	}
	item := out.Pending[0]
	if got := item["CommandFingerprint"]; got != "cmd-1" {
		t.Fatalf("expected command fingerprint in pending item, got %#v", got)
	}
	if got := item["WorkdirFingerprint"]; got != "wd-1" {
		t.Fatalf("expected workdir fingerprint in pending item, got %#v", got)
	}
	if got := item["EnvPath"]; got != "./.env" {
		t.Fatalf("expected env path in pending item, got %#v", got)
	}
	if got := item["EnvPathCanonical"]; got != filepath.Join(root, ".env") {
		t.Fatalf("expected canonical env path in pending item, got %#v", got)
	}
}
