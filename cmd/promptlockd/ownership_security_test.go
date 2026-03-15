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

type ownershipAudit struct{}

func (ownershipAudit) Write(_ ports.AuditEvent) error { return nil }

func newOwnershipTestServer(now time.Time) *server {
	store := memory.NewStore()
	store.SetSecret("github_token", "secret-value")
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-a",
		AgentID:            "agent-a",
		TaskID:             "task-a",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp-a",
		WorkdirFingerprint: "wd-a",
		Status:             domain.RequestApproved,
		CreatedAt:          now,
	})
	_ = store.SaveLease(domain.Lease{
		Token:              "lease-a",
		RequestID:          "req-a",
		AgentID:            "agent-a",
		TaskID:             "task-a",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp-a",
		WorkdirFingerprint: "wd-a",
		ExpiresAt:          now.Add(5 * time.Minute),
	})

	authStore := auth.NewStore()
	authStore.SaveGrant(auth.PairingGrant{GrantID: "grant-a", AgentID: "agent-a", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour)})
	authStore.SaveGrant(auth.PairingGrant{GrantID: "grant-b", AgentID: "agent-b", CreatedAt: now, LastUsedAt: now, IdleExpiresAt: now.Add(10 * time.Minute), AbsoluteExpiresAt: now.Add(time.Hour)})
	authStore.SaveSession(auth.SessionToken{Token: "session-a", GrantID: "grant-a", AgentID: "agent-a", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})
	authStore.SaveSession(auth.SessionToken{Token: "session-b", GrantID: "grant-b", AgentID: "agent-b", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)})

	return wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        ownershipAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-new" },
			NewLeaseTok:  func() string { return "lease-new" },
		},
		authEnabled: true,
		authCfg: config.AuthConfig{
			EnableAuth:                 true,
			OperatorToken:              "operator-token",
			AllowPlaintextSecretReturn: true,
		},
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"echo"},
			DenylistSubstrings:    []string{"printenv"},
			OutputSecurityMode:    "raw",
			MaxOutputBytes:        1024,
			DefaultTimeoutSec:     30,
			MaxTimeoutSec:         60,
		},
		authStore: authStore,
		now:       func() time.Time { return now },
	})
}

func TestRequestRejectsAgentIDMismatchWithAuthenticatedSession(t *testing.T) {
	s := newOwnershipTestServer(time.Now().UTC())

	w := callJSONHandler(
		t,
		s.handleRequest,
		http.MethodPost,
		"/v1/leases/request",
		"session-b",
		`{"agent_id":"agent-a","task_id":"spoof","reason":"impersonate","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-spoof","workdir_fingerprint":"wd-spoof"}`,
	)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for mismatched session agent/body agent, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCrossAgentRequestAndLeaseOperationsAreForbidden(t *testing.T) {
	s := newOwnershipTestServer(time.Now().UTC())

	statusW := callJSONHandler(t, s.handleRequestStatus, http.MethodGet, "/v1/requests/status?request_id=req-a", "session-b", "")
	if statusW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-agent request status, got %d body=%s", statusW.Code, statusW.Body.String())
	}

	byRequestW := callJSONHandler(t, s.handleLeaseByRequest, http.MethodGet, "/v1/leases/by-request?request_id=req-a", "session-b", "")
	if byRequestW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-agent lease-by-request, got %d body=%s", byRequestW.Code, byRequestW.Body.String())
	}

	accessReq := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"lease-a","secret":"github_token","command_fingerprint":"fp-a","workdir_fingerprint":"wd-a"}`))
	accessReq.Header.Set("Authorization", "Bearer session-b")
	accessW := httptest.NewRecorder()
	s.handleAccess(accessW, accessReq)
	if accessW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-agent secret access, got %d body=%s", accessW.Code, accessW.Body.String())
	}

	execW := callJSONHandler(
		t,
		s.handleExecute,
		http.MethodPost,
		"/v1/leases/execute",
		"session-b",
		`{"lease_token":"lease-a","intent":"run_tests","command":["echo","ok"],"secrets":["github_token"],"command_fingerprint":"fp-a","workdir_fingerprint":"wd-a"}`,
	)
	if execW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-agent execute, got %d body=%s", execW.Code, execW.Body.String())
	}
}
