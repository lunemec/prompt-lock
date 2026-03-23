package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func TestAccessBlockedWhenPlaintextDisabled(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "x")
	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := &server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: storeAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: false},
		authStore:   aStore,
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"l1","secret":"github_token"}`))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleAccess(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestAccessBlockedWhenPlaintextDisabledWithoutAuth(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "x")

	s := &server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: storeAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }},
		authEnabled: false,
		authCfg:     config.AuthConfig{EnableAuth: false, AllowPlaintextSecretReturn: false},
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"l1","secret":"github_token"}`))
	w := httptest.NewRecorder()
	s.handleAccess(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "plaintext secret return disabled by policy") {
		t.Fatalf("expected plaintext policy denial, got body=%q", w.Body.String())
	}
}

func TestAccessUsesServiceStateForPlaintextPolicy(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "x")
	aStore := auth.NewStore()
	aStore.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	aStore.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	s := &server{
		svc:         app.Service{Policy: domain.DefaultPolicy(), Requests: store, Leases: store, Secrets: store, Audit: storeAudit{}, Now: func() time.Time { return now }, NewRequestID: func() string { return "r1" }, NewLeaseTok: func() string { return "l1" }, AllowPlaintextSecretReturn: false},
		authEnabled: true,
		authCfg:     config.AuthConfig{EnableAuth: true, OperatorToken: "op", AllowPlaintextSecretReturn: true},
		authStore:   aStore,
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"l1","secret":"github_token"}`))
	req.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	s.handleAccess(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when service state disallows plaintext access, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "plaintext secret return disabled by policy") {
		t.Fatalf("expected plaintext policy denial, got body=%q", w.Body.String())
	}
}

type storeAudit struct{}

func (storeAudit) Write(_ ports.AuditEvent) error { return nil }
