package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
)

type failingSecretStore struct{}

func (failingSecretStore) GetSecret(string) (string, error) {
	return "", errors.New("external secret source timeout")
}

func TestHandleAccessReturns503WhenSecretBackendUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := memory.NewStore()
	if err := stateStore.SaveLease(domain.Lease{
		Token:     "lease-1",
		RequestID: "req-1",
		AgentID:   "agent-1",
		TaskID:    "task-1",
		Secrets:   []string{"github_token"},
		ExpiresAt: now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("save lease: %v", err)
	}

	s := &server{
		svc: app.Service{
			Policy:                     domain.DefaultPolicy(),
			AllowPlaintextSecretReturn: true,
			Requests:                   stateStore,
			Leases:                     stateStore,
			Secrets:                    failingSecretStore{},
			Audit:                      unavailableTestAudit{},
			Now:                        func() time.Time { return now },
			NewRequestID:               func() string { return "req-1" },
			NewLeaseTok:                func() string { return "lease-1" },
		},
		authEnabled: false,
		authCfg: config.AuthConfig{
			AllowPlaintextSecretReturn: true,
		},
		now: func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"lease-1","secret":"github_token","command_fingerprint":"","workdir_fingerprint":""}`))
	w := httptest.NewRecorder()
	s.handleAccess(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when secret backend is unavailable, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "secret backend unavailable") {
		t.Fatalf("expected secret backend unavailable message, got %q", w.Body.String())
	}
}
