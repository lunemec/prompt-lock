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
	"github.com/lunemec/promptlock/internal/core/ports"
)

type unavailableStateStore struct{}

type unavailableTestAudit struct{}

func (unavailableTestAudit) Write(ports.AuditEvent) error { return nil }

func (unavailableStateStore) SaveRequest(domain.LeaseRequest) error {
	return errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}
func (unavailableStateStore) GetRequest(string) (domain.LeaseRequest, error) {
	return domain.LeaseRequest{}, errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}
func (unavailableStateStore) UpdateRequest(domain.LeaseRequest) error {
	return errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}
func (unavailableStateStore) DeleteRequest(string) error {
	return errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}
func (unavailableStateStore) ListPendingRequests() ([]domain.LeaseRequest, error) {
	return nil, errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}
func (unavailableStateStore) SaveLease(domain.Lease) error {
	return errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}
func (unavailableStateStore) DeleteLease(string) error {
	return errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}
func (unavailableStateStore) GetLease(string) (domain.Lease, error) {
	return domain.Lease{}, errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}
func (unavailableStateStore) GetLeaseByRequestID(string) (domain.Lease, error) {
	return domain.Lease{}, errors.Join(ports.ErrStoreUnavailable, errors.New("state backend timeout"))
}

func TestHandleRequestReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	secretStore := memory.NewStore()
	secretStore.SetSecret("github_token", "x")
	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     stateStore,
			Leases:       stateStore,
			Secrets:      secretStore,
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unavailable" },
			NewLeaseTok:  func() string { return "lease-unavailable" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"a1","task_id":"t1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-1","workdir_fingerprint":"wd-1"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleRequestStatusReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	secretStore := memory.NewStore()
	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     stateStore,
			Leases:       stateStore,
			Secrets:      secretStore,
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unavailable" },
			NewLeaseTok:  func() string { return "lease-unavailable" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/requests/status?request_id=req-unavailable", nil)
	w := httptest.NewRecorder()
	s.handleRequestStatus(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleApproveReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	s := &server{
		svc: app.Service{
			Policy:                     domain.DefaultPolicy(),
			AllowPlaintextSecretReturn: true,
			Requests:                   stateStore,
			Leases:                     stateStore,
			Secrets:                    memory.NewStore(),
			Audit:                      unavailableTestAudit{},
			Now:                        func() time.Time { return now },
			NewRequestID:               func() string { return "req-unavailable" },
			NewLeaseTok:                func() string { return "lease-unavailable" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-unavailable", bytes.NewBufferString(`{"ttl_minutes":5}`))
	w := httptest.NewRecorder()
	s.handleApprove(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleDenyReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	s := &server{
		svc: app.Service{
			Policy:                     domain.DefaultPolicy(),
			AllowPlaintextSecretReturn: true,
			Requests:                   stateStore,
			Leases:                     stateStore,
			Secrets:                    memory.NewStore(),
			Audit:                      unavailableTestAudit{},
			Now:                        func() time.Time { return now },
			NewRequestID:               func() string { return "req-unavailable" },
			NewLeaseTok:                func() string { return "lease-unavailable" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/deny?request_id=req-unavailable", bytes.NewBufferString(`{"reason":"test"}`))
	w := httptest.NewRecorder()
	s.handleDeny(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleCancelReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	s := &server{
		svc: app.Service{
			Policy:                     domain.DefaultPolicy(),
			AllowPlaintextSecretReturn: true,
			Requests:                   stateStore,
			Leases:                     stateStore,
			Secrets:                    memory.NewStore(),
			Audit:                      unavailableTestAudit{},
			Now:                        func() time.Time { return now },
			NewRequestID:               func() string { return "req-unavailable" },
			NewLeaseTok:                func() string { return "lease-unavailable" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/cancel?request_id=req-unavailable", bytes.NewBufferString(`{"reason":"cancel"}`))
	w := httptest.NewRecorder()
	s.handleCancel(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandlePendingRequestsReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     stateStore,
			Leases:       stateStore,
			Secrets:      memory.NewStore(),
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unavailable" },
			NewLeaseTok:  func() string { return "lease-unavailable" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/requests/pending", nil)
	w := httptest.NewRecorder()
	s.handlePendingRequests(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleLeaseByRequestReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     stateStore,
			Leases:       stateStore,
			Secrets:      memory.NewStore(),
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unavailable" },
			NewLeaseTok:  func() string { return "lease-unavailable" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/leases/by-request?request_id=req-unavailable", nil)
	w := httptest.NewRecorder()
	s.handleLeaseByRequest(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAccessReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	s := &server{
		svc: app.Service{
			Policy:                     domain.DefaultPolicy(),
			Requests:                   stateStore,
			Leases:                     stateStore,
			Secrets:                    memory.NewStore(),
			Audit:                      unavailableTestAudit{},
			Now:                        func() time.Time { return now },
			NewRequestID:               func() string { return "req-unavailable" },
			NewLeaseTok:                func() string { return "lease-unavailable" },
			AllowPlaintextSecretReturn: true,
		},
		authEnabled: false,
		authCfg:     config.AuthConfig{AllowPlaintextSecretReturn: true},
		now:         func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"lease-unavailable","secret":"github_token"}`))
	w := httptest.NewRecorder()
	s.handleAccess(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "state backend unavailable") {
		t.Fatalf("expected state backend unavailable message, got %q", w.Body.String())
	}
}

func TestHandleExecuteReturns503WhenStateStoreUnavailable(t *testing.T) {
	now := time.Now().UTC()
	stateStore := unavailableStateStore{}
	s := wiredServerForTest(&server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     stateStore,
			Leases:       stateStore,
			Secrets:      memory.NewStore(),
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unavailable" },
			NewLeaseTok:  func() string { return "lease-unavailable" },
		},
		authEnabled: false,
		execPolicy: config.ExecutionPolicy{
			ExactMatchExecutables: []string{"echo"},
			MaxOutputBytes:        1024,
			DefaultTimeoutSec:     5,
			MaxTimeoutSec:         5,
		},
		now: func() time.Time { return now },
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/execute", bytes.NewBufferString(`{"lease_token":"lease-unavailable","command":["echo","hi"],"secrets":["github_token"]}`))
	w := httptest.NewRecorder()
	s.handleExecute(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unavailable state backend, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "state backend unavailable") {
		t.Fatalf("expected state backend unavailable message, got %q", w.Body.String())
	}
}
