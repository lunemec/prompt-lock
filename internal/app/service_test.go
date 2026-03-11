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

	req, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1")
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

	result, err := svc.RequestLeaseWithPolicy("agent1", "task1", "test", 5, []string{" npm_token ", "github_token", "github_token"}, "fp1", "wd1")
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

	result, err := svc.RequestLeaseWithPolicy("agent1", "task1", "test", 5, []string{"github_token"}, "fp1", "wd1")
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
