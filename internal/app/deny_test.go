package app

import (
	"testing"
	"time"

	"github.com/lunemec/promptlock/internal/adapters/memory"
	"github.com/lunemec/promptlock/internal/core/domain"
)

func TestDenyRequest(t *testing.T) {
	store := memory.NewStore()
	a := &auditBuf{}
	svc := Service{
		Policy:       domain.DefaultPolicy(),
		Requests:     store,
		Leases:       store,
		Secrets:      store,
		Audit:        a,
		Now:          func() time.Time { return time.Now().UTC() },
		NewRequestID: func() string { return "req_deny" },
		NewLeaseTok:  func() string { return "lease_deny" },
	}
	_, err := svc.RequestLease("agent1", "task1", "test", 5, []string{"github_token"}, "fp-deny", "wd-deny", "", "")
	if err != nil {
		t.Fatal(err)
	}
	req, err := svc.DenyRequest("req_deny", "operator decision")
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != domain.RequestDenied {
		t.Fatalf("expected denied status")
	}
}
