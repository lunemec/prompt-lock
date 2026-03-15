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

func newFileBackedLeaseTestServer(now time.Time, path string, store *memory.Store, auditSink ports.AuditSink, requestID, leaseToken string) *server {
	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        auditSink,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return requestID },
			NewLeaseTok:  func() string { return leaseToken },
		},
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      path,
		stateStorePersister: store,
		now:                 func() time.Time { return now },
	})
	s.svc.AuditFailureHandler = func(err error) error {
		return s.closeDurabilityGate("audit", err)
	}
	return s
}

func loadPersistedLeaseState(t *testing.T, path string) *memory.Store {
	t.Helper()
	reloaded := memory.NewStore()
	if err := reloaded.LoadStateFromFile(path); err != nil {
		t.Fatalf("load persisted state: %v", err)
	}
	return reloaded
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

func TestHandleRequestRollsBackAndAvoidsSuccessAuditWhenStatePersistFails(t *testing.T) {
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
	if _, err := store.GetRequest("req1"); err == nil {
		t.Fatalf("expected request rollback after persist failure")
	}
	for _, ev := range audit.events {
		if ev.Event == "request_created" {
			t.Fatalf("did not expect request_created audit event after persist failure")
		}
	}
}

func TestHandleRequestRollsBackPersistedStateWhenAuditFails(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "state-store.json")
	store := memory.NewStore()
	s := newFileBackedLeaseTestServer(now, path, store, failingAudit{}, "req-file-audit", "lease-unused")

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"a1","task_id":"t1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp","workdir_fingerprint":"wd"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}
	if _, err := store.GetRequest("req-file-audit"); err == nil {
		t.Fatalf("expected in-memory request rollback after audit failure")
	}

	reloaded := loadPersistedLeaseState(t, path)
	if _, err := reloaded.GetRequest("req-file-audit"); err == nil {
		t.Fatalf("expected persisted request rollback after audit failure")
	}
}

func TestHandleApproveRollsBackAndAvoidsSuccessAuditWhenStatePersistFails(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	audit := &auditCapture{}
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-approve",
		AgentID:            "a1",
		TaskID:             "t1",
		Reason:             "r",
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
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unused" },
			NewLeaseTok:  func() string { return "lease-approve" },
		},
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      "/non-empty",
		stateStorePersister: failingStateStorePersister{},
		now:                 func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-approve", nil)
	w := httptest.NewRecorder()
	s.handleApprove(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}
	stored, err := store.GetRequest("req-approve")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
	if _, err := store.GetLease("lease-approve"); err == nil {
		t.Fatalf("expected lease rollback after persist failure")
	}
	for _, ev := range audit.events {
		if ev.Event == "request_approved" {
			t.Fatalf("did not expect request_approved audit event after persist failure")
		}
	}
}

func TestHandleApproveRollsBackPersistedStateWhenAuditFails(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "state-store.json")
	store := memory.NewStore()
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-approve-audit",
		AgentID:            "a1",
		TaskID:             "t1",
		Reason:             "r",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}
	if err := store.SaveStateToFile(path); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	s := newFileBackedLeaseTestServer(now, path, store, failingAudit{}, "req-unused", "lease-approve-audit")

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-approve-audit", nil)
	w := httptest.NewRecorder()
	s.handleApprove(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}

	reloaded := loadPersistedLeaseState(t, path)
	stored, err := reloaded.GetRequest("req-approve-audit")
	if err != nil {
		t.Fatalf("get persisted request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected persisted request rollback to pending, got %s", stored.Status)
	}
	if _, err := reloaded.GetLease("lease-approve-audit"); err == nil {
		t.Fatalf("expected persisted lease rollback after audit failure")
	}
}

func TestHandleDenyRollsBackAndAvoidsSuccessAuditWhenStatePersistFails(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	audit := &auditCapture{}
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-deny",
		AgentID:            "a1",
		TaskID:             "t1",
		Reason:             "r",
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
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      "/non-empty",
		stateStorePersister: failingStateStorePersister{},
		now:                 func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/deny?request_id=req-deny", bytes.NewBufferString(`{"reason":"operator_rejected"}`))
	w := httptest.NewRecorder()
	s.handleDeny(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}
	stored, err := store.GetRequest("req-deny")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
	for _, ev := range audit.events {
		if ev.Event == "request_denied" {
			t.Fatalf("did not expect request_denied audit event after persist failure")
		}
	}
}

func TestHandleDenyRollsBackPersistedStateWhenAuditFails(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "state-store.json")
	store := memory.NewStore()
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-deny-audit",
		AgentID:            "a1",
		TaskID:             "t1",
		Reason:             "r",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}
	if err := store.SaveStateToFile(path); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	s := newFileBackedLeaseTestServer(now, path, store, failingAudit{}, "req-unused", "lease-unused")

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/deny?request_id=req-deny-audit", bytes.NewBufferString(`{"reason":"operator_rejected"}`))
	w := httptest.NewRecorder()
	s.handleDeny(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}

	reloaded := loadPersistedLeaseState(t, path)
	stored, err := reloaded.GetRequest("req-deny-audit")
	if err != nil {
		t.Fatalf("get persisted request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected persisted request rollback to pending, got %s", stored.Status)
	}
}

