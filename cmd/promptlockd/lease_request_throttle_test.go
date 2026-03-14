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
	"github.com/lunemec/promptlock/internal/config"
	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestHandleRequestReturns429WhenPendingCapReached(t *testing.T) {
	now := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp-1",
		WorkdirFingerprint: "wd-1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-20 * time.Second),
	})
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-2",
		AgentID:            "agent-1",
		TaskID:             "task-2",
		Reason:             "second",
		TTLMinutes:         5,
		Secrets:            []string{"npm_token"},
		CommandFingerprint: "fp-2",
		WorkdirFingerprint: "wd-2",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-10 * time.Second),
	})

	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-new" },
			NewLeaseTok:  func() string { return "lease-new" },
		},
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      "/non-empty",
		stateStorePersister: persistShouldNotBeCalled{},
		now:                 func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"agent-1","task_id":"task-3","reason":"third","ttl_minutes":5,"secrets":["slack_token"],"command_fingerprint":"fp-3","workdir_fingerprint":"wd-3"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for pending-cap throttle, got %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Retry-After"); got != "60" {
		t.Fatalf("expected Retry-After=60, got %q", got)
	}
	body := w.Body.String()
	if !strings.Contains(body, "pending request cap reached") {
		t.Fatalf("expected pending-cap guidance in response body, got %s", body)
	}
	if !strings.Contains(body, "retry_after_seconds=60") {
		t.Fatalf("expected retry-after metadata in response body, got %s", body)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected throttled request to avoid pending write, got %d pending", len(pending))
	}
}

func TestHandleRequestHonorsCustomPendingCap(t *testing.T) {
	now := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp-1",
		WorkdirFingerprint: "wd-1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-20 * time.Second),
	})

	s := &server{
		svc: app.Service{
			Policy:        domain.DefaultPolicy(),
			RequestPolicy: app.RequestPolicy{IdenticalRequestCooldown: 60 * time.Second, MaxPendingPerAgent: 1, EnableActiveLeaseReuse: true},
			Requests:      store,
			Leases:        store,
			Secrets:       store,
			Audit:         unavailableTestAudit{},
			Now:           func() time.Time { return now },
			NewRequestID:  func() string { return "req-new" },
			NewLeaseTok:   func() string { return "lease-new" },
		},
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      "/non-empty",
		stateStorePersister: persistShouldNotBeCalled{},
		now:                 func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"agent-1","task_id":"task-2","reason":"second","ttl_minutes":5,"secrets":["npm_token"],"command_fingerprint":"fp-2","workdir_fingerprint":"wd-2"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for custom pending-cap throttle, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleRequestReturns429WithRetryAfterForCooldown(t *testing.T) {
	now := time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC)
	store := memory.NewStore()
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-1",
		AgentID:            "agent-1",
		TaskID:             "task-1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp-1",
		WorkdirFingerprint: "wd-1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-25 * time.Second),
	})

	s := &server{
		svc: app.Service{
			Policy:       domain.DefaultPolicy(),
			Requests:     store,
			Leases:       store,
			Secrets:      store,
			Audit:        unavailableTestAudit{},
			Now:          func() time.Time { return now },
			NewRequestID: func() string { return "req-new" },
			NewLeaseTok:  func() string { return "lease-new" },
		},
		authEnabled:         false,
		authCfg:             config.AuthConfig{EnableAuth: false},
		stateStoreFile:      "/non-empty",
		stateStorePersister: persistShouldNotBeCalled{},
		now:                 func() time.Time { return now },
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/leases/request", bytes.NewBufferString(`{"agent_id":"agent-1","task_id":"task-2","reason":"repeat","ttl_minutes":5,"secrets":[" npm_token ","github_token"],"command_fingerprint":"fp-1","workdir_fingerprint":"wd-1"}`))
	w := httptest.NewRecorder()
	s.handleRequest(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for cooldown throttle, got %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Retry-After"); got != "35" {
		t.Fatalf("expected Retry-After=35, got %q", got)
	}
	body := w.Body.String()
	if !strings.Contains(body, "equivalent request cooldown active") {
		t.Fatalf("expected cooldown guidance in response body, got %s", body)
	}
	if !strings.Contains(body, "retry_after_seconds=35") {
		t.Fatalf("expected retry-after metadata in response body, got %s", body)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected throttled request to avoid pending write, got %d pending", len(pending))
	}
}
