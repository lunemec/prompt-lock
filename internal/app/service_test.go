package app

import (
	"errors"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/core/domain"
	"github.com/lunemec/promptlock/internal/core/ports"
)

type auditBuf struct{ events []ports.AuditEvent }

type failingSecretStore struct{}

func (failingSecretStore) GetSecret(string) (string, error) { return "", errors.New("backend timeout") }

func (a *auditBuf) Write(e ports.AuditEvent) error {
	a.events = append(a.events, e)
	return nil
}

func TestLeaseFlow(t *testing.T) {
	store := memory.NewStore()
	store.SetSecret("github_token", "x")
	a := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	seq := 0
	svc := Service{
		Policy:   domain.DefaultPolicy(),
		Requests: store,
		Leases:   store,
		Secrets:  store,
		Audit:    a,
		Now:      func() time.Time { return now },
		NewRequestID: func() string {
			seq++
			return "req_test"
		},
		NewLeaseTok: func() string { return "lease_test" },
	}

	req, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != domain.RequestPending {
		t.Fatalf("expected pending")
	}

	lease, err := svc.ApproveRequest(req.ID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if lease.Token == "" {
		t.Fatalf("expected lease token")
	}

	val, err := svc.AccessSecret(lease.Token, "github_token", "fp1", "wd1")
	if err != nil {
		t.Fatal(err)
	}
	if val != "x" {
		t.Fatalf("unexpected secret value")
	}

	if len(a.events) < 3 {
		t.Fatalf("expected audit events")
	}

	if _, err := svc.AccessSecret(lease.Token, "github_token", "different-fp", "wd1"); err == nil {
		t.Fatalf("expected fingerprint mismatch error")
	}
	if _, err := svc.AccessSecret(lease.Token, "github_token", "fp1", "other-wd"); err == nil {
		t.Fatalf("expected workdir mismatch error")
	}
}

func TestAccessSecretBackendFailureIsAuditedAndDeterministic(t *testing.T) {
	store := memory.NewStore()
	a := &auditBuf{}
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      failingSecretStore{},
		Audit:        a,
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_test" },
		NewLeaseTok:  func() string { return "lease_test" },
	}

	_ = store.SaveRequest(domain.LeaseRequest{ID: "req_test", AgentID: "agent1", TaskID: "task1", TTLMinutes: 5, Secrets: []string{"github_token"}, CommandFingerprint: "fp1", WorkdirFingerprint: "wd1", Status: domain.RequestApproved, CreatedAt: now})
	_ = store.SaveLease(domain.Lease{Token: "lease_test", RequestID: "req_test", AgentID: "agent1", TaskID: "task1", Secrets: []string{"github_token"}, CommandFingerprint: "fp1", WorkdirFingerprint: "wd1", ExpiresAt: now.Add(5 * time.Minute)})

	_, err := svc.AccessSecret("lease_test", "github_token", "fp1", "wd1")
	if err == nil || err.Error() != "secret backend unavailable" {
		t.Fatalf("expected deterministic backend error, got %v", err)
	}

	found := false
	for _, ev := range a.events {
		if ev.Event == "secret_backend_error" {
			found = true
			if ev.Metadata["reason"] == "" {
				t.Fatalf("expected backend error reason metadata")
			}
		}
	}
	if !found {
		t.Fatalf("expected secret_backend_error audit event")
	}
}

func TestRequestLeaseWithPolicyReusesEquivalentActiveLease(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveLease(domain.Lease{
		Token:              "lease_active",
		RequestID:          "req_existing",
		AgentID:            "agent1",
		TaskID:             "task-old",
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		ExpiresAt:          now.Add(10 * time.Minute),
	})

	svc := Service{
		Policy:   domain.DefaultPolicy(),
		Requests: store,
		Leases:   store,
		Secrets:  store,
		Audit:    &auditBuf{},
		Now:      func() time.Time { return now },
		NewRequestID: func() string {
			return "req_new"
		},
		NewLeaseTok: func() string { return "lease_new" },
	}

	result, err := svc.RequestLeaseWithPolicy("agent1", "task1", "test", 5, []string{" npm_token ", "github_token", "github_token"}, "fp1", "wd1", "", "")
	if err != nil {
		t.Fatalf("request lease with policy: %v", err)
	}
	if !result.Reused {
		t.Fatalf("expected equivalent active lease to be reused")
	}
	if result.Lease.Token != "lease_active" {
		t.Fatalf("expected reused lease token lease_active, got %q", result.Lease.Token)
	}
	if result.Request.ID != "" {
		t.Fatalf("expected no new request when reusing active lease, got %q", result.Request.ID)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending requests when lease is reused, got %d", len(pending))
	}
}

