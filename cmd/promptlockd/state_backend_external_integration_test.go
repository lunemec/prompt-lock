package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/externalstate"
	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/app"
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestExternalStateBackendHappyPathFlow(t *testing.T) {
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")

	requests := map[string]domain.LeaseRequest{}
	leases := map[string]domain.Lease{}
	seenAuth := 0

	stateBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer state-token"; got != want {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}
		seenAuth++

		switch {
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/state/requests/"):
			var req domain.LeaseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			requests[req.ID] = req
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/v1/state/requests/pending":
			pending := make([]domain.LeaseRequest, 0)
			for _, req := range requests {
				if req.Status == domain.RequestPending {
					pending = append(pending, req)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"pending": pending})
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/state/requests/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/state/requests/")
			req, ok := requests[id]
			if !ok {
				http.Error(w, "request not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(req)
			return
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/state/leases/"):
			var lease domain.Lease
			if err := json.NewDecoder(r.Body).Decode(&lease); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			leases[lease.Token] = lease
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/state/leases/by-request/"):
			requestID := strings.TrimPrefix(r.URL.Path, "/v1/state/leases/by-request/")
			for _, lease := range leases {
				if lease.RequestID == requestID {
					_ = json.NewEncoder(w).Encode(lease)
					return
				}
			}
			http.Error(w, "lease not found for request", http.StatusNotFound)
			return
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/state/leases/"):
			token := strings.TrimPrefix(r.URL.Path, "/v1/state/leases/")
			lease, ok := leases[token]
			if !ok {
				http.Error(w, "lease not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(lease)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer stateBackend.Close()

	store, err := externalstate.New(stateBackend.URL, "PROMPTLOCK_EXTERNAL_STATE_TOKEN", 5)
	if err != nil {
		t.Fatalf("new external state store: %v", err)
	}

	secrets := memory.NewStore()
	secrets.SetSecret("github_token", "token_test_value")

	now := time.Now().UTC()
	s := &server{
		svc: app.Service{
			Policy:                     domain.DefaultPolicy(),
			AllowPlaintextSecretReturn: true,
			Requests:                   store,
			Leases:                     store,
			Secrets:                    secrets,
			Audit:                      unavailableTestAudit{},
			Now:                        func() time.Time { return now },
			NewRequestID:               func() string { return "req-ext-1" },
			NewLeaseTok:                func() string { return "lease-ext-1" },
		},
		authEnabled: false,
		authCfg:     config.AuthConfig{AllowPlaintextSecretReturn: true},
		now:         func() time.Time { return now },
	}

	requestW := httptest.NewRecorder()
	requestReq := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"agent-1","task_id":"task-1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-1","workdir_fingerprint":"wd-1"}`))
	s.handleRequest(requestW, requestReq)
	if requestW.Code != http.StatusOK {
		t.Fatalf("request failed: code=%d body=%s", requestW.Code, requestW.Body.String())
	}

	approveW := httptest.NewRecorder()
	approveReq := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-ext-1", bytes.NewBufferString(`{"ttl_minutes":5}`))
	s.handleApprove(approveW, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("approve failed: code=%d body=%s", approveW.Code, approveW.Body.String())
	}
	if !strings.Contains(approveW.Body.String(), "lease-ext-1") {
		t.Fatalf("expected lease token in approve response, got %s", approveW.Body.String())
	}

	statusW := httptest.NewRecorder()
	statusReq := httptest.NewRequest(http.MethodGet, "/v1/requests/status?request_id=req-ext-1", nil)
	s.handleRequestStatus(statusW, statusReq)
	if statusW.Code != http.StatusOK {
		t.Fatalf("request status failed: code=%d body=%s", statusW.Code, statusW.Body.String())
	}
	if !strings.Contains(statusW.Body.String(), `"approved"`) {
		t.Fatalf("expected approved status, got %s", statusW.Body.String())
	}

	byReqW := httptest.NewRecorder()
	byReqReq := httptest.NewRequest(http.MethodGet, "/v1/leases/by-request?request_id=req-ext-1", nil)
	s.handleLeaseByRequest(byReqW, byReqReq)
	if byReqW.Code != http.StatusOK {
		t.Fatalf("lease by request failed: code=%d body=%s", byReqW.Code, byReqW.Body.String())
	}
	if !strings.Contains(byReqW.Body.String(), "lease-ext-1") {
		t.Fatalf("expected lease token in by-request response, got %s", byReqW.Body.String())
	}

	pendingW := httptest.NewRecorder()
	pendingReq := httptest.NewRequest(http.MethodGet, "/v1/requests/pending", nil)
	s.handlePendingRequests(pendingW, pendingReq)
	if pendingW.Code != http.StatusOK {
		t.Fatalf("pending requests failed: code=%d body=%s", pendingW.Code, pendingW.Body.String())
	}
	if !strings.Contains(pendingW.Body.String(), `"pending":[]`) {
		t.Fatalf("expected empty pending queue after approval, got %s", pendingW.Body.String())
	}

	accessW := httptest.NewRecorder()
	accessReq := httptest.NewRequest(http.MethodPost, "/v1/leases/access", bytes.NewBufferString(`{"lease_token":"lease-ext-1","secret":"github_token","command_fingerprint":"fp-1","workdir_fingerprint":"wd-1"}`))
	s.handleAccess(accessW, accessReq)
	if accessW.Code != http.StatusOK {
		t.Fatalf("access failed: code=%d body=%s", accessW.Code, accessW.Body.String())
	}
	if !strings.Contains(accessW.Body.String(), "token_test_value") {
		t.Fatalf("expected secret value in access response, got %s", accessW.Body.String())
	}

	if seenAuth == 0 {
		t.Fatalf("expected backend auth header to be used")
	}
}

func TestExternalStateRequestCreateRollsBackWhenAuditFails(t *testing.T) {
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")

	requests := map[string]domain.LeaseRequest{}
	leases := map[string]domain.Lease{}
	stateBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer state-token"; got != want {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/state/requests/"):
			var req domain.LeaseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			requests[req.ID] = req
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/state/requests/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/state/requests/")
			delete(requests, id)
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/state/leases/"):
			token := strings.TrimPrefix(r.URL.Path, "/v1/state/leases/")
			delete(leases, token)
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer stateBackend.Close()

	store, err := externalstate.New(stateBackend.URL, "PROMPTLOCK_EXTERNAL_STATE_TOKEN", 5)
	if err != nil {
		t.Fatalf("new external state store: %v", err)
	}

	now := time.Now().UTC()
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      memory.NewStore(),
			Audit:        failingAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-ext-audit-rollback" },
			NewLeaseTok:  func() string { return "lease-unused" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	}

	requestW := httptest.NewRecorder()
	requestReq := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"agent-1","task_id":"task-1","reason":"r","ttl_minutes":5,"secrets":["github_token"],"command_fingerprint":"fp-1","workdir_fingerprint":"wd-1"}`))
	s.handleRequest(requestW, requestReq)
	if requestW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on audit failure, got %d body=%s", requestW.Code, requestW.Body.String())
	}
	if len(requests) != 0 {
		t.Fatalf("expected external request rollback, got %#v", requests)
	}
	if len(leases) != 0 {
		t.Fatalf("expected no external lease state, got %#v", leases)
	}
}

func TestExternalStateApproveRollsBackWhenAuditFails(t *testing.T) {
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")

	requests := map[string]domain.LeaseRequest{
		"req-ext-approve-rollback": {
			ID:                 "req-ext-approve-rollback",
			AgentID:            "agent-1",
			TaskID:             "task-1",
			Reason:             "r",
			TTLMinutes:         5,
			Secrets:            []string{"github_token"},
			CommandFingerprint: "fp-1",
			WorkdirFingerprint: "wd-1",
			Status:             domain.RequestPending,
			CreatedAt:          time.Now().UTC(),
		},
	}
	leases := map[string]domain.Lease{}
	stateBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer state-token"; got != want {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/state/requests/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/state/requests/")
			req, ok := requests[id]
			if !ok {
				http.Error(w, "request not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(req)
			return
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/state/requests/"):
			var req domain.LeaseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			requests[req.ID] = req
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/state/leases/"):
			var lease domain.Lease
			if err := json.NewDecoder(r.Body).Decode(&lease); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			leases[lease.Token] = lease
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/state/leases/"):
			token := strings.TrimPrefix(r.URL.Path, "/v1/state/leases/")
			delete(leases, token)
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer stateBackend.Close()

	store, err := externalstate.New(stateBackend.URL, "PROMPTLOCK_EXTERNAL_STATE_TOKEN", 5)
	if err != nil {
		t.Fatalf("new external state store: %v", err)
	}

	now := time.Now().UTC()
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      memory.NewStore(),
			Audit:        failingAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unused" },
			NewLeaseTok:  func() string { return "lease-ext-approve-rollback" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	}

	approveW := httptest.NewRecorder()
	approveReq := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-ext-approve-rollback", bytes.NewBufferString(`{"ttl_minutes":5}`))
	s.handleApprove(approveW, approveReq)
	if approveW.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 on audit failure, got %d body=%s", approveW.Code, approveW.Body.String())
	}

	if got := requests["req-ext-approve-rollback"].Status; got != domain.RequestPending {
		t.Fatalf("expected external request rollback to pending, got %s", got)
	}
	if len(leases) != 0 {
		t.Fatalf("expected external lease rollback, got %#v", leases)
	}
}

func TestExternalStateApproveKeepsCommittedStateWhenSupplementaryEnvPathAuditFails(t *testing.T) {
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")

	requests := map[string]domain.LeaseRequest{
		"req-ext-env-approve-rollback": {
			ID:                 "req-ext-env-approve-rollback",
			AgentID:            "agent-1",
			TaskID:             "task-1",
			Reason:             "r",
			TTLMinutes:         5,
			Secrets:            []string{"github_token"},
			CommandFingerprint: "fp-1",
			WorkdirFingerprint: "wd-1",
			EnvPath:            "./.env",
			EnvPathCanonical:   "/workspace/.env",
			Status:             domain.RequestPending,
			CreatedAt:          time.Now().UTC(),
		},
	}
	leases := map[string]domain.Lease{}
	stateBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer state-token"; got != want {
			http.Error(w, "missing auth", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/state/requests/"):
			id := strings.TrimPrefix(r.URL.Path, "/v1/state/requests/")
			req, ok := requests[id]
			if !ok {
				http.Error(w, "request not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(req)
			return
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/state/requests/"):
			var req domain.LeaseRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			requests[req.ID] = req
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/v1/state/leases/"):
			var lease domain.Lease
			if err := json.NewDecoder(r.Body).Decode(&lease); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			leases[lease.Token] = lease
			w.WriteHeader(http.StatusNoContent)
			return
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/state/leases/"):
			token := strings.TrimPrefix(r.URL.Path, "/v1/state/leases/")
			delete(leases, token)
			w.WriteHeader(http.StatusNoContent)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer stateBackend.Close()

	store, err := externalstate.New(stateBackend.URL, "PROMPTLOCK_EXTERNAL_STATE_TOKEN", 5)
	if err != nil {
		t.Fatalf("new external state store: %v", err)
	}

	now := time.Now().UTC()
	audit := &scriptedAudit{failAt: map[int]error{2: errors.New("audit disk offline")}}
	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      memory.NewStore(),
			Audit:        audit,
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-unused" },
			NewLeaseTok:  func() string { return "lease-ext-env-approve-rollback" },
		},
		authEnabled: false,
		now:         func() time.Time { return now },
	}
	s.svc.AuditFailureHandler = func(err error) error {
		return s.closeDurabilityGate("audit", err)
	}

	approveW := httptest.NewRecorder()
	approveReq := httptest.NewRequest(http.MethodPost, "/v1/leases/approve?request_id=req-ext-env-approve-rollback", bytes.NewBufferString(`{"ttl_minutes":5}`))
	s.handleApprove(approveW, approveReq)
	if approveW.Code != http.StatusOK {
		t.Fatalf("expected 200 when supplementary env-path approval audit fails, got %d body=%s", approveW.Code, approveW.Body.String())
	}

	if got := requests["req-ext-env-approve-rollback"].Status; got != domain.RequestApproved {
		t.Fatalf("expected external request to stay approved, got %s", got)
	}
	if _, ok := leases["lease-ext-env-approve-rollback"]; !ok {
		t.Fatalf("expected external lease to stay committed, got %#v", leases)
	}
}
