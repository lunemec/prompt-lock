package main

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/auth"
	"github.com/lunemec/promptlock/internal/config"
)

func TestRequireOperator(t *testing.T) {
	s := &server{authEnabled: true, authCfg: config.AuthConfig{EnableAuth: true, OperatorToken: "op"}, authStore: auth.NewStore(), now: func() time.Time { return time.Now().UTC() }}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer op")
	w := httptest.NewRecorder()
	_, ok := s.requireOperator(w, r)
	if !ok {
		t.Fatalf("expected operator auth ok")
	}
}

func TestRequireAgentSession(t *testing.T) {
	now := time.Now().UTC()
	store := auth.NewStore()
	store.SaveGrant(auth.PairingGrant{GrantID: "g1", AgentID: "a1", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(1 * time.Hour)})
	store.SaveSession(auth.SessionToken{Token: "s1", GrantID: "g1", AgentID: "a1", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})
	s := &server{authEnabled: true, authCfg: config.AuthConfig{EnableAuth: true, OperatorToken: "op"}, authStore: store, now: func() time.Time { return now }}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer s1")
	w := httptest.NewRecorder()
	_, ok := s.requireAgentSession(w, r)
	if !ok {
		t.Fatalf("expected agent session auth ok")
	}
}
