package app

import (
	"errors"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/core/domain"
)

func newOwnershipService(now time.Time) (Service, *memory.Store) {
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

	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        &auditBuf{},
		Now:          func() time.Time { return now },
		NewRequestID: func() string { return "req-new" },
		NewLeaseTok:  func() string { return "lease-new" },
	}
	return svc, store
}

func TestRequestStatusByAgentRejectsCrossAgentRead(t *testing.T) {
	svc, _ := newOwnershipService(time.Now().UTC())

	_, err := svc.RequestStatusByAgent("req-a", "agent-b")
	if !errors.Is(err, ErrRequestNotOwned) {
		t.Fatalf("expected ErrRequestNotOwned, got %v", err)
	}
}

func TestLeaseByRequestForAgentRejectsCrossAgentRead(t *testing.T) {
	svc, _ := newOwnershipService(time.Now().UTC())

	_, err := svc.LeaseByRequestForAgent("req-a", "agent-b")
	if !errors.Is(err, ErrRequestNotOwned) {
		t.Fatalf("expected ErrRequestNotOwned, got %v", err)
	}
}

func TestLeaseSecretUseRejectsCrossAgentAccess(t *testing.T) {
	svc, _ := newOwnershipService(time.Now().UTC())

	_, err := svc.AccessSecretByAgent("agent-b", "lease-a", "github_token", "fp-a", "wd-a")
	if !errors.Is(err, ErrLeaseNotOwned) {
		t.Fatalf("expected ErrLeaseNotOwned from access, got %v", err)
	}

	_, err = svc.ResolveExecutionSecretsByAgent("agent-b", "lease-a", []string{"github_token"}, "fp-a", "wd-a")
	if !errors.Is(err, ErrLeaseNotOwned) {
		t.Fatalf("expected ErrLeaseNotOwned from resolve, got %v", err)
	}
}

func TestListPendingRequestsReturnsOnlyPendingRequests(t *testing.T) {
	now := time.Now().UTC()
	svc, store := newOwnershipService(now)
	_ = store.SaveRequest(domain.LeaseRequest{
		ID:                 "req-pending",
		AgentID:            "agent-b",
		TaskID:             "task-b",
		TTLMinutes:         5,
		Secrets:            []string{"github_token"},
		CommandFingerprint: "fp-b",
		WorkdirFingerprint: "wd-b",
		Status:             domain.RequestPending,
		CreatedAt:          now,
	})

	pending, err := svc.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected only the pending request to be returned, got %d", len(pending))
	}
	if pending[0].ID != "req-pending" {
		t.Fatalf("expected req-pending, got %q", pending[0].ID)
	}
}

func TestRequestLeaseWithPolicyDoesNotReuseOtherAgentsLease(t *testing.T) {
	now := time.Now().UTC()
	svc, store := newOwnershipService(now)

	result, err := svc.RequestLeaseWithPolicy("agent-b", "task-b", "new request", 5, []string{"github_token"}, "fp-a", "wd-a", "", "")
	if err != nil {
		t.Fatalf("request lease with policy: %v", err)
	}
	if result.Reused {
		t.Fatalf("expected cross-agent request to avoid lease reuse")
	}
	if result.Request.AgentID != "agent-b" {
		t.Fatalf("expected pending request for agent-b, got %q", result.Request.AgentID)
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected one new pending request, got %d", len(pending))
	}
	if pending[0].AgentID != "agent-b" {
		t.Fatalf("expected pending request to belong to agent-b, got %q", pending[0].AgentID)
	}
}

func TestRequestLeaseWithPolicyIgnoresSummariesForReuse(t *testing.T) {
	now := time.Now().UTC()
	svc, store := newOwnershipService(now)

	result, err := svc.RequestLeaseWithPolicyAndIntentAndSummary(
		"agent-a",
		"task-a",
		"same request with different summaries",
		5,
		[]string{"github_token"},
		"",
		"fp-a",
		"wd-a",
		"",
		"git status --short && printf 'changed'",
		"/workspace/project/subdir",
	)
	if err != nil {
		t.Fatalf("request lease with policy: %v", err)
	}
	if !result.Reused {
		t.Fatalf("expected identical fingerprints to reuse the active lease even when summaries differ")
	}

	pending, err := store.ListPendingRequests()
	if err != nil {
		t.Fatalf("list pending requests: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no new pending request when reuse succeeds, got %d", len(pending))
	}
}
