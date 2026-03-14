package externalstate

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

func TestStoreRoundTripRequestsAndLeases(t *testing.T) {
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")

	requests := map[string]domain.LeaseRequest{}
	leases := map[string]domain.Lease{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer ts.Close()

	store, err := New(ts.URL, "PROMPTLOCK_EXTERNAL_STATE_TOKEN", 5)
	if err != nil {
		t.Fatalf("new external state store: %v", err)
	}

	now := time.Now().UTC().Round(time.Second)
	request := domain.LeaseRequest{
		ID:         "req-1",
		AgentID:    "agent-1",
		TaskID:     "task-1",
		Reason:     "deploy",
		TTLMinutes: 5,
		Secrets:    []string{"github_token"},
		Status:     domain.RequestPending,
		CreatedAt:  now,
	}
	if err := store.SaveRequest(request); err != nil {
		t.Fatalf("save request: %v", err)
	}

	gotRequest, err := store.GetRequest(request.ID)
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if gotRequest.ID != request.ID || gotRequest.Status != domain.RequestPending {
		t.Fatalf("unexpected request round-trip: %#v", gotRequest)
	}

	request.Status = domain.RequestApproved
	if err := store.UpdateRequest(request); err != nil {
		t.Fatalf("update request: %v", err)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending requests after update, got %d", len(pending))
	}

	lease := domain.Lease{
		Token:     "lease-1",
		RequestID: request.ID,
		AgentID:   "agent-1",
		TaskID:    "task-1",
		Secrets:   []string{"github_token"},
		ExpiresAt: now.Add(5 * time.Minute),
	}
	if err := store.SaveLease(lease); err != nil {
		t.Fatalf("save lease: %v", err)
	}

	gotLease, err := store.GetLease(lease.Token)
	if err != nil {
		t.Fatalf("get lease: %v", err)
	}
	if gotLease.Token != lease.Token || gotLease.RequestID != request.ID {
		t.Fatalf("unexpected lease round-trip: %#v", gotLease)
	}

	gotByReq, err := store.GetLeaseByRequestID(request.ID)
	if err != nil {
		t.Fatalf("get lease by request id: %v", err)
	}
	if gotByReq.Token != lease.Token {
		t.Fatalf("expected lease token %q, got %q", lease.Token, gotByReq.Token)
	}
}

func TestStoreUnavailableClassification(t *testing.T) {
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream timeout", http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	store, err := New(ts.URL, "PROMPTLOCK_EXTERNAL_STATE_TOKEN", 5)
	if err != nil {
		t.Fatalf("new external state store: %v", err)
	}

	err = store.SaveRequest(domain.LeaseRequest{ID: "req-1"})
	if err == nil {
		t.Fatalf("expected unavailable error")
	}
	if !errors.Is(err, ports.ErrStoreUnavailable) {
		t.Fatalf("expected ErrStoreUnavailable, got %v", err)
	}
}

func TestStoreRequiresConfiguredAuthToken(t *testing.T) {
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "")
	store, err := New("https://state.example.internal", "PROMPTLOCK_EXTERNAL_STATE_TOKEN", 5)
	if err != nil {
		t.Fatalf("new external state store: %v", err)
	}

	err = store.SaveRequest(domain.LeaseRequest{ID: "req-1"})
	if err == nil {
		t.Fatalf("expected missing token error")
	}
	if !errors.Is(err, ports.ErrStoreUnavailable) {
		t.Fatalf("expected ErrStoreUnavailable for missing auth token, got %v", err)
	}
}

func TestStoreNotFoundIsNotUnavailable(t *testing.T) {
	t.Setenv("PROMPTLOCK_EXTERNAL_STATE_TOKEN", "state-token")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "request not found", http.StatusNotFound)
	}))
	defer ts.Close()

	store, err := New(ts.URL, "PROMPTLOCK_EXTERNAL_STATE_TOKEN", 5)
	if err != nil {
		t.Fatalf("new external state store: %v", err)
	}

	_, err = store.GetRequest("missing")
	if err == nil {
		t.Fatalf("expected not-found error")
	}
	if errors.Is(err, ports.ErrStoreUnavailable) {
		t.Fatalf("did not expect unavailable classification for not-found: %v", err)
	}
}