func TestRequestLeaseWithPolicyCreatesPendingWhenNoActiveEquivalentLease(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	svc := Service{
		Policy:   domain.DefaultPolicy(),
		Requests: store,
		Leases:   store,
		Secrets:  store,
		Audit:    &auditBuf{},
		Now:      func() time.Time { return now },
		NewRequestID: func() string {
			return "req_new"
		},
		NewLeaseTok: func() string { return "lease_new" },
	}

	result, err := svc.RequestLeaseWithPolicy("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1", "", "")
	if err != nil {
		t.Fatalf("request lease with policy: %v", err)
	}
	if result.Reused {
		t.Fatalf("expected new pending request when no reusable lease exists")
	}
	if result.Request.ID != "req_new" {
		t.Fatalf("expected created request id req_new, got %q", result.Request.ID)
	}
	if result.Request.Status != domain.RequestPending {
		t.Fatalf("expected pending request status, got %q", result.Request.Status)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "req_new" {
		t.Fatalf("expected created request to be pending, got %#v", pending)
	}
}

func TestRequestLeaseWithPolicyThrottlesWhenPendingCapReached(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-20 * time.Second),
	})
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_2",
		AgentID:            "agent1",
		TaskID:             "task_2",
		Reason:             "second",
		TTLMinutes:         5,
		Secrets:            []string{"npm_token"},
		CommandFingerprint: "fp2",
		WorkdirFingerprint: "wd2",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-10 * time.Second),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task3", "third", 5, []string{"slack_token"}, "fp3", "wd3", "", "")
	if err == nil {
		t.Fatalf("expected pending-cap throttle error")
	}

	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected RequestThrottleError, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonPendingCap {
		t.Fatalf("expected pending-cap throttle reason, got %q", throttleErr.Reason)
	}
	if throttleErr.RetryAfter != 60*time.Second {
		t.Fatalf("expected retry-after 60s for pending-cap, got %s", throttleErr.RetryAfter)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected pending queue unchanged on throttle, got %d", len(pending))
	}
}

func TestRequestLeaseWithPolicyThrottlesEquivalentRequestWithinCooldown(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token", "npm_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-15 * time.Second),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task2", "repeat", 5, []string{" npm_token ", "github_token"}, "fp1", "wd1", "", "")
	if err == nil {
		t.Fatalf("expected cooldown throttle error")
	}

	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected RequestThrottleError, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonCooldown {
		t.Fatalf("expected cooldown throttle reason, got %q", throttleErr.Reason)
	}
	if throttleErr.RetryAfter != 45*time.Second {
		t.Fatalf("expected retry-after 45s for cooldown, got %s", throttleErr.RetryAfter)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected pending queue unchanged on cooldown throttle, got %d", len(pending))
	}
}

func TestRequestLeaseWithPolicyChecksPendingCapBeforeCooldown(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_1",
		AgentID:            "agent1",
		TaskID:             "task_1",
		Reason:             "first",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-10 * time.Second),
	})
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req_2",
		AgentID:            "agent1",
		TaskID:             "task_2",
		Reason:             "second",
		TTLMinutes:         5,
		Secrets:            []string{"npm_token"},
		CommandFingerprint: "fp2",
		WorkdirFingerprint: "wd2",
		Status:             domain.RequestPending,
		CreatedAt:          now.Add(-40 * time.Second),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new" },
		NewLeaseTok:  func() string { return "lease_new" },
	}

	_, err := svc.RequestLeaseWithPolicy("agent1", "task2", "repeat", 5, []string{"github_token"}, "fp1", "wd1", "", "")
	if err == nil {
		t.Fatalf("expected pending-cap throttle error")
	}

	var throttleErr *RequestThrottleError
	if !errors.As(err, &throttleErr) {
		t.Fatalf("expected RequestThrottleError, got %v", err)
	}
	if throttleErr.Reason != RequestThrottleReasonPendingCap {
		t.Fatalf("expected pending-cap to win when both checks match, got %q", throttleErr.Reason)
	}
}

func TestRequestLeaseWithPolicyDoesNotReuseActiveLeaseWhenEnvPathProvided(t *testing.T) {
	store := memory.NewStore()
	now := time.Date(2026, 3, 7, 18, 0, 0, 0, time.UTC)
	_ = store.SaveLease(domain.Lease{
		Token:              "lease_active",
		RequestID:          "req_existing",
		AgentID:            "agent1",
		TaskID:             "task-old",
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp1",
		WorkdirFingerprint: "wd1",
		ExpiresAt:          now.Add(10 * time.Minute),
	})

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req_new_env" },
		NewLeaseTok:  func() string { return "lease_new_env" },
	}

	result, err := svc.RequestLeaseWithPolicy(
		"agent1",
		"task-new",
		"requires env path confirmation",
		5,
		[]string{"github_token"},
		"fp1",
		"wd1",
		"./.env",
		"/workspace/project/.env",
	)
	if err != nil {
		t.Fatalf("request lease with env path: %v", err)
	}
	if result.Reused {
		t.Fatalf("expected env-path request to require fresh approval instead of active-lease reuse")
	}
	if result.Request.ID != "req_new_env" {
		t.Fatalf("expected new pending request, got %#v", result.Request)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != "req_new_env" {
		t.Fatalf("expected one new pending request, got %#v", pending)
	}
}
