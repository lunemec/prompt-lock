package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

type failingAudit struct{}

func (failingAudit) Write(ports.AuditEvent) error { return errors.New("audit disk offline") }

func TestHandleRequestFailsClosedOnAuditWriteFailure(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        failingAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-audit-fail" },
			NewLeaseTok:  func() string { return "lease-audit-fail" },
		},
		authEnabled: false,
		authCfg:     config.AuthConfig{EnableAuth: false},
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"a1","task_id":"t1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"cmd","workdir_fingerprint":"wd"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when audit write fails, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAccessFailsClosedOnAuditWriteFailure(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	store.SetSecret("github_token", "secret-value")
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "cmd",
		WorkdirFingerprint: "wd",
		Status:             domain.RequestApproved,
		CreatedAt:          now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLease(domain.Lease{
		Token:              "lease-1",
		RequestID:          "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "cmd",
		WorkdirFingerprint: "wd",
		ExpiresAt:          now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        failingAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-1" },
			NewLeaseTok:  func() string { return "lease-1" },
		},
		authEnabled: false,
		authCfg: config.AuthConfig{
			AllowPlaintextSecretReturn: true,
		},
		now: func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"lease-1","secret":"github_token","command_fingerprint":"cmd","workdir_fingerprint":"wd"}`))
	w := httptest.NewRecorder()
	s.handleAccess(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when audit write fails before returning secret, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAuthBootstrapCreateFailsClosedOnAuditWriteFailure(t *testing.T) {
	now := time.Now().UTC()
	s := &server{
		svc:         app.Service{Audit: failingAudit{}, Now: func() time.Time { return now }},
		authEnabled: true,
		authCfg: config.AuthConfig{
			EnableAuth:               true,
			OperatorToken:            "op-token",
			BootstrapTokenTTLSeconds: 60,
			GrantIdleTimeoutMinutes:  10,
			GrantAbsoluteMaxMinutes:  60,
			SessionTTLMinutes:        10,
		},
		authStore: auth.NewStore(),
		now:       func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bootstrap/create", bytes.NewBufferString(`{"agent_id":"a1","container_id":"c1"}`))
	req.Header.Set("Authorization", "Bearer op-token")
	w := httptest.NewRecorder()
	s.handleAuthBootstrapCreate(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when auth bootstrap audit fails, got %d body=%s", w.Code, w.Body.String())
	}

	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err == nil && out["bootstrap_token"] != nil {
		t.Fatalf("expected bootstrap response to omit token on audit failure, got %+v", out)
	}
}

func TestHandleApproveKeepsCommittedStateWhenSupplementaryEnvPathAuditFails(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-env-approve-audit",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Reason:             "r",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "cmd",
		WorkdirFingerprint: "wd",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/.env",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatal(err)
	}

	audit := &scriptedAudit{failAt: map[int]error{2: errors.New("audit disk offline")}}
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unused" },
			NewLeaseTok:  func() string { return "lease-env-approve-audit" },
		},
		authEnabled: false,
		authCfg:     config.AuthConfig{EnableAuth: false},
		now:         func() time.Time { return now },
	}
	s.svc.AuditFailureHandler = func(err error) error {
		return s.closeDurabilityGate("audit", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-env-approve-audit", bytes.NewBufferString(`{"ttl_minutes":5}`))
	w := httptest.NewRecorder()
	s.handleApprove(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when supplementary env-path approval audit fails, got %d body=%s", w.Code, w.Body.String())
	}
	if s.durabilityClosed {
		t.Fatalf("did not expect durability gate to close when supplementary env-path approval audit fails")
	}
	storedReq, err := store.GetRequest("req-env-approve-audit")
	if err != nil {
		t.Fatalf("get request after supplementary env-path approval audit failure: %v", err)
	}
	if storedReq.Status != domain.RequestApproved {
		t.Fatalf("expected request to stay approved after supplementary env-path approval audit failure, got %s", storedReq.Status)
	}
	if _, err := store.GetLease("lease-env-approve-audit"); err != nil {
		t.Fatalf("expected lease to stay committed after supplementary env-path approval audit failure, got %v", err)
	}

	foundPrimary := false
	for _, ev := range audit.events {
		if ev.Event != "request_approved" {
			continue
		}
		foundPrimary = true
		if ev.Metadata["env_path_original"] != "./.env" {
			t.Fatalf("expected primary approval audit to carry env_path_original, got %q", ev.Metadata["env_path_original"])
		}
		if ev.Metadata["env_path_canonical"] != "/workspace/.env" {
			t.Fatalf("expected primary approval audit to carry env_path_canonical, got %q", ev.Metadata["env_path_canonical"])
		}
	}
	if !foundPrimary {
		t.Fatalf("expected request_approved audit event")
	}
}

func TestHandleDenyKeepsCommittedStateWhenSupplementaryEnvPathAuditFails(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-env-deny-audit",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Reason:             "r",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "cmd",
		WorkdirFingerprint: "wd",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/.env",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatal(err)
	}

	audit := &scriptedAudit{failAt: map[int]error{2: errors.New("audit disk offline")}}
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unused" },
			NewLeaseTok:  func() string { return "lease-unused" },
		},
		authEnabled: false,
		authCfg:     config.AuthConfig{EnableAuth: false},
		now:         func() time.Time { return now },
	}
	s.svc.AuditFailureHandler = func(err error) error {
		return s.closeDurabilityGate("audit", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/deny?request_id=req-env-deny-audit", bytes.NewBufferString(`{"reason":"operator_rejected"}`))
	w := httptest.NewRecorder()
	s.handleDeny(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when supplementary env-path deny audit fails, got %d body=%s", w.Code, w.Body.String())
	}
	if s.durabilityClosed {
		t.Fatalf("did not expect durability gate to close when supplementary env-path deny audit fails")
	}
	storedReq, err := store.GetRequest("req-env-deny-audit")
	if err != nil {
		t.Fatalf("get request after supplementary env-path deny audit failure: %v", err)
	}
	if storedReq.Status != domain.RequestDenied {
		t.Fatalf("expected request to stay denied after supplementary env-path deny audit failure, got %s", storedReq.Status)
	}

	foundPrimary := false
	for _, ev := range audit.events {
		if ev.Event != "request_denied" {
			continue
		}
		foundPrimary = true
		if ev.Metadata["env_path_original"] != "./.env" {
			t.Fatalf("expected primary deny audit to carry env_path_original, got %q", ev.Metadata["env_path_original"])
		}
		if ev.Metadata["env_path_canonical"] != "/workspace/.env" {
			t.Fatalf("expected primary deny audit to carry env_path_canonical, got %q", ev.Metadata["env_path_canonical"])
		}
	}
	if !foundPrimary {
		t.Fatalf("expected request_denied audit event")
	}
}