func TestHandleApproveKeepsPersistedStateWhenSupplementaryEnvPathAuditFails(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "state-store.json")
	store := memory.NewStore()
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-env-approve-audit",
		AgentID:            "a1",
		TaskID:             "t1",
		Reason:             "r",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/.env",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}
	if err := store.SaveStateToFile(path); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	audit := &scriptedAudit{failAt: map[int]error{2: errors.New("audit disk offline")}}
	s := newFileBackedLeaseTestServer(now, path, store, audit, "req-unused", "lease-env-approve-audit")

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-env-approve-audit", bytes.NewBufferString(`{"ttl_minutes":5}`))
	w := httptest.NewRecorder()
	s.handleApprove(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}

	reloaded := loadPersistedLeaseState(t, path)
	stored, err := reloaded.GetRequest("req-env-approve-audit")
	if err != nil {
		t.Fatalf("get persisted request: %v", err)
	}
	if stored.Status != domain.RequestApproved {
		t.Fatalf("expected persisted request to stay approved, got %s", stored.Status)
	}
	if _, err := reloaded.GetLease("lease-env-approve-audit"); err != nil {
		t.Fatalf("expected persisted lease to stay committed after supplementary env-path approval audit failure, got %v", err)
	}
}

func TestHandleDenyKeepsPersistedStateWhenSupplementaryEnvPathAuditFails(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "state-store.json")
	store := memory.NewStore()
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-env-deny-audit",
		AgentID:            "a1",
		TaskID:             "t1",
		Reason:             "r",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		EnvPath:            "./.env",
		EnvPathCanonical:   "/workspace/.env",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}
	if err := store.SaveStateToFile(path); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	audit := &scriptedAudit{failAt: map[int]error{2: errors.New("audit disk offline")}}
	s := newFileBackedLeaseTestServer(now, path, store, audit, "req-unused", "lease-unused")

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/deny?request_id=req-env-deny-audit", bytes.NewBufferString(`{"reason":"operator_rejected"}`))
	w := httptest.NewRecorder()
	s.handleDeny(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%q", w.Code, w.Body.String())
	}

	reloaded := loadPersistedLeaseState(t, path)
	stored, err := reloaded.GetRequest("req-env-deny-audit")
	if err != nil {
		t.Fatalf("get persisted request: %v", err)
	}
	if stored.Status != domain.RequestDenied {
		t.Fatalf("expected persisted request to stay denied, got %s", stored.Status)
	}
}

func TestHandleCancelRollsBackAndAvoidsSuccessAuditWhenStatePersistFails(t *testing.T) {
	now := time.Now().UTC()
	store := memory.NewStore()
	audit := &auditCapture{}
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-cancel",
		AgentID:            "a1",
		TaskID:             "t1",
		Reason:             "r",
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
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      "/non-empty",
		stateStorePersister: failingStateStorePersister{},
		now:                 func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/cancel?request_id=req-cancel", bytes.NewBufferString(`{"reason":"agent requested cancellation"}`))
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}
	stored, err := store.GetRequest("req-cancel")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected request rollback to pending, got %s", stored.Status)
	}
	for _, ev := range audit.events {
		if ev.Event == "request_cancelled_by_agent" {
			t.Fatalf("did not expect request_cancelled_by_agent audit event after persist failure")
		}
	}
}

func TestHandleCancelRollsBackPersistedStateWhenAuditFails(t *testing.T) {
	now := time.Now().UTC()
	path := filepath.Join(t.TempDir(), "state-store.json")
	store := memory.NewStore()
	if err := store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-cancel-audit",
		AgentID:            "a1",
		TaskID:             "t1",
		Reason:             "r",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp",
		WorkdirFingerprint: "wd",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	}); err != nil {
		t.Fatalf("save request: %v", err)
	}
	if err := store.SaveStateToFile(path); err != nil {
		t.Fatalf("seed state file: %v", err)
	}
	s := newFileBackedLeaseTestServer(now, path, store, failingAudit{}, "req-unused", "lease-unused")

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/cancel?request_id=req-cancel-audit", bytes.NewBufferString(`{"reason":"agent requested cancellation"}`))
	w := httptest.NewRecorder()
	s.handleCancel(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%q", w.Code, w.Body.String())
	}

	reloaded := loadPersistedLeaseState(t, path)
	stored, err := reloaded.GetRequest("req-cancel-audit")
	if err != nil {
		t.Fatalf("get persisted request: %v", err)
	}
	if stored.Status != domain.RequestPending {
		t.Fatalf("expected persisted request rollback to pending, got %s", stored.Status)
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
