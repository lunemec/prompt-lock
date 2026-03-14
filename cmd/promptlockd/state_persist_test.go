package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func TestPersistRequestLeaseStateWritesFile(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "state-store.json")
	store := memory.NewStore()
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req1",
		AgentID:            "a1",
		TaskID:             "t1",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}
	s := &server{
		stateStoreFile:      path,
		stateStorePersister: store,
	}
	if err := s.persistRequestLeaseState(); err != nil {
		t.Fatalf("persist request/lease state: %v", err)
	}

	reloaded := memory.NewStore()
	if err := reloaded.LoadStateFromFile(path); err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	if _, err := reloaded.GetRequest("req1"); err != nil {
		t.Fatalf("expected persisted request, got %v", err)
	}
}

type failingStateStorePersister struct{}

func (failingStateStorePersister) SaveStateToFile(string) error { return errors.New("disk full") }

type auditCapture struct {
	events []ports.AuditEvent
}

func (a *auditCapture) Write(ev ports.AuditEvent) error {
	a.events = append(a.events, ev)
	return nil
}

func TestHandleRequestFailsClosedOnStatePersistFailure(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	audit := &auditCapture{}
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req1" },
			NewLeaseTok:  func() string { return "lease1" },
		},
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      "/non-empty",
		stateStorePersister: failingStateStorePersister{},
		now:                 func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"a1","task_id":"t1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "durability persistence unavailable") {
		t.Fatalf("expected durability failure message, got body=%q", w.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"a2","task_id":"t2","reason":"r2","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp2","workdir_fingerprint":"wd2"}`))
	w2 := httptest.NewRecorder()
	s.handleRequest(w2, req2)
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected gate-closed 503 on second write, got %d", w2.Code)
	}

	foundPersistFailure := false
	for _, ev := range audit.events {
		if ev.Event == "durability_persist_failed" {
			foundPersistFailure = true
			break
		}
	}
	if !foundPersistFailure {
		t.Fatalf("expected durability_persist_failed audit event")
	}
}

func TestDurabilityGateBlocksLeaseUseAfterApprovePersistFailure(t *testing.T) {
	now := time.Now().UTC()
	s := testServer(now)
	s.stateStoreFile = "/non-empty"
	s.stateStorePersister = failingStateStorePersister{}

	created, err := s.svc.RequestLease("a1", "task-1", "test", 5, []string{"github_token"}, "fp-approve", "wd-approve", "", "")
	if err != nil {
		t.Fatal(err)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id="+created.ID, nil)
	approveReq.Header.Set("Authorization", "Bearer op")
	approveW := httptest.NewRecorder()
	s.handleApprove(approveW, approveReq)
	if approveW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on approve persist failure, got %d body=%s", approveW.Code, approveW.Body.String())
	}

	leaseReq := httptest.NewRequest(http.MethodGet, "/v1/leases/by-request?request_id="+created.ID, nil)
	leaseReq.Header.Set("Authorization", "Bearer s1")
	leaseW := httptest.NewRecorder()
	s.handleLeaseByRequest(leaseW, leaseReq)
	if leaseW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected durability gate to block lease lookup, got %d body=%s", leaseW.Code, leaseW.Body.String())
	}

	accessReq := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"l1","secret":"github_token","command_fingerprint":"fp-approve","workdir_fingerprint":"wd-approve"}`))
	accessReq.Header.Set("Authorization", "Bearer s1")
	accessW := httptest.NewRecorder()
	s.handleAccess(accessW, accessReq)
	if accessW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected durability gate to block secret access, got %d body=%s", accessW.Code, accessW.Body.String())
	}

	execReq := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(`{"lease_token":"l1","command":["bash","-lc","echo ok"],"secrets":["github_token"],"command_fingerprint":"fp-approve","workdir_fingerprint":"wd-approve"}`))
	execReq.Header.Set("Authorization", "Bearer s1")
	execW := httptest.NewRecorder()
	s.handleExecute(execW, execReq)
	if execW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected durability gate to block execute, got %d body=%s", execW.Code, execW.Body.String())
	}
}
