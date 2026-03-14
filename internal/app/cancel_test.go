package app

import (
	"errors"
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestCancelRequestByAgent(t *testing.T) {
	store := memory.NewStore()
	a := &auditBuf{}
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        a,
		Now:          func() time.Time { return time.Now().UTC() },
		NewRequestID: func() string { return "req_cancel" },
		NewLeaseTok:  func() string { return "lease_cancel" },
	}
	_, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"}, "fp-cancel", "wd-cancel", "", "")
	if err != nil {
		t.Fatal(err)
	}
	req, err := svc.CancelRequestByAgent("req_cancel", "agent1", "mcp notification cancelled")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != domain.RequestDenied {
		t.Fatalf("expected denied status")
	}
}

func TestCancelRequestByAgentRejectsNonOwner(t *testing.T) {
	store := memory.NewStore()
	a := &auditBuf{}
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        a,
		Now:          func() time.Time { return time.Now().UTC() },
		NewRequestID: func() string { return "req_cancel_owner" },
		NewLeaseTok:  func() string { return "lease_cancel_owner" },
	}
	_, err := svc.RequestLease("agent-owner", "task1", "test", 5, []string{"github_token"}, "fp-cancel", "wd-cancel", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CancelRequestByAgent("req_cancel_owner", "agent-other", "mcp notification cancelled")
	if !errors.Is(err, ErrRequestNotOwned) {
		t.Fatalf("expected ErrRequestNotOwned, got %v", err)
	}
}
