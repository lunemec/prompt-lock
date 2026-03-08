package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type testAudit struct{}

func (testAudit) Write(_ ports.AuditEvent) error { return nil }

func TestExecuteWithSecrets(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := &server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy:  config.ExecutionPolicy{AllowlistPrefixes: []string{"bash"}, DenylistSubstrings: []string{"printenv"}, MaxOutputBytes: 65536, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
		authStore:   aStore,
		now:         func() time.Time { return now },
	}

	payload := `{"lease_token":"l1","command":["bash","-lc","echo -n $GITHUB_TOKEN"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := out["stdout_stderr"]; !ok {
		t.Fatalf("expected stdout_stderr in response")
	}
}

func TestExecuteWithSecretsOutputModeNone(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "ok123")
	_ = store.SaveRequest(domain.LeaseRequest{ID: "r1", AgentID: "a1", TaskID: "t1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "l1", RequestID: "r1", AgentID: "a1", TaskID: "t1", Secrets: []string{"github_token"}, CommandFingerprint: "fp", WorkdirFingerprint: "wd", ExpiresAt: now.Add(5 * time.Minute)})

	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := &server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: testAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		execPolicy:  config.ExecutionPolicy{AllowlistPrefixes: []string{"bash"}, DenylistSubstrings: []string{"printenv"}, OutputSecurityMode: "none", MaxOutputBytes: 65536, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
		authStore:   aStore,
		now:         func() time.Time { return now },
	}

	payload := `{"lease_token":"l1","command":["bash","-lc","echo -n $GITHUB_TOKEN"],"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(payload))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleExecute(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		StdoutStderr string `json:"stdout_stderr"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out.StdoutStderr != "" {
		t.Fatalf("expected empty output in none mode, got %q", out.StdoutStderr)
	}
}
