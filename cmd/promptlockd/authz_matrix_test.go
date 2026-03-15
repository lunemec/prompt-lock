package main

import (
	"bytes"
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

type matrixAudit struct{}

func (matrixAudit) Write(_ ports.AuditEvent) error { return nil }

func testServer(now time.Time) *server {
	store := memory.NewStore()
	store.SetSecret("github_token", "x")
	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})
	return wiredServerForTest(&server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: matrixAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: true},
		authStore:   aStore,
		now:         func() time.Time { return now },
		intents:     map[string][]string{"run_tests": {"github_token"}},
		execPolicy:  config.ExecutionPolicy{ExactMatchExecutables: []string{"bash"}, DenylistSubstrings: []string{"printenv"}, MaxOutputBytes: 1024, DefaultTimeoutSec: 30, MaxTimeoutSec: 60},
	})
}

func TestOperatorEndpointRejectsAgentToken(t *testing.T) {
	s := testServer(time.Now().UTC())
	req := httptest.NewRequest(http.MethodGet, "/v1/requests/pending", nil)
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handlePendingRequests(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAgentEndpointRejectsOperatorToken(t *testing.T) {
	s := testServer(time.Now().UTC())
	req := httptest.NewRequest(http.MethodPost, "/v1/intents/resolve", bytes.NewBufferString(`{"intent":"run_tests"}`))
	req.Header.Set("Authorization", "Bearer op")
	w := httptest.NewRecorder()
	s.handleResolveIntent(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestCancelEndpointRejectsOperatorToken(t *testing.T) {
	s := testServer(time.Now().UTC())
	req := httptest.NewRequest(http.MethodPost, "/v1/leases/cancel?request_id=r1", bytes.NewBufferString(`{"reason":"cancel"}`))
	req.Header.Set("Authorization", "Bearer op")
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
